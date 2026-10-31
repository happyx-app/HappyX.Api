package http_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/happyx-app/HappyX.Api/internal/repository/demo"
	httpapi "github.com/happyx-app/HappyX.Api/internal/transport/http"
	"github.com/happyx-app/HappyX.Api/internal/usecase"
)

// The public demo needs no login, only ever shows the made-up server, and can't write anything.
func TestDemo(t *testing.T) {
	now := func() time.Time { return time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC) }
	srv := newTestServerClock(t, now, func(s *httpapi.Server) {
		s.WithWeb(httpapi.Web{Identity: fakeDiscord{}, SessionKey: []byte("0123456789abcdef0123456789abcdef"), PublicURL: "http://happyx.test/"})
		s.WithDemo(httpapi.Demo{Services: map[string]*usecase.Service{"es": demo.NewStore("es", now).NewService(), "en": demo.NewStore("en", now).NewService()}, Viewer: demo.Viewer()})
	})
	// A real server that happens to have the demo's ID, with a secret in it.
	if guildA != demo.GuildID {
		t.Fatal("this test wants a real server with the demo's ID")
	}
	for _, s := range []struct{ method, path, body string }{
		{"POST", "/v1/guilds", `{"discord_guild_id":"` + guildA + `","name":"Real","admin_discord_user_id":"` + adminA + `"}`},
		{"PUT", "/v1/guilds/" + guildA + "/members/" + memberID, `{"username":"mia"}`},
		{"PUT", "/v1/guilds/" + guildA + "/members/" + memberID + "/mood", `{"mood":1,"message":"real secret"}`},
	} {
		if code, body := do(t, srv, s.method, s.path, key, s.body); code/100 != 2 {
			t.Fatalf("%s %s: %d %v", s.method, s.path, code, body)
		}
	}

	c := newWebClient(t) // no session
	res, me := get(t, c, srv.URL+"/web/demo/es/me")
	if res.StatusCode != 200 || len(res.Cookies()) != 0 {
		t.Fatalf("me: %d %v", res.StatusCode, res.Cookies())
	}
	gs := me["guilds"].([]any)
	if len(gs) != 1 || gs[0].(map[string]any)["name"] != "Equipo Demo" || gs[0].(map[string]any)["access"] != "owner" {
		t.Fatalf("me: %v", me)
	}
	base := srv.URL + "/web/demo/es/guilds/" + demo.GuildID
	res, r := get(t, c, base+"/report?from=2026-10-01&to=2026-10-22")
	if res.StatusCode != 200 || r["responses"].(float64) < 50 {
		t.Fatalf("report: %d %v", res.StatusCode, r)
	}
	for _, m := range r["messages"].([]any) {
		if strings.Contains(m.(map[string]any)["message"].(string), "real secret") {
			t.Fatal("the demo showed a real server's answer")
		}
	}
	for _, p := range []string{"/health", "/roles", "/channels", "/access", "/game"} {
		if res, body := get(t, c, base+p); res.StatusCode != 200 {
			t.Fatalf("%s: %d %v", p, res.StatusCode, body)
		}
	}
	if res, _ := get(t, c, srv.URL+"/web/demo/es/me/moods?from=2026-10-01&to=2026-10-22"); res.StatusCode != 200 {
		t.Fatalf("moods: %d", res.StatusCode)
	}
	// Other servers don't exist in the demo, and the real routes still want a login.
	if res, _ := get(t, c, srv.URL+"/web/demo/es/guilds/100000000000000002/report"); res.StatusCode != 404 {
		t.Fatalf("other server: %d", res.StatusCode)
	}
	if res, me := get(t, c, srv.URL+"/web/demo/en/me"); res.StatusCode != 200 || me["guilds"].([]any)[0].(map[string]any)["name"] != "Demo Team" {
		t.Fatalf("English demo: %d %v", res.StatusCode, me)
	}
	if res, _ := get(t, c, srv.URL+"/web/demo/fr/me"); res.StatusCode != 404 {
		t.Fatalf("unknown language: %d", res.StatusCode)
	}
	if res, _ := get(t, c, srv.URL+"/web/guilds/"+guildA+"/report"); res.StatusCode != 401 {
		t.Fatalf("real report without login: %d", res.StatusCode)
	}
	// There are no write routes.
	for _, w := range []struct{ method, path string }{
		{"PATCH", "/settings"}, {"PUT", "/access"}, {"DELETE", ""}, {"PUT", "/game/guesses"}, {"GET", "/export"},
	} {
		req, _ := http.NewRequest(w.method, base+w.path, strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		res, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode/100 == 2 {
			t.Fatalf("%s %s: %d", w.method, w.path, res.StatusCode)
		}
	}
}

// Without WithDemo the demo routes don't exist.
func TestDemoOff(t *testing.T) {
	srv := newTestServer(t, func(s *httpapi.Server) {
		s.WithWeb(httpapi.Web{Identity: fakeDiscord{}, SessionKey: []byte("0123456789abcdef0123456789abcdef"), PublicURL: "http://happyx.test/"})
	})
	if res, _ := get(t, newWebClient(t), srv.URL+"/web/demo/es/me"); res.StatusCode != 404 {
		t.Fatalf("demo off: %d", res.StatusCode)
	}
}
