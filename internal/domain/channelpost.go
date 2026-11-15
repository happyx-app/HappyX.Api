package domain

import "time"

// ChannelPostKinds are the messages HappyX keeps in a channel, one of each per guild. «survey»
// is the daily survey for whoever doesn't get HappyX's DMs: its button answers it privately.
var ChannelPostKinds = []string{"survey"}

// ChannelPost is where the bot published one of those messages, so it edits it or checks it is
// still there instead of posting it again.
type ChannelPost struct {
	GuildDiscordID string
	Kind           string
	ChannelID      string
	MessageID      string
	CreatedAt      time.Time
}

// ValidChannelPostKind reports whether kind is one of ChannelPostKinds.
func ValidChannelPostKind(kind string) bool {
	for _, k := range ChannelPostKinds {
		if k == kind {
			return true
		}
	}
	return false
}

func (p ChannelPost) Validate() error {
	if !ValidChannelPostKind(p.Kind) {
		return invalid("kind must be one of %v", ChannelPostKinds)
	}
	if err := validateSnowflake("channel_id", p.ChannelID); err != nil {
		return err
	}
	return validateSnowflake("message_id", p.MessageID)
}

// ValidateDiscordID checks a Discord ID (a snowflake) given as field.
func ValidateDiscordID(field, v string) error { return validateSnowflake(field, v) }
