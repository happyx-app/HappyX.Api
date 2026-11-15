package usecase

import (
	"context"
	"strings"
	"time"

	"github.com/happyx-app/HappyX.Api/internal/domain"
)

// WithTeamNotes gives the Service the storage of what admins tell the team. Without it notes
// can't be published (domain.ErrNotFound) and there is never a current one.
func (s *Service) WithTeamNotes(t TeamNoteRepository) *Service {
	s.teamNotes = t
	return s
}

// PublishTeamNote records a note the bot has just published in a channel, as of today in the
// guild's timezone.
func (s *Service) PublishTeamNote(ctx context.Context, n domain.TeamNote) (domain.TeamNote, error) {
	if s.teamNotes == nil {
		return domain.TeamNote{}, domain.ErrNotFound
	}
	g, err := s.activeGuild(ctx, n.GuildDiscordID)
	if err != nil {
		return domain.TeamNote{}, err
	}
	n.Body = strings.TrimSpace(n.Body)
	n.From, n.To = domain.TruncateDay(n.From), domain.TruncateDay(n.To)
	n.PostedOn = s.today(g)
	if err := n.Validate(); err != nil {
		return domain.TeamNote{}, err
	}
	return s.teamNotes.Create(ctx, n)
}

func (s *Service) TeamNote(ctx context.Context, guildID string, id int64) (domain.TeamNote, error) {
	if s.teamNotes == nil {
		return domain.TeamNote{}, domain.ErrNotFound
	}
	return s.teamNotes.Get(ctx, guildID, id)
}

// EditTeamNote changes a note's text, once the bot has edited the message in the channel.
func (s *Service) EditTeamNote(ctx context.Context, guildID string, id int64, body string) (domain.TeamNote, error) {
	if s.teamNotes == nil {
		return domain.TeamNote{}, domain.ErrNotFound
	}
	body = strings.TrimSpace(body)
	if err := domain.ValidateTeamNoteBody(body); err != nil {
		return domain.TeamNote{}, err
	}
	return s.teamNotes.UpdateBody(ctx, guildID, id, body)
}

// DeleteTeamNote forgets a note, once the bot has deleted its message.
func (s *Service) DeleteTeamNote(ctx context.Context, guildID string, id int64) error {
	if s.teamNotes == nil {
		return domain.ErrNotFound
	}
	return s.teamNotes.Delete(ctx, guildID, id)
}

// CurrentTeamNote is the note the thanks card points to on day: the last one published since
// the previous working day, before day itself. A note is news on the first survey day after it.
func (s *Service) CurrentTeamNote(ctx context.Context, guildID string, day time.Time) (domain.TeamNote, error) {
	if s.teamNotes == nil {
		return domain.TeamNote{}, domain.ErrNotFound
	}
	g, err := s.activeGuild(ctx, guildID)
	if err != nil {
		return domain.TeamNote{}, err
	}
	day = domain.TruncateDay(day)
	return s.teamNotes.Latest(ctx, guildID, g.Settings.PreviousWorkingDay(day), day)
}
