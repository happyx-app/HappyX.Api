package domain

import (
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// MaxTeamEventTitle and MaxTeamEventDescription bound what fits a bookmark's tooltip.
	MaxTeamEventTitle       = 60
	MaxTeamEventDescription = 280
	// MaxTeamEvents bounds how many events a server keeps.
	MaxTeamEvents = 1000
)

// TeamEventColors are the colours a bookmark can have, by name: the dashboard draws each so it
// reads in light and dark themes, and none is the chart line's violet. The first is the default.
var TeamEventColors = []string{"yellow", "orange", "red", "green", "blue", "pink"}

// TeamEvent is something that happened to a server's team over one or more days (From = To for
// one), marked on the mood chart so a change in mood can be read against it. Who marked it is
// not kept: an event is the server's, not a person's.
type TeamEvent struct {
	ID             int64
	GuildDiscordID string
	From, To       time.Time
	Title          string
	Description    string
	Color          string // one of TeamEventColors
}

// Normalize trims the texts and truncates the dates to days.
func (e TeamEvent) Normalize() TeamEvent {
	e.Title = strings.TrimSpace(e.Title)
	e.Description = strings.TrimSpace(e.Description)
	e.From, e.To = TruncateDay(e.From), TruncateDay(e.To)
	if e.Color == "" {
		e.Color = TeamEventColors[0]
	}
	return e
}

// Validate checks a normalized event. Events are about what already happened: the chart never
// reaches past today, so one that ends later would show nowhere.
func (e TeamEvent) Validate(today time.Time) error {
	switch {
	case e.Title == "":
		return invalid("title is required")
	case utf8.RuneCountInString(e.Title) > MaxTeamEventTitle:
		return invalid("title must be at most %d characters", MaxTeamEventTitle)
	case utf8.RuneCountInString(e.Description) > MaxTeamEventDescription:
		return invalid("description must be at most %d characters", MaxTeamEventDescription)
	case !slices.Contains(TeamEventColors, e.Color):
		return invalid("color must be one of %s", strings.Join(TeamEventColors, ", "))
	case e.From.IsZero() || e.To.IsZero():
		return invalid("from and to are required")
	case e.To.Before(e.From):
		return invalid("to must not be before from")
	case e.To.After(TruncateDay(today)):
		return invalid("events can't end after today")
	}
	return nil
}
