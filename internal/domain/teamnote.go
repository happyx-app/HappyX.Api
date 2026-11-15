package domain

import (
	"strings"
	"time"
	"unicode/utf8"
)

// MaxTeamNoteLength bounds what an admin tells the team about a report. Discord fits more, but a
// note is a few lines the team reads in a channel.
const MaxTeamNoteLength = 1000

// TeamNote is what an admin told the team in a channel after a report: what they saw and what
// they'll do. The bot publishes the message and stores where it went, so the note can be edited
// or deleted later, and the next survey day's thanks card can point to it.
type TeamNote struct {
	ID             int64
	GuildDiscordID string
	// AuthorDiscordID is who published it; AuthorUsername their name, empty once they erased
	// their data (the note goes with it).
	AuthorDiscordID string
	AuthorUsername  string
	// From and To are the report's period the note is about.
	From, To time.Time
	Body     string
	// ChannelID and MessageID are where the bot published it.
	ChannelID string
	MessageID string
	// PostedOn is the day it was published, in the guild's timezone.
	PostedOn  time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (n TeamNote) Validate() error {
	if err := validateSnowflake("discord_guild_id", n.GuildDiscordID); err != nil {
		return err
	}
	if err := validateSnowflake("author_discord_id", n.AuthorDiscordID); err != nil {
		return err
	}
	if err := validateSnowflake("channel_id", n.ChannelID); err != nil {
		return err
	}
	if err := validateSnowflake("message_id", n.MessageID); err != nil {
		return err
	}
	if n.From.IsZero() || n.To.IsZero() || n.To.Before(n.From) {
		return invalid("from and to must be a period, from no later than to")
	}
	return ValidateTeamNoteBody(n.Body)
}

// ValidateTeamNoteBody checks the text of a note: 1 to MaxTeamNoteLength characters.
func ValidateTeamNoteBody(body string) error {
	if strings.TrimSpace(body) == "" || utf8.RuneCountInString(body) > MaxTeamNoteLength {
		return invalid("body must be 1-%d characters", MaxTeamNoteLength)
	}
	return nil
}

// PreviousWorkingDay is the last working day before day under s's working days. With no working
// day in the week before it, it is the day before.
func (s GuildSettings) PreviousWorkingDay(day time.Time) time.Time {
	day = TruncateDay(day)
	for i := 1; i <= 7; i++ {
		d := day.AddDate(0, 0, -i)
		if s.IsWorkingDay(d) {
			return d
		}
	}
	return day.AddDate(0, 0, -1)
}
