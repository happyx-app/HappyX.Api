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

// CanView reports whether the viewer may see the guild's moods and reports: they are the admin
// the bot sends reports to, or an owner or Administrator of the server.
func (v Viewer) CanView(g Guild) bool {
	if v.DiscordID == "" {
		return false
	}
	if g.AdminDiscordID == v.DiscordID {
		return true
	}
	for _, id := range v.ManagedGuilds {
		if id == g.DiscordID {
			return true
		}
	}
	return false
}
