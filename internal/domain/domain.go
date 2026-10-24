// Package domain holds HappyX's core entities and business rules.
// It has no dependencies on storage or transport.
package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrNotFound   = errors.New("not found")
	ErrInvalid    = errors.New("invalid input")
	ErrInactive   = errors.New("guild is inactive")
	ErrNotAMember = errors.New("user is not an active member of this guild")
	ErrTooMany    = errors.New("too many requests")
	// ErrClosed means the action isn't open right now, e.g. answering a game after Wednesday.
	ErrClosed = errors.New("not open now")
	// ErrGamesOff means the guild hasn't turned games on.
	ErrGamesOff = errors.New("games are turned off in this guild")
	// ErrRemoving means the guild was deleted from HappyX and the bot is about to leave it.
	ErrRemoving = errors.New("guild was deleted from HappyX")
	// ErrForbidden means the person can see the guild but not do this with it.
	ErrForbidden = errors.New("not allowed")
)

// ValidationError wraps ErrInvalid with a human readable reason.
type ValidationError struct{ Reason string }

func (e ValidationError) Error() string { return "invalid input: " + e.Reason }
func (e ValidationError) Unwrap() error { return ErrInvalid }

func invalid(format string, args ...any) error {
	return ValidationError{Reason: fmt.Sprintf(format, args...)}
}

// ReportFrequency controls how often the guild admin receives a report.
type ReportFrequency string

const (
	ReportWeekly  ReportFrequency = "weekly"
	ReportMonthly ReportFrequency = "monthly"
)

func (f ReportFrequency) Valid() bool { return f == ReportWeekly || f == ReportMonthly }

// Guild is a Discord server that added the bot. All other data hangs off a guild,
// which is the tenant boundary.
type Guild struct {
	DiscordID      string
	Name           string
	AdminDiscordID string
	Settings       GuildSettings
	Active         bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type GuildSettings struct {
	// Anonymous hides who answered what in reports. Defaults to true.
	Anonymous       bool
	Timezone        string
	SurveyHour      int // 0-23, local to Timezone
	ReportFrequency ReportFrequency
	// Language is the code of the language the bot speaks in this guild, e.g. "en" or "pt-BR".
	// The API only checks its shape; the bot decides which languages it supports.
	Language string
	// KudosChannelID is the channel where kudos cards are posted. Empty means the channel where
	// the command was used.
	KudosChannelID string
	// GamesEnabled turns on the weekly «Who's who?» game. Off by default.
	GamesEnabled bool
	// WorkingDays are the days the daily survey goes out. Monday to Friday by default.
	WorkingDays Weekdays
}

// Weekdays is a set of days of the week, one bit per time.Weekday (bit 0 is Sunday).
type Weekdays uint8

// MondayToFriday is the default working week.
const MondayToFriday Weekdays = 1<<time.Monday | 1<<time.Tuesday | 1<<time.Wednesday | 1<<time.Thursday | 1<<time.Friday

const allWeekdays Weekdays = 1<<7 - 1

// WeekdaysOf builds a set from ISO day numbers, 1 (Monday) to 7 (Sunday).
func WeekdaysOf(iso []int) (Weekdays, error) {
	var w Weekdays
	for _, n := range iso {
		if n < 1 || n > 7 {
			return 0, invalid("working_days must hold days 1 (Monday) to 7 (Sunday)")
		}
		w |= 1 << time.Weekday(n%7)
	}
	return w, nil
}

// Has reports whether d is in the set.
func (w Weekdays) Has(d time.Weekday) bool { return w&(1<<d) != 0 }

// ISO lists the set as ISO day numbers, Monday (1) first.
func (w Weekdays) ISO() []int {
	out := []int{}
	for n := 1; n <= 7; n++ {
		if w.Has(time.Weekday(n % 7)) {
			out = append(out, n)
		}
	}
	return out
}

// IsWorkingDay reports whether the survey goes out on day. Settings that never set their
// working days count as Monday to Friday.
func (s GuildSettings) IsWorkingDay(day time.Time) bool {
	w := s.WorkingDays
	if w == 0 {
		w = MondayToFriday
	}
	return w.Has(day.Weekday())
}

func DefaultGuildSettings() GuildSettings {
	return GuildSettings{
		Anonymous:       true,
		Timezone:        "Europe/Madrid",
		SurveyHour:      13,
		ReportFrequency: ReportWeekly,
		Language:        "en",
		WorkingDays:     MondayToFriday,
	}
}

func (s GuildSettings) Validate() error {
	if s.SurveyHour < 0 || s.SurveyHour > 23 {
		return invalid("survey_hour must be between 0 and 23")
	}
	if _, err := time.LoadLocation(s.Timezone); err != nil || s.Timezone == "" {
		return invalid("unknown timezone %q", s.Timezone)
	}
	if !validLanguage(s.Language) {
		return invalid("language must be a code like \"en\" or \"pt-BR\"")
	}
	if s.WorkingDays == 0 || s.WorkingDays&^allWeekdays != 0 {
		return invalid("working_days must have at least one day")
	}
	if !s.ReportFrequency.Valid() {
		return invalid("report_frequency must be %q or %q", ReportWeekly, ReportMonthly)
	}
	if s.KudosChannelID != "" {
		if err := validateSnowflake("kudos_channel_id", s.KudosChannelID); err != nil {
			return err
		}
	}
	return nil
}

func (g Guild) Validate() error {
	if err := validateSnowflake("discord_guild_id", g.DiscordID); err != nil {
		return err
	}
	if err := validateSnowflake("admin_discord_user_id", g.AdminDiscordID); err != nil {
		return err
	}
	if strings.TrimSpace(g.Name) == "" || utf8.RuneCountInString(g.Name) > 100 {
		return invalid("name must be 1-100 characters")
	}
	return g.Settings.Validate()
}

// DispatchKind is something the bot sends on a schedule.
type DispatchKind string

const (
	DispatchSurvey DispatchKind = "survey"
	DispatchReport DispatchKind = "report"
	// DispatchGame is the week's game question; period is the round's Monday.
	DispatchGame DispatchKind = "game"
	// DispatchGameGuess tells members guessing is open; period is the round's Monday.
	DispatchGameGuess DispatchKind = "game_guess"
)

func (k DispatchKind) Valid() bool {
	switch k {
	case DispatchSurvey, DispatchReport, DispatchGame, DispatchGameGuess:
		return true
	}
	return false
}

// Member is a human user of a guild who receives the daily survey.
type Member struct {
	GuildDiscordID string
	DiscordID      string
	Username       string
	Active         bool
	// RoleIDs are the member's Discord roles, as the bot last saw them. In an Upsert, nil keeps
	// the stored ones.
	RoleIDs []string
	// InServer is false once the member left the Discord server.
	InServer  bool
	CreatedAt time.Time
}

func (m Member) Validate() error {
	if err := validateSnowflake("discord_guild_id", m.GuildDiscordID); err != nil {
		return err
	}
	if err := validateSnowflake("discord_user_id", m.DiscordID); err != nil {
		return err
	}
	if utf8.RuneCountInString(m.Username) > 100 {
		return invalid("username must be at most 100 characters")
	}
	return ValidateRoleIDs(m.RoleIDs)
}

// Mood is the 1-5 answer to "how are you today?".
type Mood int

const (
	MoodAwful Mood = iota + 1
	MoodBad
	MoodNeutral
	MoodGood
	MoodGreat
)

func (m Mood) Valid() bool { return m >= MoodAwful && m <= MoodGreat }

const MaxMessageLength = 500

// MoodEntry is one member's answer for one day. A member has at most one entry per day;
// answering again replaces it.
type MoodEntry struct {
	GuildDiscordID  string
	MemberDiscordID string
	Day             time.Time // date only, UTC midnight
	Mood            Mood
	Message         string // optional
	// Anonymous records whether the guild was anonymous when the member answered. Such an
	// answer stays anonymous even if the guild later turns anonymity off.
	Anonymous bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (e MoodEntry) Validate() error {
	if err := validateSnowflake("discord_guild_id", e.GuildDiscordID); err != nil {
		return err
	}
	if err := validateSnowflake("discord_user_id", e.MemberDiscordID); err != nil {
		return err
	}
	if !e.Mood.Valid() {
		return invalid("mood must be between 1 and 5")
	}
	if utf8.RuneCountInString(e.Message) > MaxMessageLength {
		return invalid("message must be at most %d characters", MaxMessageLength)
	}
	if e.Day.IsZero() {
		return invalid("day is required")
	}
	return nil
}

// TruncateDay drops the time part, keeping the calendar date.
func TruncateDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// Discord IDs are snowflakes: unsigned 64-bit integers sent as strings.
func validateSnowflake(field, v string) error {
	if len(v) < 15 || len(v) > 20 {
		return invalid("%s must be a Discord snowflake", field)
	}
	for _, r := range v {
		if r < '0' || r > '9' {
			return invalid("%s must be a Discord snowflake", field)
		}
	}
	return nil
}

// validLanguage accepts "xx", "xxx" and "xx-YY" style codes.
func validLanguage(v string) bool {
	base, region, hasRegion := strings.Cut(v, "-")
	if len(base) < 2 || len(base) > 3 || strings.ToLower(base) != base || !isLetters(base) {
		return false
	}
	if hasRegion {
		return len(region) == 2 && strings.ToUpper(region) == region && isLetters(region)
	}
	return true
}

func isLetters(v string) bool {
	for _, r := range v {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') {
			return false
		}
	}
	return true
}
