package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/happyx-app/HappyX.Api/internal/domain"
)

// WithTeamEvents gives the Service the storage of the events marked on the chart. Without it no
// server has events and marking one is domain.ErrNotFound.
func (s *Service) WithTeamEvents(r TeamEventRepository) *Service {
	s.events = r
	return s
}

// TeamEvents returns the guild's events that touch [from, to], for whoever can see its dashboard.
func (s *Service) TeamEvents(ctx context.Context, v domain.Viewer, guildID string, from, to time.Time) ([]domain.TeamEvent, error) {
	if _, err := s.ViewerGuild(ctx, v, guildID, domain.AccessView); err != nil {
		return nil, err
	}
	from, to = domain.TruncateDay(from), domain.TruncateDay(to)
	if to.Before(from) {
		return nil, domain.ValidationError{Reason: "to must not be before from"}
	}
	if to.Sub(from) > maxReportDays*24*time.Hour {
		return nil, domain.ValidationError{Reason: fmt.Sprintf("range must be at most %d days", maxReportDays)}
	}
	if s.events == nil {
		return []domain.TeamEvent{}, nil
	}
	return s.events.ForGuild(ctx, guildID, from, to)
}

// CreateTeamEvent marks an event on the guild's chart. It needs the "manage" level.
func (s *Service) CreateTeamEvent(ctx context.Context, v domain.Viewer, guildID string, e domain.TeamEvent) (domain.TeamEvent, error) {
	g, err := s.ViewerGuild(ctx, v, guildID, domain.AccessManage)
	if err != nil {
		return domain.TeamEvent{}, err
	}
	if s.events == nil {
		return domain.TeamEvent{}, domain.ErrNotFound
	}
	e = e.Normalize()
	e.GuildDiscordID = guildID
	if err := e.Validate(s.today(g)); err != nil {
		return domain.TeamEvent{}, err
	}
	n, err := s.events.Count(ctx, guildID)
	if err != nil {
		return domain.TeamEvent{}, err
	}
	if n >= domain.MaxTeamEvents {
		return domain.TeamEvent{}, domain.ValidationError{Reason: fmt.Sprintf("a server keeps at most %d events", domain.MaxTeamEvents)}
	}
	return s.events.Create(ctx, e)
}

// UpdateTeamEvent changes one of the guild's events. It needs the "manage" level.
func (s *Service) UpdateTeamEvent(ctx context.Context, v domain.Viewer, guildID string, e domain.TeamEvent) (domain.TeamEvent, error) {
	g, err := s.ViewerGuild(ctx, v, guildID, domain.AccessManage)
	if err != nil {
		return domain.TeamEvent{}, err
	}
	if s.events == nil {
		return domain.TeamEvent{}, domain.ErrNotFound
	}
	e = e.Normalize()
	e.GuildDiscordID = guildID
	if err := e.Validate(s.today(g)); err != nil {
		return domain.TeamEvent{}, err
	}
	return s.events.Update(ctx, e)
}

// DeleteTeamEvent removes one of the guild's events. It needs the "manage" level.
func (s *Service) DeleteTeamEvent(ctx context.Context, v domain.Viewer, guildID string, id int64) error {
	if _, err := s.ViewerGuild(ctx, v, guildID, domain.AccessManage); err != nil {
		return err
	}
	if s.events == nil {
		return domain.ErrNotFound
	}
	return s.events.Delete(ctx, guildID, id)
}
