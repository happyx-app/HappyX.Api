package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/happyx-app/HappyX.Api/internal/domain"
)

// WithVacations gives the Service the vacations' storage. Without it nobody is ever on vacation
// and setting one fails with domain.ErrNotFound.
func (s *Service) WithVacations(v VacationRepository) *Service {
	s.vacations = v
	return s
}

// errNoVacations is what writes return when the Service has no vacation storage (the demo).
var errNoVacations = domain.ErrNotFound

// utcYesterday is the earliest "today" anywhere: a vacation set without a server is checked
// against it, so nobody ahead of UTC is told their last day has passed.
func (s *Service) utcYesterday() time.Time {
	return domain.TruncateDay(s.now().UTC()).AddDate(0, 0, -1)
}

// vacationsIn returns the person's vacations that apply in guildID and end on since or later.
func (s *Service) vacationsIn(ctx context.Context, userID, guildID string, since time.Time) (domain.Vacations, error) {
	if s.vacations == nil {
		return nil, nil
	}
	all, err := s.vacations.ForUser(ctx, userID, since)
	if err != nil {
		return nil, err
	}
	var out domain.Vacations
	for _, v := range all {
		if v.GuildDiscordID == "" || v.GuildDiscordID == guildID {
			out = append(out, v)
		}
	}
	return out, nil
}

// onVacation returns the Discord IDs of the guild's members whose vacation covers day.
func (s *Service) onVacation(ctx context.Context, guildID string, day time.Time) (map[string]bool, error) {
	if s.vacations == nil {
		return nil, nil
	}
	vs, err := s.vacations.ForGuild(ctx, guildID, day)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, v := range vs {
		if v.Covers(day) {
			out[v.UserDiscordID] = true
		}
	}
	return out, nil
}

// --- The person's own ---

// MyVacations returns the viewer's vacations that haven't ended: their own first, then the ones
// admins set in their servers.
func (s *Service) MyVacations(ctx context.Context, v domain.Viewer) ([]domain.Vacation, error) {
	if v.DiscordID == "" || s.vacations == nil {
		return nil, nil
	}
	return s.vacations.ForUser(ctx, v.DiscordID, s.utcYesterday())
}

// SetMyVacation sets the viewer's own vacation, for every server they are in, replacing the one
// they had.
func (s *Service) SetMyVacation(ctx context.Context, v domain.Viewer, from, to time.Time) (domain.Vacation, error) {
	return s.setOwnVacation(ctx, v.DiscordID, from, to, s.utcYesterday())
}

// DeleteMyVacation removes the viewer's own vacation (guildID "") or the one an admin set for
// them in guildID: the survey is theirs to get back.
func (s *Service) DeleteMyVacation(ctx context.Context, v domain.Viewer, guildID string) error {
	if s.vacations == nil {
		return errNoVacations
	}
	return s.vacations.Delete(ctx, v.DiscordID, guildID)
}

func (s *Service) setOwnVacation(ctx context.Context, userID string, from, to, today time.Time) (domain.Vacation, error) {
	if s.vacations == nil {
		return domain.Vacation{}, errNoVacations
	}
	vac := domain.Vacation{UserDiscordID: userID, From: domain.TruncateDay(from), To: domain.TruncateDay(to), SetByDiscordID: userID}
	if err := vac.Validate(today); err != nil {
		return domain.Vacation{}, err
	}
	return s.vacations.Set(ctx, vac, false)
}

// --- From the bot ---

// MemberVacations returns the vacations that apply to the member in the guild and haven't ended.
func (s *Service) MemberVacations(ctx context.Context, guildID, userID string) (domain.Vacations, error) {
	g, err := s.activeGuild(ctx, guildID)
	if err != nil {
		return nil, err
	}
	return s.vacationsIn(ctx, userID, guildID, s.today(g))
}

// SetMemberOwnVacation is /happyx vacation: the member sets their own vacation, for every server,
// with the dates read in this server's timezone.
func (s *Service) SetMemberOwnVacation(ctx context.Context, guildID, userID string, from, to time.Time) (domain.Vacation, error) {
	g, err := s.activeGuild(ctx, guildID)
	if err != nil {
		return domain.Vacation{}, err
	}
	if _, err := s.members.Get(ctx, guildID, userID); err != nil {
		return domain.Vacation{}, err
	}
	return s.setOwnVacation(ctx, userID, from, to, s.today(g))
}

// EndMemberVacations removes the member's own vacation and the one set for them in the guild, so
// the survey comes back from the next one. Having none is fine.
func (s *Service) EndMemberVacations(ctx context.Context, guildID, userID string) error {
	if _, err := s.activeGuild(ctx, guildID); err != nil {
		return err
	}
	if s.vacations == nil {
		return nil
	}
	for _, scope := range []string{"", guildID} {
		if err := s.vacations.Delete(ctx, userID, scope); err != nil && !errors.Is(err, domain.ErrNotFound) {
			return err
		}
	}
	return nil
}

// VacationNotices returns the vacations admins set that the bot has yet to tell the person about.
func (s *Service) VacationNotices(ctx context.Context) ([]domain.Vacation, error) {
	if s.vacations == nil {
		return nil, nil
	}
	return s.vacations.PendingNotices(ctx)
}

// DoneVacationNotice records that the bot told the person about the vacation.
func (s *Service) DoneVacationNotice(ctx context.Context, id int64) error {
	if s.vacations == nil {
		return errNoVacations
	}
	return s.vacations.DoneNotice(ctx, id)
}

// DeleteOldVacations deletes the vacations that ended more than domain.VacationRetention days ago,
// once they can no longer matter to anyone's streak.
func (s *Service) DeleteOldVacations(ctx context.Context) (int, error) {
	if s.vacations == nil {
		return 0, nil
	}
	return s.vacations.DeleteEndedBefore(ctx, s.utcYesterday().AddDate(0, 0, -domain.VacationRetention))
}

// --- Admins, on the dashboard ---

// GuildVacations is what the dashboard's vacations card shows: the vacations of the guild's members
// that haven't ended, and who can be given one.
type GuildVacations struct {
	Guild     domain.Guild
	Vacations []domain.Vacation
	Members   []domain.Member // in the server, by name
}

// ListGuildVacations needs the "manage" level, like the rest of the settings.
func (s *Service) ListGuildVacations(ctx context.Context, v domain.Viewer, guildID string) (GuildVacations, error) {
	g, err := s.ViewerGuild(ctx, v, guildID, domain.AccessManage)
	if err != nil {
		return GuildVacations{}, err
	}
	out := GuildVacations{Guild: g}
	if out.Members, err = s.members.ListInServer(ctx, guildID); err != nil {
		return out, err
	}
	if s.vacations != nil {
		if out.Vacations, err = s.vacations.ForGuild(ctx, guildID, s.today(g)); err != nil {
			return out, err
		}
	}
	return out, nil
}

// SetGuildVacation sets a member's vacation in this guild only, for when they forgot to. The bot
// tells them by DM, unless the admin set their own.
func (s *Service) SetGuildVacation(ctx context.Context, v domain.Viewer, guildID, userID string, from, to time.Time) (domain.Vacation, error) {
	g, err := s.ViewerGuild(ctx, v, guildID, domain.AccessManage)
	if err != nil {
		return domain.Vacation{}, err
	}
	if s.vacations == nil {
		return domain.Vacation{}, errNoVacations
	}
	m, err := s.members.Get(ctx, guildID, userID)
	if err != nil {
		return domain.Vacation{}, err
	}
	if !m.InServer {
		return domain.Vacation{}, domain.ErrNotFound
	}
	vac := domain.Vacation{
		UserDiscordID: userID, GuildDiscordID: guildID, SetByDiscordID: v.DiscordID,
		From: domain.TruncateDay(from), To: domain.TruncateDay(to),
	}
	if err := vac.Validate(s.today(g)); err != nil {
		return domain.Vacation{}, err
	}
	saved, err := s.vacations.Set(ctx, vac, vac.SetByAdmin())
	saved.Username = m.Username
	return saved, err
}

// DeleteGuildVacation removes the vacation set for the member in this guild. A member's own
// vacation is theirs: admins can't remove it.
func (s *Service) DeleteGuildVacation(ctx context.Context, v domain.Viewer, guildID, userID string) error {
	if _, err := s.ViewerGuild(ctx, v, guildID, domain.AccessManage); err != nil {
		return err
	}
	if s.vacations == nil {
		return errNoVacations
	}
	return s.vacations.Delete(ctx, userID, guildID)
}
