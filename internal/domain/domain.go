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
}

func DefaultGuildSettings() GuildSettings {
	return GuildSettings{
		Anonymous:       true,
		Timezone:        "Europe/Madrid",
		SurveyHour:      13,
		ReportFrequency: ReportWeekly,
	}
}

func (s GuildSettings) Validate() error {
	if s.SurveyHour < 0 || s.SurveyHour > 23 {
		return invalid("survey_hour must be between 0 and 23")
	}
	if _, err := time.LoadLocation(s.Timezone); err != nil || s.Timezone == "" {
		return invalid("unknown timezone %q", s.Timezone)
	}
	if !s.ReportFrequency.Valid() {
		return invalid("report_frequency must be %q or %q", ReportWeekly, ReportMonthly)
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

// Member is a human user of a guild who receives the daily survey.
type Member struct {
	GuildDiscordID string
	DiscordID      string
	Username       string
	Active         bool
	CreatedAt      time.Time
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
	return nil
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
	CreatedAt       time.Time
	UpdatedAt       time.Time
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
