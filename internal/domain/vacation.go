package domain

import (
	"fmt"
	"time"
)

// Vacation is a stretch of days, both included, when someone doesn't get the survey or the game.
// Those days don't count against their participation or their streak.
//
// A person sets their own for every server they are in (GuildDiscordID is ""). Someone who manages
// a server can set one for a member there, in case they forgot (GuildDiscordID is that server);
// the bot then tells the member by DM. Both kinds apply together.
type Vacation struct {
	ID             int64
	UserDiscordID  string
	GuildDiscordID string // "" for the person's own, in every server
	GuildName      string // filled in when reading
	From, To       time.Time
	SetByDiscordID string
	// Username is the member's, filled in when listing a server's vacations.
	Username  string
	CreatedAt time.Time
}

// MaxVacationDays is the longest vacation, counting both ends. A longer absence is what pausing
// the survey is for.
const MaxVacationDays = 90

// MaxVacationLead is how far ahead a vacation may start.
const MaxVacationLead = 365

// VacationRetention is how long a vacation is kept after it ends. A streak is counted over that
// long, and the vacation's days must keep not breaking it.
const VacationRetention = 400

// Validate checks the dates against today: a vacation that already ended can't be set.
func (v Vacation) Validate(today time.Time) error {
	if err := validateSnowflake("discord_user_id", v.UserDiscordID); err != nil {
		return err
	}
	if v.GuildDiscordID != "" {
		if err := validateSnowflake("discord_guild_id", v.GuildDiscordID); err != nil {
			return err
		}
	}
	from, to, today := TruncateDay(v.From), TruncateDay(v.To), TruncateDay(today)
	switch {
	case from.IsZero() || to.IsZero():
		return invalid("from and to are required")
	case to.Before(from):
		return invalid("to must not be before from")
	case to.Before(today):
		return invalid("the vacation has already ended")
	case to.Sub(from) >= MaxVacationDays*24*time.Hour:
		return ValidationError{Reason: fmt.Sprintf("a vacation can be at most %d days", MaxVacationDays)}
	case from.After(today.AddDate(0, 0, MaxVacationLead)):
		return ValidationError{Reason: fmt.Sprintf("a vacation can start at most %d days ahead", MaxVacationLead)}
	}
	return nil
}

// Covers reports whether day is one of the vacation's days.
func (v Vacation) Covers(day time.Time) bool {
	day = TruncateDay(day)
	return !day.Before(TruncateDay(v.From)) && !day.After(TruncateDay(v.To))
}

// SetByAdmin reports whether someone else set it, which only a server's admins can do.
func (v Vacation) SetByAdmin() bool {
	return v.SetByDiscordID != "" && v.SetByDiscordID != v.UserDiscordID
}

// Vacations is a person's vacations that apply in one server.
type Vacations []Vacation

// Covers reports whether any of them covers day.
func (vs Vacations) Covers(day time.Time) bool {
	for _, v := range vs {
		if v.Covers(day) {
			return true
		}
	}
	return false
}
