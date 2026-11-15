package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/happyx-app/HappyX.Api/internal/domain"
)

// WithChannelPosts gives the Service the storage of the messages HappyX keeps in a channel.
// Without it there are none (domain.ErrNotFound).
func (s *Service) WithChannelPosts(p ChannelPostRepository) *Service {
	s.posts = p
	return s
}

func (s *Service) ChannelPost(ctx context.Context, guildID, kind string) (domain.ChannelPost, error) {
	if s.posts == nil || !domain.ValidChannelPostKind(kind) {
		return domain.ChannelPost{}, domain.ErrNotFound
	}
	return s.posts.Get(ctx, guildID, kind)
}

// PutChannelPost records where the bot has just published the guild's post of that kind.
func (s *Service) PutChannelPost(ctx context.Context, p domain.ChannelPost) (domain.ChannelPost, error) {
	if s.posts == nil {
		return domain.ChannelPost{}, domain.ErrNotFound
	}
	if err := p.Validate(); err != nil {
		return domain.ChannelPost{}, err
	}
	if _, err := s.activeGuild(ctx, p.GuildDiscordID); err != nil {
		return domain.ChannelPost{}, err
	}
	return s.posts.Put(ctx, p)
}

// DeleteChannelPost forgets the guild's post of that kind, once it is gone from the channel.
func (s *Service) DeleteChannelPost(ctx context.Context, guildID, kind string) error {
	if s.posts == nil || !domain.ValidChannelPostKind(kind) {
		return domain.ErrNotFound
	}
	return s.posts.Delete(ctx, guildID, kind)
}

// maxMemberMoodDays bounds what the bot reads of one member's answers at a time: it shows
// them a week.
const maxMemberMoodDays = 31

// MemberMoods returns the member's own answers in the guild from from to to, oldest first. The
// bot shows them only to that member: their week on the thanks card, or today's answer when
// they answer from a channel.
func (s *Service) MemberMoods(ctx context.Context, guildID, userID string, from, to time.Time) ([]domain.MoodEntry, error) {
	if _, err := s.activeGuild(ctx, guildID); err != nil {
		return nil, err
	}
	from, to = domain.TruncateDay(from), domain.TruncateDay(to)
	if to.Before(from) {
		return nil, domain.ValidationError{Reason: "to must not be before from"}
	}
	if to.Sub(from) >= maxMemberMoodDays*24*time.Hour {
		return nil, domain.ValidationError{Reason: fmt.Sprintf("range must be at most %d days", maxMemberMoodDays)}
	}
	if _, err := s.members.Get(ctx, guildID, userID); err != nil {
		return nil, err
	}
	entries, err := s.moods.ListForUser(ctx, userID, from, to)
	if err != nil {
		return nil, err
	}
	out := make([]domain.MoodEntry, 0, len(entries))
	for _, e := range entries {
		if e.GuildDiscordID == guildID {
			out = append(out, e)
		}
	}
	return out, nil
}

// UserGuild is a server the user is in, and whether they paused the survey there.
type UserGuild struct {
	Guild  domain.Guild
	Paused bool
}

// UserGuilds returns the active guilds where the user is in the Discord server, whether or not
// they get the survey there: what /happyx in a DM with the bot can be about.
func (s *Service) UserGuilds(ctx context.Context, userID string) ([]UserGuild, error) {
	if err := domain.ValidateDiscordID("discord_user_id", userID); err != nil {
		return nil, err
	}
	in, err := s.guilds.ListInServer(ctx, userID)
	if err != nil {
		return nil, err
	}
	joined, err := s.guilds.ListJoined(ctx, userID)
	if err != nil {
		return nil, err
	}
	taking := make(map[string]bool, len(joined))
	for _, g := range joined {
		taking[g.DiscordID] = true
	}
	out := make([]UserGuild, len(in))
	for n, g := range in {
		out[n] = UserGuild{Guild: g, Paused: !taking[g.DiscordID]}
	}
	return out, nil
}
