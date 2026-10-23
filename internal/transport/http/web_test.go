package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/moixblau/HappyX.Api/internal/domain"
	httpapi "github.com/moixblau/HappyX.Api/internal/transport/http"
)

const (
	guildA, guildB = "100000000000000001", "100000000000000002"
	adminA, adminB = "300000000000000001", "300000000000000009"
	managerB       = "300000000000000002"
	memberID       = "200000000000000001"
)

// fakeDiscord logs in whoever the code names.
type fakeDiscord struct{}

func (fakeDiscord) AuthURL(state string) string {
	return "https://discord.test/authorize?state=" + state
}

func (fakeDiscord) Viewer(_ context.Context, code string) (domain.Viewer, error) {
	switch code {
	case "adminA":
		return domain.Viewer{DiscordID: adminA, Username: "Ana"}, nil
	case "member":
		return domain.Viewer{DiscordID: memberID, Username: "Mia"}, nil
	case "managerB":
		return domain.Viewer{DiscordID: managerB, Username: "Bea", ManagedGuilds: []string{guildB, "100000000000000077"}}, nil
	}
	return domain.Viewer{}, errors.New("bad code")
}

func newWebClient(t *testing.T) *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
}

func get(t *testing.T, c *http.Client, u string) (*http.Response, map[string]any) {
	t.Helper()
	res, err := c.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res, out
}

func login(t *testing.T, srv *httptest.Server, c *http.Client, code string) {
	t.Helper()
	res, _ := get(t, c, srv.URL+"/web/auth/login")
	loc, _ := url.Parse(res.Header.Get("Location"))
	if res.StatusCode != http.StatusFound || loc.Host != "discord.test" {
		t.Fatalf("login: %d %s", res.StatusCode, loc)
	}
	res, _ = get(t, c, srv.URL+"/web/auth/callback?code="+code+"&state="+loc.Query().Get("state"))
	if res.StatusCode != http.StatusFound || res.Header.Get("Location") != "http://happyx.test/" {
		t.Fatalf("callback: %d %s", res.StatusCode, res.Header.Get("Location"))
	}
}

func guildIDs(me map[string]any) []string {
	var ids []string
	for _, g := range me["guilds"].([]any) {
		ids = append(ids, g.(map[string]any)["discord_guild_id"].(string))
	}
	return ids
}

func TestWebDashboard(t *testing.T) {
	srv := newTestServer(t, func(s *httpapi.Server) {
		s.WithWeb(httpapi.Web{Identity: fakeDiscord{}, SessionKey: []byte("0123456789abcdef0123456789abcdef"), PublicURL: "http://happyx.test/"})
	})
	for _, s := range []struct{ method, path, body string }{
		{"POST", "/v1/guilds", `{"discord_guild_id":"` + guildA + `","name":"A","admin_discord_user_id":"` + adminA + `"}`},
		{"POST", "/v1/guilds", `{"discord_guild_id":"` + guildB + `","name":"B","admin_discord_user_id":"` + adminB + `"}`},
		{"PUT", "/v1/guilds/" + guildA + "/members/" + memberID, `{"username":"x"}`},
		{"PUT", "/v1/guilds/" + guildA + "/members/" + memberID + "/mood", `{"mood":5,"message":"genial"}`},
		{"PUT", "/v1/guilds/" + guildB + "/members/" + memberID, `{"username":"x"}`},
		{"PUT", "/v1/guilds/" + guildB + "/members/" + memberID + "/mood", `{"mood":2}`},
	} {
		if code, body := do(t, srv, s.method, s.path, key, s.body); code != 200 {
			t.Fatalf("%s %s: %d %v", s.method, s.path, code, body)
		}
	}
	report := "/report?from=2026-10-22&to=2026-10-22"

	anon := newWebClient(t)
	if res, _ := get(t, anon, srv.URL+"/web/me"); res.StatusCode != 401 {
		t.Fatalf("me without a session: %d", res.StatusCode)
	}
	if res, _ := get(t, anon, srv.URL+"/web/guilds/"+guildA+report); res.StatusCode != 401 {
		t.Fatalf("report without a session: %d", res.StatusCode)
	}
	// A callback whose state doesn't match the cookie is rejected (CSRF on login).
	if res, _ := get(t, anon, srv.URL+"/web/auth/callback?code=adminA&state=forged"); !strings.HasSuffix(res.Header.Get("Location"), "login=failed") {
		t.Fatalf("forged state: %s", res.Header.Get("Location"))
	}
	if res, _ := get(t, anon, srv.URL+"/web/me"); res.StatusCode != 401 {
		t.Fatal("forged state logged the user in")
	}
	// The bot's API key does not open the dashboard routes, and a session doesn't open /v1.
	if code, _ := do(t, srv, "GET", "/web/me", key, ""); code != 401 {
		t.Fatalf("API key on /web: %d", code)
	}

	// The registered admin sees their guild only.
	a := newWebClient(t)
	login(t, srv, a, "adminA")
	if _, me := get(t, a, srv.URL+"/web/me"); strings.Join(guildIDs(me), ",") != guildA {
		t.Fatalf("admin A guilds: %v", me)
	}
	res, rep := get(t, a, srv.URL+"/web/guilds/"+guildA+report)
	if res.StatusCode != 200 || rep["average"].(float64) != 5 {
		t.Fatalf("admin A report: %d %v", res.StatusCode, rep)
	}
	if m := rep["messages"].([]any)[0].(map[string]any); m["discord_user_id"] != nil {
		t.Fatalf("dashboard must keep reports anonymous: %v", m)
	}
	if res, _ := get(t, a, srv.URL+"/web/guilds/"+guildB+report); res.StatusCode != 404 {
		t.Fatalf("admin A reading guild B: %d", res.StatusCode)
	}
	if res, _ := a.Get(srv.URL + "/v1/guilds"); res.StatusCode != 401 {
		t.Fatalf("session on /v1: %d", res.StatusCode)
	}

	// A Discord administrator of B sees B (and not servers without HappyX).
	b := newWebClient(t)
	login(t, srv, b, "managerB")
	if _, me := get(t, b, srv.URL+"/web/me"); strings.Join(guildIDs(me), ",") != guildB {
		t.Fatalf("manager B guilds: %v", me)
	}
	if res, rep := get(t, b, srv.URL+"/web/guilds/"+guildB+report); res.StatusCode != 200 || rep["average"].(float64) != 2 {
		t.Fatalf("manager B report: %d %v", res.StatusCode, rep)
	}
	if res, _ := get(t, b, srv.URL+"/web/guilds/"+guildA+report); res.StatusCode != 404 {
		t.Fatalf("manager B reading guild A: %d", res.StatusCode)
	}

	// Logging out drops the session.
	req, _ := http.NewRequest("POST", srv.URL+"/web/auth/logout", nil)
	if res, err := b.Do(req); err != nil || res.StatusCode != 204 {
		t.Fatalf("logout: %v %v", res, err)
	}
	if res, _ := get(t, b, srv.URL+"/web/me"); res.StatusCode != 401 {
		t.Fatalf("me after logout: %d", res.StatusCode)
	}
}

func TestWebDisabled(t *testing.T) {
	srv := newTestServer(t)
	if code, _ := do(t, srv, "GET", "/web/auth/login", "", ""); code != 404 {
		t.Fatalf("dashboard routes should not exist without web config: %d", code)
	}
}

func TestWebMyMoods(t *testing.T) {
	srv := newTestServer(t, func(s *httpapi.Server) {
		s.WithWeb(httpapi.Web{Identity: fakeDiscord{}, SessionKey: []byte("0123456789abcdef0123456789abcdef"), PublicURL: "http://happyx.test/"})
	})
	const other = "200000000000000002"
	mood := func(g, u, day string, m int, msg string) {
		body := `{"mood":` + strconv.Itoa(m) + `,"day":"` + day + `","message":"` + msg + `"}`
		if code, out := do(t, srv, "PUT", "/v1/guilds/"+g+"/members/"+u+"/mood", key, body); code != 200 {
			t.Fatalf("mood: %d %v", code, out)
		}
	}
	for _, s := range []struct{ method, path, body string }{
		{"POST", "/v1/guilds", `{"discord_guild_id":"` + guildA + `","name":"Zeta","admin_discord_user_id":"` + adminA + `"}`},
		{"POST", "/v1/guilds", `{"discord_guild_id":"` + guildB + `","name":"alfa","admin_discord_user_id":"` + adminB + `"}`},
		{"PUT", "/v1/guilds/" + guildA + "/members/" + memberID, `{"username":"mia"}`},
		{"PUT", "/v1/guilds/" + guildB + "/members/" + memberID, `{"username":"mia"}`},
		{"PUT", "/v1/guilds/" + guildA + "/members/" + other, `{"username":"otro"}`},
	} {
		if code, body := do(t, srv, s.method, s.path, key, s.body); code != 200 {
			t.Fatalf("%s %s: %d %v", s.method, s.path, code, body)
		}
	}
	mood(guildA, memberID, "2026-10-01", 1, "") // outside the period, still counts for nothing
	mood(guildA, memberID, "2026-10-20", 4, "")
	mood(guildA, memberID, "2026-10-21", 5, "")
	mood(guildA, memberID, "2026-10-22", 3, "regular")
	mood(guildB, memberID, "2026-10-21", 2, "")
	mood(guildA, other, "2026-10-22", 1, "no es mio")

	anon := newWebClient(t)
	if res, _ := get(t, anon, srv.URL+"/web/me/moods"); res.StatusCode != 401 {
		t.Fatalf("my moods without a session: %d", res.StatusCode)
	}

	// A member who administers nothing still sees their own answers, and only theirs.
	c := newWebClient(t)
	login(t, srv, c, "member")
	if _, me := get(t, c, srv.URL+"/web/me"); len(me["guilds"].([]any)) != 0 {
		t.Fatalf("a plain member should not get dashboard servers: %v", me)
	}
	res, out := get(t, c, srv.URL+"/web/me/moods?from=2026-10-16&to=2026-10-22")
	if res.StatusCode != 200 {
		t.Fatalf("my moods: %d %v", res.StatusCode, out)
	}
	gs := out["guilds"].([]any)
	if len(gs) != 2 {
		t.Fatalf("want 2 servers, got %v", gs)
	}
	b, a := gs[0].(map[string]any), gs[1].(map[string]any)                                              // ordered by name: alfa, Zeta
	if b["discord_guild_id"] != guildB || b["responses"].(float64) != 1 || b["streak"].(float64) != 1 { // answered yesterday, not yet today
		t.Fatalf("server B: %v", b)
	}
	if a["discord_guild_id"] != guildA || a["responses"].(float64) != 3 || a["average"].(float64) != 4 || a["streak"].(float64) != 3 {
		t.Fatalf("server A: %v", a)
	}
	entries := a["entries"].([]any)
	if last := entries[len(entries)-1].(map[string]any); last["message"] != "regular" || last["day"] != "2026-10-22" {
		t.Fatalf("server A entries: %v", entries)
	}
	if res, _ := get(t, c, srv.URL+"/web/guilds/"+guildA+"/report?from=2026-10-16&to=2026-10-22"); res.StatusCode != 404 {
		t.Fatalf("a plain member reading the server report: %d", res.StatusCode)
	}
	if res, _ := get(t, c, srv.URL+"/web/me/moods?from=2026-10-22&to=2026-10-01"); res.StatusCode != 400 {
		t.Fatalf("reversed range: %d", res.StatusCode)
	}
}

func patchSettings(t *testing.T, srv *httptest.Server, c *http.Client, guild, contentType, body string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest("PATCH", srv.URL+"/web/guilds/"+guild+"/settings", strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

func TestWebSettings(t *testing.T) {
	srv := newTestServer(t, func(s *httpapi.Server) {
		s.WithWeb(httpapi.Web{Identity: fakeDiscord{}, SessionKey: []byte("0123456789abcdef0123456789abcdef"), PublicURL: "http://happyx.test/"})
	})
	for _, body := range []string{
		`{"discord_guild_id":"` + guildA + `","name":"A","admin_discord_user_id":"` + adminA + `"}`,
		`{"discord_guild_id":"` + guildB + `","name":"B","admin_discord_user_id":"` + adminB + `"}`,
	} {
		if code, out := do(t, srv, "POST", "/v1/guilds", key, body); code != 200 {
			t.Fatalf("register: %d %v", code, out)
		}
	}
	const jsonCT = "application/json"

	if code, _ := patchSettings(t, srv, newWebClient(t), guildA, jsonCT, `{"language":"es"}`); code != 401 {
		t.Fatalf("no session: %d", code)
	}

	a := newWebClient(t)
	login(t, srv, a, "adminA")
	code, g := patchSettings(t, srv, a, guildA, jsonCT+"; charset=utf-8",
		`{"language":"es","survey_hour":9,"timezone":"America/Bogota","report_frequency":"monthly","anonymous":false}`)
	if code != 200 || g["language"] != "es" || g["survey_hour"].(float64) != 9 || g["timezone"] != "America/Bogota" ||
		g["report_frequency"] != "monthly" || g["anonymous"] != false || g["admin_discord_user_id"] != adminA {
		t.Fatalf("admin A update: %d %v", code, g)
	}
	// The bot sees the change.
	if code, g := do(t, srv, "GET", "/v1/guilds/"+guildA, key, ""); code != 200 || g["language"] != "es" {
		t.Fatalf("bot view after update: %d %v", code, g)
	}
	for _, c := range []struct {
		name, guild, ct, body string
		want                  int
	}{
		{"another server", guildB, jsonCT, `{"language":"es"}`, 404},
		{"form post", guildA, "application/x-www-form-urlencoded", `language=es`, 415},
		{"no content type", guildA, "", `{"language":"es"}`, 415},
		{"bad hour", guildA, jsonCT, `{"survey_hour":24}`, 400},
		{"bad timezone", guildA, jsonCT, `{"timezone":"Mars/Olympus"}`, 400},
		{"changing the report admin", guildA, jsonCT, `{"admin_discord_user_id":"` + adminB + `"}`, 400},
	} {
		if code, out := patchSettings(t, srv, a, c.guild, c.ct, c.body); code != c.want {
			t.Fatalf("%s: got %d (%v), want %d", c.name, code, out, c.want)
		}
	}

	// A Discord Administrator of B may change B; a plain member may change nothing.
	b := newWebClient(t)
	login(t, srv, b, "managerB")
	if code, g := patchSettings(t, srv, b, guildB, jsonCT, `{"anonymous":true,"survey_hour":20}`); code != 200 || g["survey_hour"].(float64) != 20 {
		t.Fatalf("manager B update: %d %v", code, g)
	}
	m := newWebClient(t)
	login(t, srv, m, "member")
	if code, _ := patchSettings(t, srv, m, guildA, jsonCT, `{"language":"en"}`); code != 404 {
		t.Fatalf("plain member update: %d", code)
	}

	// A server whose bot was removed can't be changed.
	if code, _ := do(t, srv, "DELETE", "/v1/guilds/"+guildA, key, ""); code != 204 {
		t.Fatalf("remove guild: %d", code)
	}
	if code, _ := patchSettings(t, srv, a, guildA, jsonCT, `{"language":"en"}`); code != 409 {
		t.Fatalf("inactive guild: %d", code)
	}
}
