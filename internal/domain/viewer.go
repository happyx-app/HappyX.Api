package domain

// Viewer is a person logged in to the web dashboard with their Discord account.
type Viewer struct {
	DiscordID string
	Username  string
	Avatar    string // Discord avatar hash, may be empty
	// ManagedGuilds are the Discord IDs of the servers the viewer owns or has the Administrator
	// permission in, as Discord reported them at login.
	ManagedGuilds []string
}

// ManagesInDiscord reports whether the viewer owns the guild or is one of its Discord
// Administrators.
func (v Viewer) ManagesInDiscord(guildID string) bool {
	for _, id := range v.ManagedGuilds {
		if id == guildID {
			return true
		}
	}
	return false
}
