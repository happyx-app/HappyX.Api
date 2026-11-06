package http_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	httpapi "github.com/happyx-app/HappyX.Api/internal/transport/http"
)

// doList is do for routes that answer with a JSON array.
func doList(t *testing.T, srv *httptest.Server, method, path, body string) (int, []map[string]any) {
	t.Helper()
	req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+key)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out []map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

// A member sets their own vacation on the web or with the bot; an admin sets one in their server,
// which the bot hears about once; the survey list leaves them out on those days.
func TestVacationRoutes(t *testing.T) {
	srv := newTestServer(t, func(s *httpapi.Server) { // 2026-10-22, a Thursday
		s.WithWeb(httpapi.Web{Identity: fakeDiscord{}, SessionKey: []byte("0123456789abcdef0123456789abcdef"), PublicURL: "http://happyx.test/"})
	})
	const other = "200000000000000002"
	for _, s := range []struct{ method, path, body string }{
		{"POST", "/v1/guilds", `{"discord_guild_id":"` + guildA + `","name":"A","admin_discord_user_id":"` + adminA + `"}`},
		{"PUT", "/v1/guilds/" + guildA + "/members/" + memberID, `{"username":"Mia"}`},
		{"PUT", "/v1/guilds/" + guildA + "/members/" + other, `{"username":"Olga"}`},
	} {
		if code, body := do(t, srv, s.method, s.path, key, s.body); code != 200 {
			t.Fatalf("%s %s: %d %v", s.method, s.path, code, body)
		}
	}
	surveyed := func() string {
		_, ms := doList(t, srv, "GET", "/v1/guilds/"+guildA+"/members?for=survey", "")
		var ids []string
		for _, m := range ms {
			ids = append(ids, m["discord_user_id"].(string))
		}
		return strings.Join(ids, ",")
	}

	m := newWebClient(t)
	login(t, srv, m, "member")
	if _, out := get(t, m, srv.URL+"/web/me/vacations"); out["own"] != nil || len(out["set_by_admins"].([]any)) != 0 {
		t.Fatalf("no vacations yet: %v", out)
	}
	if code, _ := send(t, m, "PUT", srv.URL+"/web/me/vacation", "text/plain", `{"from":"2026-10-22","to":"2026-10-23"}`); code != 415 {
		t.Fatalf("vacation without JSON: %d", code)
	}
	if code, out := send(t, m, "PUT", srv.URL+"/web/me/vacation", "application/json", `{"from":"2026-10-22","to":"22/10"}`); code != 400 {
		t.Fatalf("bad date: %d %v", code, out)
	}
	if code, out := send(t, m, "PUT", srv.URL+"/web/me/vacation", "application/json", `{"from":"2026-10-22","to":"2026-10-23"}`); code != 200 || out["to"] != "2026-10-23" || out["set_by_admin"] != false {
		t.Fatalf("set own: %d %v", code, out)
	}
	if got := surveyed(); got != other {
		t.Fatalf("survey on vacation: %s", got)
	}
	if code, out := doList(t, srv, "GET", "/v1/guilds/"+guildA+"/members/"+memberID+"/vacations", ""); code != 200 || len(out) != 1 || out[0]["from"] != "2026-10-22" {
		t.Fatalf("bot reads vacations: %d %v", code, out)
	}
	if code, _ := send(t, m, "DELETE", srv.URL+"/web/me/vacation", "application/json", ""); code != 204 {
		t.Fatalf("delete own: %d", code)
	}
	if got := surveyed(); got != memberID+","+other && got != other+","+memberID {
		t.Fatalf("survey after removing the vacation: %s", got)
	}

	// The admin's card: members to pick from, and a vacation set here for Olga.
	a := newWebClient(t)
	login(t, srv, a, "adminA")
	if res, out := get(t, a, srv.URL+"/web/guilds/"+guildA+"/vacations"); res.StatusCode != 200 || len(out["members"].([]any)) != 2 || len(out["vacations"].([]any)) != 0 {
		t.Fatalf("guild vacations: %d %v", res.StatusCode, out)
	}
	if code, _ := get(t, m, srv.URL+"/web/guilds/"+guildA+"/vacations"); code.StatusCode != 404 {
		t.Fatalf("a member read the vacations card: %d", code.StatusCode)
	}
	if code, out := send(t, a, "PUT", srv.URL+"/web/guilds/"+guildA+"/vacations/"+other, "application/json", `{"from":"2026-10-22","to":"2026-10-30"}`); code != 200 || out["set_here"] != true || out["username"] != "Olga" {
		t.Fatalf("admin sets: %d %v", code, out)
	}
	if code, out := do(t, srv, "PUT", "/v1/guilds/"+guildA+"/members/"+memberID+"/vacation", key, `{"from":"2026-10-22","to":"2026-10-22"}`); code != 200 || out["discord_guild_id"] != nil {
		t.Fatalf("own vacation from the bot: %d %v", code, out)
	}
	if got := surveyed(); got != "" {
		t.Fatalf("survey with both away: %s", got)
	}
	code, notices := doList(t, srv, "GET", "/v1/vacation-notices", "")
	if code != 200 || len(notices) != 1 || notices[0]["discord_user_id"] != other || notices[0]["set_by_discord_user_id"] != adminA {
		t.Fatalf("notices: %d %v", code, notices)
	}
	id := jsonNumber(notices[0]["id"])
	if code, _ := do(t, srv, "DELETE", "/v1/vacation-notices/"+id, key, ""); code != 204 {
		t.Fatalf("notice done: %d", code)
	}
	if _, notices := doList(t, srv, "GET", "/v1/vacation-notices", ""); len(notices) != 0 {
		t.Fatalf("notice still pending: %v", notices)
	}
	// Olga sees it on her page and can take it off; /happyx resume ends Mia's.
	o := newWebClient(t)
	login(t, srv, o, "user:"+other)
	if _, out := get(t, o, srv.URL+"/web/me/vacations"); len(out["set_by_admins"].([]any)) != 1 {
		t.Fatalf("Olga's vacations: %v", out)
	}
	if code, _ := send(t, o, "DELETE", srv.URL+"/web/guilds/"+guildA+"/me/vacation", "application/json", ""); code != 204 {
		t.Fatalf("Olga removes it: %d", code)
	}
	if code, _ := do(t, srv, "DELETE", "/v1/guilds/"+guildA+"/members/"+memberID+"/vacations", key, ""); code != 204 {
		t.Fatalf("resume ends vacations: %d", code)
	}
	if code, out := doList(t, srv, "GET", "/v1/guilds/"+guildA+"/members/"+memberID+"/vacations", ""); code != 200 || len(out) != 0 {
		t.Fatalf("after resume: %d %v", code, out)
	}
}

func jsonNumber(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
