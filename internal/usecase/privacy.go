package usecase

import (
	"context"
	"strings"

	"github.com/moixblau/HappyX.Api/internal/domain"
)

// --- Personal data (GDPR) ---

// MyData returns everything HappyX keeps about the viewer, in every guild. Anyone logged in can
// download their own data, whether or not they manage a server.
func (s *Service) MyData(ctx context.Context, v domain.Viewer) (domain.UserData, error) {
	if v.DiscordID == "" {
		return domain.UserData{}, domain.ErrNotFound
	}
	return s.privacy.UserData(ctx, v.DiscordID)
}

// EraseMyData deletes the viewer's answers, kudos and game activity in every guild, and pauses
// the survey for them everywhere. confirmName must be the viewer's username, as they typed it
// to confirm. Guild settings that name them, such as who receives the reports, are left alone.
func (s *Service) EraseMyData(ctx context.Context, v domain.Viewer, confirmName string) error {
	if v.DiscordID == "" {
		return domain.ErrNotFound
	}
	if strings.TrimSpace(confirmName) == "" || strings.TrimSpace(confirmName) != strings.TrimSpace(v.Username) {
		return domain.ValidationError{Reason: "type your username to confirm"}
	}
	return s.privacy.EraseUser(ctx, v.DiscordID)
}

// GuildExport returns the guild's data for an owner (see domain.AccessOwner), with the same
// anonymity as the dashboard (see domain.BuildGuildExport).
func (s *Service) GuildExport(ctx context.Context, v domain.Viewer, guildID string) (domain.GuildExport, error) {
	g, err := s.ViewerGuild(ctx, v, guildID, domain.AccessOwner)
	if err != nil {
		return domain.GuildExport{}, err
	}
	d, err := s.privacy.GuildData(ctx, guildID)
	if err != nil {
		return domain.GuildExport{}, err
	}
	return domain.BuildGuildExport(d, s.today(g)), nil
}
