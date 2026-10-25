package usecase

import (
	"context"
	"errors"

	"github.com/moixblau/HappyX.Api/internal/domain"
)

// accessOf works out userID's level in g from the main admin, the guild's grants and the roles
// the bot last saw them with. managesInDiscord says they own the server or are one of its
// Discord Administrators. Grants only count while the person is still in the server.
func (s *Service) accessOf(ctx context.Context, g domain.Guild, userID string, managesInDiscord bool) (domain.AccessLevel, error) {
	if userID == "" {
		return domain.AccessNone, nil
	}
	if g.AdminDiscordID == userID || managesInDiscord {
		return domain.AccessOwner, nil
	}
	grants, err := s.access.ListGrants(ctx, g.DiscordID)
	if err != nil || len(grants) == 0 {
		return domain.AccessNone, err
	}
	m, err := s.members.Get(ctx, g.DiscordID, userID)
	if errors.Is(err, domain.ErrNotFound) || (err == nil && !m.InServer) {
		return domain.AccessNone, nil
	}
	if err != nil {
		return domain.AccessNone, err
	}
	return domain.ResolveAccess(g, userID, false, m.RoleIDs, grants), nil
}

// MemberAccess is what the bot asks before running an admin command for userID.
// managesInDiscord is true when Discord says they own the server or are an Administrator.
func (s *Service) MemberAccess(ctx context.Context, guildID, userID string, managesInDiscord bool) (domain.AccessLevel, error) {
	g, err := s.activeGuild(ctx, guildID)
	if err != nil {
		return domain.AccessNone, err
	}
	return s.accessOf(ctx, g, userID, managesInDiscord)
}

// SyncRoles stores the guild's Discord roles, as the bot sees them. The @everyone role, whose ID
// is the guild's, is left out: granting it would open the reports to the whole server.
func (s *Service) SyncRoles(ctx context.Context, guildID string, roles []domain.Role) error {
	if _, err := s.activeGuild(ctx, guildID); err != nil {
		return err
	}
	if len(roles) > domain.MaxRoleIDs {
		return domain.ValidationError{Reason: "too many roles"}
	}
	out := make([]domain.Role, 0, len(roles))
	for _, ro := range roles {
		if ro.DiscordID == guildID {
			continue
		}
		if err := ro.Validate(); err != nil {
			return err
		}
		out = append(out, ro)
	}
	return s.access.ReplaceRoles(ctx, guildID, out)
}

// AccessSettings is who has access to a guild, and who could be given it.
type AccessSettings struct {
	Guild  domain.Guild
	Grants []domain.AccessGrant
	// Roles and Members are what the main admin and the grants can be chosen from.
	Roles   []domain.Role
	Members []domain.Member
}

// GuildAccessSettings returns who has access to the guild. Only owners may read it.
func (s *Service) GuildAccessSettings(ctx context.Context, v domain.Viewer, guildID string) (AccessSettings, error) {
	g, err := s.ViewerGuild(ctx, v, guildID, domain.AccessOwner)
	if err != nil {
		return AccessSettings{}, err
	}
	return s.accessSettings(ctx, g)
}

func (s *Service) accessSettings(ctx context.Context, g domain.Guild) (AccessSettings, error) {
	out := AccessSettings{Guild: g}
	var err error
	if out.Grants, err = s.access.ListGrants(ctx, g.DiscordID); err != nil {
		return AccessSettings{}, err
	}
	if out.Roles, err = s.access.ListRoles(ctx, g.DiscordID); err != nil {
		return AccessSettings{}, err
	}
	if out.Members, err = s.members.ListInServer(ctx, g.DiscordID); err != nil {
		return AccessSettings{}, err
	}
	return out, nil
}

// SetGuildAccess changes the guild's main admin, who receives the reports, and replaces who
// else has access. Only owners may do it. The admin and every user granted access must be in
// the server, and every role must exist in it.
func (s *Service) SetGuildAccess(ctx context.Context, v domain.Viewer, guildID, adminID string, grants []domain.AccessGrant) (AccessSettings, error) {
	g, err := s.ViewerGuild(ctx, v, guildID, domain.AccessOwner)
	if err != nil {
		return AccessSettings{}, err
	}
	if err := domain.ValidateGrants(grants); err != nil {
		return AccessSettings{}, err
	}
	members, err := s.members.ListInServer(ctx, guildID)
	if err != nil {
		return AccessSettings{}, err
	}
	roles, err := s.access.ListRoles(ctx, guildID)
	if err != nil {
		return AccessSettings{}, err
	}
	inServer := map[string]bool{}
	for _, m := range members {
		inServer[m.DiscordID] = true
	}
	hasRole := map[string]bool{}
	for _, ro := range roles {
		hasRole[ro.DiscordID] = true
	}
	if !inServer[adminID] {
		return AccessSettings{}, domain.ValidationError{Reason: "the main admin must be a member of the server"}
	}
	for _, gr := range grants {
		if gr.Kind == domain.GrantUser && !inServer[gr.TargetID] {
			return AccessSettings{}, domain.ValidationError{Reason: "user " + gr.TargetID + " is not a member of the server"}
		}
		if gr.Kind == domain.GrantRole && !hasRole[gr.TargetID] {
			return AccessSettings{}, domain.ValidationError{Reason: "role " + gr.TargetID + " doesn't exist in the server"}
		}
	}
	if err := s.access.SetAccess(ctx, guildID, adminID, grants); err != nil {
		return AccessSettings{}, err
	}
	g.AdminDiscordID = adminID
	return s.accessSettings(ctx, g)
}

// checkRolesExist fails unless every role is one of the guild's, as the bot last saw them.
func (s *Service) checkRolesExist(ctx context.Context, guildID string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	roles, err := s.access.ListRoles(ctx, guildID)
	if err != nil {
		return err
	}
	has := map[string]bool{}
	for _, ro := range roles {
		has[ro.DiscordID] = true
	}
	for _, id := range ids {
		if !has[id] {
			return domain.ValidationError{Reason: "role " + id + " doesn't exist in the server"}
		}
	}
	return nil
}

// GuildRoles returns the guild's Discord roles, highest first, for those who change its settings.
func (s *Service) GuildRoles(ctx context.Context, v domain.Viewer, guildID string) ([]domain.Role, error) {
	if _, err := s.ViewerGuild(ctx, v, guildID, domain.AccessManage); err != nil {
		return nil, err
	}
	return s.access.ListRoles(ctx, guildID)
}
