package domain

import (
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MaxKudosLength = 300
	// MaxKudosPerDay stops one member from flooding the channel.
	MaxKudosPerDay = 10
)

// Kudos is public thanks from one member of a guild to another. Unlike moods, kudos are never
// anonymous: they are posted in the server with both names.
type Kudos struct {
	GuildDiscordID    string
	GiverDiscordID    string
	GiverUsername     string
	ReceiverDiscordID string
	ReceiverUsername  string
	Day               time.Time // date only, in the guild's timezone
	Message           string
	CreatedAt         time.Time
}

func (k Kudos) Validate() error {
	if err := validateSnowflake("discord_guild_id", k.GuildDiscordID); err != nil {
		return err
	}
	if err := validateSnowflake("from_discord_user_id", k.GiverDiscordID); err != nil {
		return err
	}
	if err := validateSnowflake("to_discord_user_id", k.ReceiverDiscordID); err != nil {
		return err
	}
	if k.GiverDiscordID == k.ReceiverDiscordID {
		return invalid("you can't give kudos to yourself")
	}
	if strings.TrimSpace(k.Message) == "" || utf8.RuneCountInString(k.Message) > MaxKudosLength {
		return invalid("message must be 1-%d characters", MaxKudosLength)
	}
	if k.Day.IsZero() {
		return invalid("day is required")
	}
	return nil
}
