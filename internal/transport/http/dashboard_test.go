package http_test

import (
	"testing"

	httpapi "github.com/happyx-app/HappyX.Api/internal/transport/http"
)

// The dashboard's participation, top kudos receivers and server health.
func TestWebDashboardInsights(t *testing.T) {
	srv := newTestServer(t, func(s *httpapi.Server) {
		s.WithWeb(httpapi.Web{Identity: fakeDiscord{}, SessionKey: []byte("0123456789abcdef0123456789abcdef"), PublicURL: "http://happyx.test/"})
	})
	const other, third = "200000000000000002", "200000000000000003"
	for _, s := range []struct{ method, path, body string }{
		{"POST", "/v1/guilds", `{"discord_guild_id":"` + guildA + `","name":"A","admin_discord_user_id":"` + adminA + `"}`},
		{"PUT", "/v1/guilds/" + guildA + "/members/" + memberID, `{"username":"mia"}`},
		{"PUT", "/v1/guilds/" + guildA + "/members/" + other, `{"username":"otro"}`},
		{"PUT", "/v1/guilds/" + guildA + "/members/" + third, `{"username":"tere"}`},
		{"DELETE", "/v1/guilds/" + guildA + "/members/" + third, ``}, // «don't ask me again»
		{"PUT", "/v1/guilds/" + guildA + "/members/" + memberID + "/mood", `{"mood":4,"day":"2026-10-21"}`},
		{"PUT", "/v1/guilds/" + guildA + "/members/" + memberID + "/mood", `{"mood":5}`},
		{"PUT", "/v1/guilds/" + guildA + "/members/" + other + "/mood", `{"mood":3}`},
		{"PATCH", "/v1/guilds/" + guildA, `{"survey_enabled":true}`},
		{"POST", "/v1/guilds/" + guildA + "/dispatches", `{"kind":"survey","period":"2026-10-22"}`},
		{"PUT", "/v1/guilds/" + guildA + "/dispatches/survey/2026-10-22", `{"recipients":3,"failed":1,"closed_dms":1}`},
		{"POST", "/v1/guilds/" + guildA + "/kudos", `{"from_discord_user_id":"` + memberID + `","to_discord_user_id":"` + other + `","message":"gracias"}`},
		{"POST", "/v1/guilds/" + guildA + "/kudos", `{"from_discord_user_id":"` + memberID + `","to_discord_user_id":"` + other + `","message":"otra vez"}`},
		{"POST", "/v1/guilds/" + guildA + "/kudos", `{"from_discord_user_id":"` + other + `","to_discord_user_id":"` + memberID + `","message":"a ti"}`},
	} {
		if code, body := do(t, srv, s.method, s.path, key, s.body); code/100 != 2 {
			t.Fatalf("%s %s: %d %v", s.method, s.path, code, body)
		}
	}
	a := newWebClient(t)
	login(t, srv, a, "adminA")
	base := srv.URL + "/web/guilds/" + guildA

	// Only the 22nd has a known number of recipients: 2 answers out of 3 sent.
	res, r := get(t, a, base+"/report?from=2026-10-21&to=2026-10-22")
	if res.StatusCode != 200 {
		t.Fatalf("report: %d %v", res.StatusCode, r)
	}
	if p := r["participation"].(map[string]any); p["recipients"] != 3.0 || p["answered"] != 2.0 {
		t.Fatalf("participation: %v", p)
	}
	days := r["days"].([]any)
	if _, ok := days[0].(map[string]any)["recipients"]; ok || days[1].(map[string]any)["recipients"] != 3.0 {
		t.Fatalf("days: %v", days)
	}
	top := r["kudos"].(map[string]any)["top_receivers"].([]any)
	if len(top) != 2 || top[0].(map[string]any)["username"] != "otro" || top[0].(map[string]any)["count"] != 2.0 {
		t.Fatalf("top receivers: %v", top)
	}
	// Without any survey sent in the range there is no participation at all.
	if _, r := get(t, a, base+"/report?from=2026-10-21&to=2026-10-21"); r["participation"] != nil {
		t.Fatalf("participation without dispatches: %v", r["participation"])
	}

	res, h := get(t, a, base+"/health")
	if res.StatusCode != 200 || h["audience"] != 2.0 || h["opted_out"] != 1.0 {
		t.Fatalf("health: %d %v", res.StatusCode, h)
	}
	if l := h["last_survey"].(map[string]any); l["day"] != "2026-10-22" || l["recipients"] != 3.0 || l["failed"] != 1.0 || l["closed_dms"] != 1.0 {
		t.Fatalf("last survey: %v", l)
	}
	// The bot reads the same for /happyx status.
	if code, bh := do(t, srv, "GET", "/v1/guilds/"+guildA+"/health", key, ""); code != 200 || bh["audience"] != 2.0 || bh["last_survey"].(map[string]any)["closed_dms"] != 1.0 {
		t.Fatalf("bot health: %d %v", code, bh)
	}
	// Where whoever has DMs closed can answer, and whether HappyX can post there.
	if h["team_channel"] != nil {
		t.Fatalf("team channel without one: %v", h["team_channel"])
	}
	const team, general = "111111111111111111", "222222222222222222"
	teamChannel := func() map[string]any {
		t.Helper()
		_, h := get(t, a, base+"/health")
		c, _ := h["team_channel"].(map[string]any)
		return c
	}
	for _, s := range []struct{ method, path, body string }{
		{"PUT", "/v1/guilds/" + guildA + "/channels", `{"channels":[{"id":"` + team + `","name":"equipo","position":1,"can_post":false},{"id":"` + general + `","name":"general","position":2,"can_post":true}]}`},
		{"PATCH", "/v1/guilds/" + guildA, `{"team_channel_id":"` + team + `"}`},
	} {
		if code, body := do(t, srv, s.method, s.path, key, s.body); code/100 != 2 {
			t.Fatalf("%s %s: %d %v", s.method, s.path, code, body)
		}
	}
	if c := teamChannel(); c["id"] != team || c["can_post"] != false {
		t.Fatalf("team channel HappyX can't post in: %v", c)
	}
	// Without a team channel, the kudos one is where HappyX posts.
	if code, body := do(t, srv, "PATCH", "/v1/guilds/"+guildA, key, `{"team_channel_id":"","kudos_channel_id":"`+general+`"}`); code/100 != 2 {
		t.Fatalf("settings: %d %v", code, body)
	}
	if c := teamChannel(); c["id"] != general || c["can_post"] != true {
		t.Fatalf("kudos channel as the team's: %v", c)
	}
	if code, _ := do(t, srv, "GET", "/v1/guilds/999999999999999999/health", key, ""); code != 404 {
		t.Fatalf("health of an unknown server: %d", code)
	}

	// Someone who can't see the server gets nothing.
	m := newWebClient(t)
	login(t, srv, m, "member")
	if res, _ := get(t, m, base+"/health"); res.StatusCode != 404 {
		t.Fatalf("member health: %d", res.StatusCode)
	}
}
