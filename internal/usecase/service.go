package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/moixblau/HappyX.Api/internal/domain"
)

type Service struct {
	guilds     GuildRepository
	members    MemberRepository
	moods      MoodRepository
	dispatches DispatchRepository
	now        Clock
}

func NewService(g GuildRepository, m MemberRepository, mo MoodRepository, d DispatchRepository, now Clock) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{guilds: g, members: m, moods: mo, dispatches: d, now: now}
}

// --- Guilds ---

type RegisterGuildInput struct {
	DiscordID      string
	Name           string
	AdminDiscordID string
}

// RegisterGuild is called when the bot joins a server (or on startup, for servers it is in).
func (s *Service) RegisterGuild(ctx context.Context, in RegisterGuildInput) (domain.Guild, error) {
	g := domain.Guild{
		DiscordID:      in.DiscordID,
		Name:           in.Name,
		AdminDiscordID: in.AdminDiscordID,
		Settings:       domain.DefaultGuildSettings(),
		Active:         true,
	}
	if err := g.Validate(); err != nil {
		return domain.Guild{}, err
	}
	return s.guilds.Upsert(ctx, g)
}

func (s *Service) GetGuild(ctx context.Context, guildID string) (domain.Guild, error) {
	return s.guilds.Get(ctx, guildID)
}

func (s *Service) ListActiveGuilds(ctx context.Context) ([]domain.Guild, error) {
	return s.guilds.ListActive(ctx)
}

// SettingsPatch carries optional changes; nil fields are left untouched.
type SettingsPatch struct {
	Anonymous       *bool
	Timezone        *string
	Language        *string
	SurveyHour      *int
	ReportFrequency *domain.ReportFrequency
	AdminDiscordID  *string
}

func (s *Service) UpdateGuildSettings(ctx context.Context, guildID string, p SettingsPatch) (domain.Guild, error) {
	g, err := s.activeGuild(ctx, guildID)
	if err != nil {
		return domain.Guild{}, err
	}
	st := g.Settings
	if p.Anonymous != nil {
		st.Anonymous = *p.Anonymous
	}
	if p.Timezone != nil {
		st.Timezone = *p.Timezone
	}
	if p.Language != nil {
		st.Language = *p.Language
	}
	if p.SurveyHour != nil {
		st.SurveyHour = *p.SurveyHour
	}
	if p.ReportFrequency != nil {
		st.ReportFrequency = *p.ReportFrequency
	}
	if p.AdminDiscordID != nil {
		g.AdminDiscordID = *p.AdminDiscordID
	}
	g.Settings = st
	if err := g.Validate(); err != nil {
		return domain.Guild{}, err
	}
	return s.guilds.UpdateConfig(ctx, guildID, g.AdminDiscordID, st)
}

// RemoveGuild is called when the bot is kicked. Data is kept but the guild stops
// receiving surveys and reports.
func (s *Service) RemoveGuild(ctx context.Context, guildID string) error {
	return s.guilds.Deactivate(ctx, guildID)
}

// --- Members ---

// UpsertMember registers a guild member or refreshes their username. With active nil an
// existing member keeps their opt-in state (used when syncing the member list); true opts
// them in (they joined the server or asked to receive the survey again).
func (s *Service) UpsertMember(ctx context.Context, guildID, userID, username string, active *bool) (domain.Member, error) {
	if _, err := s.activeGuild(ctx, guildID); err != nil {
		return domain.Member{}, err
	}
	m := domain.Member{GuildDiscordID: guildID, DiscordID: userID, Username: username}
	if err := m.Validate(); err != nil {
		return domain.Member{}, err
	}
	return s.members.Upsert(ctx, m, active)
}

func (s *Service) LeaveMember(ctx context.Context, guildID, userID string) error {
	return s.members.Deactivate(ctx, guildID, userID)
}

func (s *Service) ListMembers(ctx context.Context, guildID string) ([]domain.Member, error) {
	if _, err := s.activeGuild(ctx, guildID); err != nil {
		return nil, err
	}
	return s.members.ListActive(ctx, guildID)
}

// --- Moods ---

type RecordMoodInput struct {
	GuildID string
	UserID  string
	Mood    domain.Mood
	Message string
	// Day is optional; when zero, today in the guild's timezone is used.
	Day time.Time
}

func (s *Service) RecordMood(ctx context.Context, in RecordMoodInput) (domain.MoodEntry, error) {
	g, err := s.activeGuild(ctx, in.GuildID)
	if err != nil {
		return domain.MoodEntry{}, err
	}
	m, err := s.members.Get(ctx, in.GuildID, in.UserID)
	if err != nil || !m.Active {
		if err == nil || err == domain.ErrNotFound {
			return domain.MoodEntry{}, domain.ErrNotAMember
		}
		return domain.MoodEntry{}, err
	}
	today := s.today(g)
	day := in.Day
	if day.IsZero() {
		day = today
	}
	day = domain.TruncateDay(day)
	// Late answers to yesterday's survey are fine; the future is not.
	if day.After(today) {
		return domain.MoodEntry{}, domain.ValidationError{Reason: "day cannot be in the future"}
	}
	e := domain.MoodEntry{
		GuildDiscordID:  in.GuildID,
		MemberDiscordID: in.UserID,
		Day:             day,
		Mood:            in.Mood,
		Message:         in.Message,
	}
	if err := e.Validate(); err != nil {
		return domain.MoodEntry{}, err
	}
	return s.moods.Upsert(ctx, e)
}

// --- Dispatches ---

// ClaimDispatch lets the bot reserve sending a survey or report for a period exactly once.
// It returns false when it was already sent.
func (s *Service) ClaimDispatch(ctx context.Context, guildID string, kind domain.DispatchKind, period time.Time) (bool, error) {
	if !kind.Valid() {
		return false, domain.ValidationError{Reason: "kind must be \"survey\" or \"report\""}
	}
	if period.IsZero() {
		return false, domain.ValidationError{Reason: "period is required"}
	}
	if _, err := s.activeGuild(ctx, guildID); err != nil {
		return false, err
	}
	return s.dispatches.Claim(ctx, guildID, kind, domain.TruncateDay(period))
}

// --- Reports ---

const maxReportDays = 366

func (s *Service) Report(ctx context.Context, guildID string, from, to time.Time) (domain.Report, error) {
	g, err := s.guilds.Get(ctx, guildID)
	if err != nil {
		return domain.Report{}, err
	}
	from, to = domain.TruncateDay(from), domain.TruncateDay(to)
	if to.Before(from) {
		return domain.Report{}, domain.ValidationError{Reason: "to must not be before from"}
	}
	if to.Sub(from) > maxReportDays*24*time.Hour {
		return domain.Report{}, domain.ValidationError{Reason: fmt.Sprintf("range must be at most %d days", maxReportDays)}
	}
	entries, err := s.moods.ListBetween(ctx, guildID, from, to)
	if err != nil {
		return domain.Report{}, err
	}
	usernames := map[string]string{}
	if !g.Settings.Anonymous {
		ms, err := s.members.ListActive(ctx, guildID)
		if err != nil {
			return domain.Report{}, err
		}
		for _, m := range ms {
			usernames[m.DiscordID] = m.Username
		}
	}
	return domain.BuildReport(g, from, to, entries, usernames), nil
}

// --- Web dashboard ---

// VisibleGuilds lists the guilds the viewer may see on the dashboard.
func (s *Service) VisibleGuilds(ctx context.Context, v domain.Viewer) ([]domain.Guild, error) {
	if v.DiscordID == "" {
		return nil, nil
	}
	gs, err := s.guilds.ListVisible(ctx, v.DiscordID, v.ManagedGuilds)
	if err != nil {
		return nil, err
	}
	out := gs[:0]
	for _, g := range gs {
		if v.CanView(g) {
			out = append(out, g)
		}
	}
	return out, nil
}

// ViewerGuild returns the guild if the viewer may see it. Guilds they can't see are reported
// as not found, so the dashboard doesn't reveal which servers use HappyX.
func (s *Service) ViewerGuild(ctx context.Context, v domain.Viewer, guildID string) (domain.Guild, error) {
	g, err := s.guilds.Get(ctx, guildID)
	if err != nil {
		return domain.Guild{}, err
	}
	if !v.CanView(g) {
		return domain.Guild{}, domain.ErrNotFound
	}
	return g, nil
}

// LastPeriod returns the most recent complete week (Mon-Sun) or month before now,
// in the guild's timezone.
func (s *Service) LastPeriod(g domain.Guild) (from, to time.Time) {
	today := s.today(g)
	if g.Settings.ReportFrequency == domain.ReportMonthly {
		firstThisMonth := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC)
		return firstThisMonth.AddDate(0, -1, 0), firstThisMonth.AddDate(0, 0, -1)
	}
	offset := (int(today.Weekday()) + 6) % 7 // days since Monday
	thisMonday := today.AddDate(0, 0, -offset)
	return thisMonday.AddDate(0, 0, -7), thisMonday.AddDate(0, 0, -1)
}

func (s *Service) today(g domain.Guild) time.Time {
	loc, err := time.LoadLocation(g.Settings.Timezone)
	if err != nil {
		loc = time.UTC
	}
	return domain.TruncateDay(s.now().In(loc))
}

func (s *Service) activeGuild(ctx context.Context, guildID string) (domain.Guild, error) {
	g, err := s.guilds.Get(ctx, guildID)
	if err != nil {
		return domain.Guild{}, err
	}
	if !g.Active {
		return domain.Guild{}, domain.ErrInactive
	}
	return g, nil
}
