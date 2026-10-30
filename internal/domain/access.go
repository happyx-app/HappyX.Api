package domain

import (
	"strings"
	"unicode/utf8"
)

// AccessLevel is what someone may do with a guild on the dashboard and with the bot's admin
// commands. Each level includes the ones below it.
type AccessLevel int

const (
	AccessNone AccessLevel = iota
	// AccessView sees the dashboard and the reports.
	AccessView
	// AccessManage also changes the survey settings.
	AccessManage
	// AccessOwner is the main admin, who receives the reports, and the server's owner and
	// Discord Administrators. Only they choose who else has access, and delete the server.
	AccessOwner
)

func (l AccessLevel) String() string {
	switch l {
	case AccessView:
		return "view"
	case AccessManage:
		return "manage"
	case AccessOwner:
		return "owner"
	}
	return "none"
}

// ParseGrantLevel reads a level that can be granted: "view" or "manage".
func ParseGrantLevel(s string) (AccessLevel, error) {
	switch s {
	case "view":
		return AccessView, nil
	case "manage":
		return AccessManage, nil
	}
	return AccessNone, invalid("level must be \"view\" or \"manage\"")
}

// GrantKind says whether a grant names a Discord user or a Discord role.
type GrantKind string

const (
	GrantUser GrantKind = "user"
	GrantRole GrantKind = "role"
)

// AccessGrant gives a user, or everyone with a role, extra access to a guild.
type AccessGrant struct {
	Kind     GrantKind
	TargetID string
	Level    AccessLevel
}

// MaxAccessGrants keeps the list short enough to read at a glance.
const MaxAccessGrants = 50

// ValidateGrants checks a guild's full list of grants.
func ValidateGrants(gs []AccessGrant) error {
	if len(gs) > MaxAccessGrants {
		return invalid("at most %d people or roles can have extra access", MaxAccessGrants)
	}
	seen := map[string]bool{}
	for _, g := range gs {
		if g.Kind != GrantUser && g.Kind != GrantRole {
			return invalid("kind must be %q or %q", GrantUser, GrantRole)
		}
		if err := validateSnowflake("id", g.TargetID); err != nil {
			return err
		}
		if g.Level != AccessView && g.Level != AccessManage {
			return invalid("level must be \"view\" or \"manage\"")
		}
		k := string(g.Kind) + ":" + g.TargetID
		if seen[k] {
			return invalid("%s %s is listed twice", g.Kind, g.TargetID)
		}
		seen[k] = true
	}
	return nil
}

// Role is a Discord role of a guild, as the bot last saw it.
type Role struct {
	DiscordID string
	Name      string
	Color     int // RGB, 0 means no colour
	Position  int // higher is listed first, as in Discord
}

func (r Role) Validate() error {
	if err := validateSnowflake("role id", r.DiscordID); err != nil {
		return err
	}
	if strings.TrimSpace(r.Name) == "" || utf8.RuneCountInString(r.Name) > 100 {
		return invalid("role name must be 1-100 characters")
	}
	if r.Color < 0 || r.Color > 0xFFFFFF {
		return invalid("role color must be an RGB value")
	}
	return nil
}

// MaxRoleIDs is Discord's limit on roles per server, which bounds a member's roles too.
const MaxRoleIDs = 250

func ValidateRoleIDs(ids []string) error {
	if len(ids) > MaxRoleIDs {
		return invalid("at most %d roles", MaxRoleIDs)
	}
	for _, id := range ids {
		if err := validateSnowflake("role id", id); err != nil {
			return err
		}
	}
	return nil
}

// ResolveAccess works out userID's level in g. managesInDiscord is true when they own the
// server or have Discord's Administrator permission in it; roleIDs are their roles in it.
func ResolveAccess(g Guild, userID string, managesInDiscord bool, roleIDs []string, grants []AccessGrant) AccessLevel {
	if userID == "" {
		return AccessNone
	}
	if g.AdminDiscordID == userID || managesInDiscord {
		return AccessOwner
	}
	has := map[string]bool{}
	for _, id := range roleIDs {
		has[id] = true
	}
	level := AccessNone
	for _, gr := range grants {
		match := (gr.Kind == GrantUser && gr.TargetID == userID) || (gr.Kind == GrantRole && has[gr.TargetID])
		if match && gr.Level > level {
			level = gr.Level
		}
	}
	return level
}

// Channel is a text or announcement channel of a guild, as the bot last saw it.
type Channel struct {
	DiscordID string
	Name      string
	Category  string // the category it is in, empty for none
	Position  int    // lower is listed first, as in Discord's channel list
	CanPost   bool   // whether the bot may post (and embed) there
}

func (c Channel) Validate() error {
	if err := validateSnowflake("channel id", c.DiscordID); err != nil {
		return err
	}
	if strings.TrimSpace(c.Name) == "" || utf8.RuneCountInString(c.Name) > 100 {
		return invalid("channel name must be 1-100 characters")
	}
	if utf8.RuneCountInString(c.Category) > 100 {
		return invalid("channel category must be at most 100 characters")
	}
	return nil
}

// MaxChannels is Discord's limit on channels per server.
const MaxChannels = 500
