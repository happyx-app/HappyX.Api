package http_test

import (
	"encoding/json"
	"net/http"
	"testing"

	httpapi "github.com/happyx-app/HappyX.Api/internal/transport/http"
)

// TestKudosChannelFromWeb checks the bot's channels reach the dashboard in Discord's order, that
// the kudos channel can be chosen from them or cleared, and that deleting it clears it.
func TestKudosChannelFromWeb(t *testing.T) {
	srv := newTestServer(t, func(s *httpapi.Server) {
		s.WithWeb(httpapi.Web{Identity: fakeDiscord{}, SessionKey: []byte("0123456789abcdef0123456789abcdef"), PublicURL: "http://happyx.test/"})
	})
	const general, kudos, readOnly, voice = "500000000000000001", "500000000000000002", "500000000000000003", "500000000000000099"
	if code, out := do(t, srv, "POST", "/v1/guilds", key, `{"discord_guild_id":"`+guildA+`","name":"A","admin_discord_user_id":"`+adminA+`"}`); code != 200 {
		t.Fatalf("register: %d %v", code, out)
	}
	owner := newWebClient(t)
	login(t, srv, owner, "adminA")

	// Before the bot sends any channel (an older bot), the bot's command still sets one.
	if code, g := do(t, srv, "PATCH", "/v1/guilds/"+guildA, key, `{"kudos_channel_id":"`+voice+`"}`); code != 200 || g["kudos_channel_id"] != voice {
		t.Fatalf("kudos channel before any sync: %d %v", code, g)
	}

	syncChannels := func(channels string) {
		t.Helper()
		if code, body := do(t, srv, "PUT", "/v1/guilds/"+guildA+"/channels", key, `{"channels":[`+channels+`]}`); code != 204 {
			t.Fatalf("sync channels: %d %v", code, body)
		}
	}
	syncChannels(`{"id":"` + kudos + `","name":"kudos","category":"Team","position":2,"can_post":true},` +
		`{"id":"` + general + `","name":"general","position":0,"can_post":true},` +
		`{"id":"` + readOnly + `","name":"announcements","position":1,"can_post":false}`)
	if code, g := do(t, srv, "GET", "/v1/guilds/"+guildA, key, ""); code != 200 || g["kudos_channel_id"] != "" {
		t.Fatalf("a kudos channel that isn't in the server is cleared: %d %v", code, g)
	}
	if code, body := do(t, srv, "PUT", "/v1/guilds/"+guildA+"/channels", key, `{"channels":[{"id":"x","name":"bad","position":0}]}`); code != 400 {
		t.Fatalf("bad channel id: %d %v", code, body)
	}

	res, err := owner.Get(srv.URL + "/web/guilds/" + guildA + "/channels")
	if err != nil {
		t.Fatal(err)
	}
	var channels []struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Category string `json:"category"`
		CanPost  bool   `json:"can_post"`
	}
	err = json.NewDecoder(res.Body).Decode(&channels)
	res.Body.Close()
	if err != nil || res.StatusCode != 200 || len(channels) != 3 || channels[0].ID != general || channels[1].CanPost || channels[2].Category != "Team" {
		t.Fatalf("channels: %d %v %+v", res.StatusCode, err, channels)
	}

	if code, _ := patchSettings(t, srv, owner, guildA, "application/json", `{"kudos_channel_id":"500000000000000077"}`); code != 400 {
		t.Fatalf("unknown channel: %d, want 400", code)
	}
	if code, g := patchSettings(t, srv, owner, guildA, "application/json", `{"kudos_channel_id":"`+kudos+`"}`); code != 200 || g["kudos_channel_id"] != kudos {
		t.Fatalf("set kudos channel: %d %v", code, g)
	}
	if code, g := patchSettings(t, srv, owner, guildA, "application/json", `{"kudos_channel_id":""}`); code != 200 || g["kudos_channel_id"] != "" {
		t.Fatalf("clear kudos channel: %d %v", code, g)
	}

	// Losing the permission to post keeps the choice; deleting the channel clears it.
	patchSettings(t, srv, owner, guildA, "application/json", `{"kudos_channel_id":"`+kudos+`"}`)
	syncChannels(`{"id":"` + kudos + `","name":"kudos","position":2,"can_post":false},{"id":"` + general + `","name":"general","position":0,"can_post":true}`)
	if _, g := do(t, srv, "GET", "/v1/guilds/"+guildA, key, ""); g["kudos_channel_id"] != kudos {
		t.Fatalf("kept without permission: %v", g["kudos_channel_id"])
	}
	syncChannels(`{"id":"` + general + `","name":"general","position":0,"can_post":true}`)
	if _, g := do(t, srv, "GET", "/v1/guilds/"+guildA, key, ""); g["kudos_channel_id"] != "" {
		t.Fatalf("cleared when deleted: %v", g["kudos_channel_id"])
	}

	// Other servers' channels stay out of reach.
	do(t, srv, "POST", "/v1/guilds", key, `{"discord_guild_id":"`+guildB+`","name":"B","admin_discord_user_id":"`+adminB+`"}`)
	other := newWebClient(t)
	login(t, srv, other, "managerB")
	if res, err := other.Get(srv.URL + "/web/guilds/" + guildA + "/channels"); err != nil || res.StatusCode == http.StatusOK {
		t.Fatalf("someone else's channels: %v %v", res.StatusCode, err)
	}
}
