package http_test

import (
	"net/http"
	"testing"

	httpapi "github.com/happyx-app/HappyX.Api/internal/transport/http"
)

// An admin marks, changes and deletes events on the chart; whoever sees the dashboard reads them,
// members see them on their own chart, and nobody else reaches them, in this guild or another.
func TestTeamEventRoutes(t *testing.T) {
	srv := newTestServer(t, func(s *httpapi.Server) { // 2026-10-22
		s.WithWeb(httpapi.Web{Identity: fakeDiscord{}, SessionKey: []byte("0123456789abcdef0123456789abcdef"), PublicURL: "http://happyx.test/"})
	})
	const jsonCT = "application/json"
	for _, s := range []struct{ method, path, body string }{
		{"POST", "/v1/guilds", `{"discord_guild_id":"` + guildA + `","name":"A","admin_discord_user_id":"` + adminA + `"}`},
		{"POST", "/v1/guilds", `{"discord_guild_id":"` + guildB + `","name":"B","admin_discord_user_id":"` + adminB + `"}`},
		{"PUT", "/v1/guilds/" + guildA + "/members/" + memberID, `{"username":"Mia"}`},
		{"PUT", "/v1/guilds/" + guildA + "/members/" + memberID + "/mood", `{"mood":2,"day":"2026-10-21"}`},
	} {
		if code, body := do(t, srv, s.method, s.path, key, s.body); code != 200 {
			t.Fatalf("%s %s: %d %v", s.method, s.path, code, body)
		}
	}
	admin, otherAdmin, m := newWebClient(t), newWebClient(t), newWebClient(t)
	login(t, srv, admin, "adminA")
	login(t, srv, otherAdmin, "user:"+adminB)
	login(t, srv, m, "member")
	events := srv.URL + "/web/guilds/" + guildA + "/events"
	period := "?from=2026-10-01&to=2026-10-22"

	if code, _ := send(t, admin, "POST", events, "text/plain", `{}`); code != 415 {
		t.Fatalf("without JSON: %d", code)
	}
	for _, bad := range []string{
		`{"from":"2026-10-20","to":"2026-10-21","title":" "}`,
		`{"from":"2026-10-21","to":"2026-10-20","title":"Hack"}`,
		`{"from":"2026-10-22","to":"2026-10-23","title":"Tomorrow"}`,
		`{"from":"2026-10-20","to":"2026-10-21","title":"Hack","color":"violet"}`,
		`{"from":"20 oct","to":"2026-10-21","title":"Hack"}`,
	} {
		if code, _ := send(t, admin, "POST", events, jsonCT, bad); code != 400 {
			t.Fatalf("%s: %d", bad, code)
		}
	}
	code, e := send(t, admin, "POST", events, jsonCT, `{"from":"2026-10-20","to":"2026-10-21","title":" Hack ","description":"Web down"}`)
	if code != 201 || e["title"] != "Hack" || e["color"] != "yellow" || e["from"] != "2026-10-20" || e["to"] != "2026-10-21" {
		t.Fatalf("create: %d %v", code, e)
	}
	id := e["id"].(string)

	// Only those who see the dashboard list them; another guild's admin and plain members get 404.
	if res, out := get(t, admin, events+period); res.StatusCode != 200 || len(out["events"].([]any)) != 1 {
		t.Fatalf("list: %d %v", res.StatusCode, out)
	}
	if res, out := get(t, admin, events+"?from=2026-10-01&to=2026-10-19"); res.StatusCode != 200 || len(out["events"].([]any)) != 0 {
		t.Fatalf("list outside: %d %v", res.StatusCode, out)
	}
	for name, c := range map[string]*http.Client{"other admin": otherAdmin, "member": m} {
		if res, _ := get(t, c, events+period); res.StatusCode != 404 {
			t.Fatalf("%s lists: %d", name, res.StatusCode)
		}
		if code, _ := send(t, c, "PUT", events+"/"+id, jsonCT, `{"from":"2026-10-20","to":"2026-10-20","title":"x"}`); code != 404 {
			t.Fatalf("%s edits: %d", name, code)
		}
	}
	// Another guild's path can't reach this guild's event either.
	if code, _ := send(t, otherAdmin, "DELETE", srv.URL+"/web/guilds/"+guildB+"/events/"+id, jsonCT, `{}`); code != 404 {
		t.Fatalf("delete through another guild: %d", code)
	}

	// Members see the event on their own chart.
	if res, out := get(t, m, srv.URL+"/web/me/moods"+period); res.StatusCode != 200 {
		t.Fatalf("my moods: %d", res.StatusCode)
	} else if gs := out["guilds"].([]any); len(gs) != 1 || len(gs[0].(map[string]any)["events"].([]any)) != 1 {
		t.Fatalf("my moods events: %v", out)
	}

	code, e = send(t, admin, "PUT", events+"/"+id, jsonCT, `{"from":"2026-10-21","to":"2026-10-21","title":"Hack","color":"red"}`)
	if code != 200 || e["color"] != "red" || e["from"] != "2026-10-21" || e["description"] != nil {
		t.Fatalf("update: %d %v", code, e)
	}
	if code, _ := send(t, admin, "PUT", events+"/abc", jsonCT, `{"from":"2026-10-21","to":"2026-10-21","title":"x"}`); code != 400 {
		t.Fatalf("bad id: %d", code)
	}
	if code, _ := send(t, admin, "DELETE", events+"/"+id, jsonCT, `{}`); code != 204 {
		t.Fatalf("delete: %d", code)
	}
	if code, _ := send(t, admin, "DELETE", events+"/"+id, jsonCT, `{}`); code != 404 {
		t.Fatalf("delete twice: %d", code)
	}
}
