package domain

import "testing"

func TestViewerCanView(t *testing.T) {
	g := Guild{DiscordID: "100000000000000001", AdminDiscordID: "300000000000000001"}
	cases := []struct {
		name string
		v    Viewer
		want bool
	}{
		{"registered admin", Viewer{DiscordID: "300000000000000001"}, true},
		{"server administrator", Viewer{DiscordID: "300000000000000002", ManagedGuilds: []string{"100000000000000001"}}, true},
		{"administrator elsewhere", Viewer{DiscordID: "300000000000000002", ManagedGuilds: []string{"100000000000000009"}}, false},
		{"plain member", Viewer{DiscordID: "300000000000000002"}, false},
		{"nobody", Viewer{}, false},
	}
	for _, c := range cases {
		if got := c.v.CanView(g); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}
