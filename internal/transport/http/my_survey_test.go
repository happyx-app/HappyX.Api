package http_test

import (
	"testing"

	httpapi "github.com/happyx-app/HappyX.Api/internal/transport/http"
)

// A member who paused the survey sees it in /web/me and can resume it from the web, like
// /happyx resume. Nobody can resume it in a server they aren't in.
func TestWebPausedSurvey(t *testing.T) {
	srv := newTestServer(t, func(s *httpapi.Server) {
		s.WithWeb(httpapi.Web{Identity: fakeDiscord{}, SessionKey: []byte("0123456789abcdef0123456789abcdef"), PublicURL: "http://happyx.test/"})
	})
	for _, s := range []struct{ method, path, body string }{
		{"POST", "/v1/guilds", `{"discord_guild_id":"` + guildA + `","name":"A","admin_discord_user_id":"` + adminA + `"}`},
		{"POST", "/v1/guilds", `{"discord_guild_id":"` + guildB + `","name":"B","admin_discord_user_id":"` + adminB + `"}`},
		{"PUT", "/v1/guilds/" + guildA + "/members/" + memberID, `{"username":"Mia"}`},
		{"PUT", "/v1/guilds/" + guildB + "/members/" + memberID, `{"username":"Mia"}`},
	} {
		if code, body := do(t, srv, s.method, s.path, key, s.body); code != 200 {
			t.Fatalf("%s %s: %d %v", s.method, s.path, code, body)
		}
	}
	// «Don't ask me again» in A.
	if code, _ := do(t, srv, "DELETE", "/v1/guilds/"+guildA+"/members/"+memberID, key, ""); code != 204 {
		t.Fatalf("pause: %d", code)
	}
	m := newWebClient(t)
	login(t, srv, m, "member")
	paused := func() []string {
		_, me := get(t, m, srv.URL+"/web/me")
		var ids []string
		for _, g := range me["paused_guilds"].([]any) {
			ids = append(ids, g.(map[string]any)["discord_guild_id"].(string))
		}
		return ids
	}
	if p := paused(); len(p) != 1 || p[0] != guildA {
		t.Fatalf("paused: %v", p)
	}
	u := srv.URL + "/web/guilds/" + guildA + "/me/survey"
	if code, _ := send(t, m, "PUT", u, "text/plain", `{"active":true}`); code != 415 {
		t.Fatalf("resume without JSON: %d", code)
	}
	if code, body := send(t, m, "PUT", u, "application/json", `{"active":true}`); code != 200 || body["active"] != true {
		t.Fatalf("resume: %d %v", code, body)
	}
	if p := paused(); len(p) != 0 {
		t.Fatalf("still paused: %v", p)
	}
	// Pausing from the web works too, and a server they aren't in is not found.
	if code, _ := send(t, m, "PUT", u, "application/json", `{"active":false}`); code != 200 {
		t.Fatalf("pause from web: %d", code)
	}
	if p := paused(); len(p) != 1 {
		t.Fatalf("paused after web pause: %v", p)
	}
	if code, _ := send(t, m, "PUT", srv.URL+"/web/guilds/100000000000000077/me/survey", "application/json", `{"active":true}`); code != 404 {
		t.Fatalf("unknown server: %d", code)
	}
	a := newWebClient(t)
	login(t, srv, a, "adminA") // in no server as a member
	if code, _ := send(t, a, "PUT", u, "application/json", `{"active":true}`); code != 404 {
		t.Fatalf("not a member: %d", code)
	}
}
