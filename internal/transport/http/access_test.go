package http_test

import (
	"strings"
	"testing"

	httpapi "github.com/moixblau/HappyX.Api/internal/transport/http"
)

// TestWebAccess checks the main admin can hand out access to users and roles, that each level
// opens what it should, and that only owners choose who has access.
func TestWebAccess(t *testing.T) {
	srv := newTestServer(t, func(s *httpapi.Server) {
		s.WithWeb(httpapi.Web{Identity: fakeDiscord{}, SessionKey: []byte("0123456789abcdef0123456789abcdef"), PublicURL: "http://happyx.test/"})
	})
	const (
		viewer, manager, outsider = "200000000000000011", "200000000000000012", "200000000000000013"
		staffRole, leadsRole      = "400000000000000001", "400000000000000002"
	)
	for _, s := range []struct{ method, path, body string }{
		{"POST", "/v1/guilds", `{"discord_guild_id":"` + guildA + `","name":"A","admin_discord_user_id":"` + adminA + `"}`},
		{"PUT", "/v1/guilds/" + guildA + "/members/" + adminA, `{"username":"ana"}`},
		{"PUT", "/v1/guilds/" + guildA + "/members/" + viewer, `{"username":"vic","role_ids":["` + staffRole + `"]}`},
		{"PUT", "/v1/guilds/" + guildA + "/members/" + manager, `{"username":"max"}`},
		{"PUT", "/v1/guilds/" + guildA + "/members/" + outsider, `{"username":"oli"}`},
	} {
		if code, body := do(t, srv, s.method, s.path, key, s.body); code != 200 {
			t.Fatalf("%s %s: %d %v", s.method, s.path, code, body)
		}
	}
	// The bot syncs the roles; @everyone (the guild's own ID) is never stored.
	roles := `{"roles":[{"id":"` + staffRole + `","name":"Staff","color":3447003,"position":2},` +
		`{"id":"` + leadsRole + `","name":"Leads","color":0,"position":5},{"id":"` + guildA + `","name":"@everyone","position":0}]}`
	if code, body := do(t, srv, "PUT", "/v1/guilds/"+guildA+"/roles", key, roles); code != 204 {
		t.Fatalf("sync roles: %d %v", code, body)
	}

	owner := newWebClient(t)
	login(t, srv, owner, "adminA")
	code, acc := send(t, owner, "GET", srv.URL+"/web/guilds/"+guildA+"/access", "", "")
	if code != 200 || len(acc["roles"].([]any)) != 2 || acc["roles"].([]any)[0].(map[string]any)["name"] != "Leads" || len(acc["members"].([]any)) != 4 {
		t.Fatalf("access settings: %d %v", code, acc)
	}

	set := func(c, body string) (int, map[string]any) {
		return send(t, owner, "PUT", srv.URL+"/web/guilds/"+guildA+"/access", c, body)
	}
	grants := `"grants":[{"kind":"role","id":"` + staffRole + `","level":"view"},{"kind":"user","id":"` + manager + `","level":"manage"}]`
	if code, _ := set("text/plain", `{"admin_discord_user_id":"`+adminA+`",`+grants+`}`); code != 415 {
		t.Fatalf("access without JSON content type: %d", code)
	}
	for name, body := range map[string]string{
		"admin not a member": `{"admin_discord_user_id":"300000000000000077",` + grants + `}`,
		"unknown role":       `{"admin_discord_user_id":"` + adminA + `","grants":[{"kind":"role","id":"400000000000000099","level":"view"}]}`,
		"everyone role":      `{"admin_discord_user_id":"` + adminA + `","grants":[{"kind":"role","id":"` + guildA + `","level":"view"}]}`,
		"owner level":        `{"admin_discord_user_id":"` + adminA + `","grants":[{"kind":"user","id":"` + manager + `","level":"owner"}]}`,
	} {
		if code, _ := set("application/json", body); code != 400 {
			t.Errorf("%s: %d, want 400", name, code)
		}
	}
	if code, out := set("application/json", `{"admin_discord_user_id":"`+adminA+`",`+grants+`}`); code != 200 || len(out["grants"].([]any)) != 2 {
		t.Fatalf("set access: %d %v", code, out)
	}

	// The role grants "view": reports yes, settings and access no.
	v := newWebClient(t)
	login(t, srv, v, "user:"+viewer)
	if _, me := get(t, v, srv.URL+"/web/me"); strings.Join(guildIDs(me), ",") != guildA || me["guilds"].([]any)[0].(map[string]any)["access"] != "view" {
		t.Fatalf("viewer me: %v", me)
	}
	if res, _ := get(t, v, srv.URL+"/web/guilds/"+guildA+"/report"); res.StatusCode != 200 {
		t.Fatalf("viewer report: %d", res.StatusCode)
	}
	if code, _ := patchSettings(t, srv, v, guildA, "application/json", `{"survey_hour":9}`); code != 403 {
		t.Fatalf("viewer changing settings: %d", code)
	}
	if code, _ := send(t, v, "GET", srv.URL+"/web/guilds/"+guildA+"/access", "", ""); code != 403 {
		t.Fatalf("viewer reading access: %d", code)
	}

	// The user grant gives "manage": settings yes, access and deleting no.
	m := newWebClient(t)
	login(t, srv, m, "user:"+manager)
	if code, _ := patchSettings(t, srv, m, guildA, "application/json", `{"survey_hour":9}`); code != 200 {
		t.Fatalf("manager changing settings: %d", code)
	}
	if code, _ := send(t, m, "PUT", srv.URL+"/web/guilds/"+guildA+"/access", "application/json", `{"admin_discord_user_id":"`+manager+`","grants":[]}`); code != 403 {
		t.Fatalf("manager making themselves admin: %d", code)
	}
	if code := deleteGuild(t, srv, m, guildA, "application/json", `{"confirm_name":"A"}`); code != 403 {
		t.Fatalf("manager deleting the server: %d", code)
	}

	// Someone without a grant still can't tell the server uses HappyX.
	o := newWebClient(t)
	login(t, srv, o, "user:"+outsider)
	if res, _ := get(t, o, srv.URL+"/web/guilds/"+guildA+"/report"); res.StatusCode != 404 {
		t.Fatalf("outsider report: %d", res.StatusCode)
	}

	// The bot asks the same question for its commands.
	for user, want := range map[string]string{adminA: "owner", viewer: "view", manager: "manage", outsider: "none"} {
		if code, out := do(t, srv, "GET", "/v1/guilds/"+guildA+"/members/"+user+"/access", key, ""); code != 200 || out["level"] != want {
			t.Errorf("bot access for %s: %d %v, want %s", user, code, out, want)
		}
	}
	if _, out := do(t, srv, "GET", "/v1/guilds/"+guildA+"/members/"+outsider+"/access?discord_admin=true", key, ""); out["level"] != "owner" {
		t.Errorf("Discord administrator: %v", out)
	}

	// Losing the role, or leaving the server, ends the access.
	if code, _ := do(t, srv, "PUT", "/v1/guilds/"+guildA+"/members/"+viewer, key, `{"username":"vic","role_ids":[]}`); code != 200 {
		t.Fatal("drop role")
	}
	if res, _ := get(t, v, srv.URL+"/web/guilds/"+guildA+"/report"); res.StatusCode != 404 {
		t.Fatalf("viewer without the role: %d", res.StatusCode)
	}
	if code, _ := do(t, srv, "DELETE", "/v1/guilds/"+guildA+"/members/"+manager+"?left=true", key, ""); code != 204 {
		t.Fatal("manager leaves")
	}
	if res, _ := get(t, m, srv.URL+"/web/guilds/"+guildA+"/report"); res.StatusCode != 404 {
		t.Fatalf("manager after leaving the server: %d", res.StatusCode)
	}

	// Handing the main admin over: the new admin is an owner, and the bot reconnecting keeps them.
	if code, out := set("application/json", `{"admin_discord_user_id":"`+outsider+`","grants":[]}`); code != 200 || out["admin_discord_user_id"] != outsider {
		t.Fatalf("change admin: %d %v", code, out)
	}
	do(t, srv, "POST", "/v1/guilds", key, `{"discord_guild_id":"`+guildA+`","name":"A","admin_discord_user_id":"`+adminA+`"}`)
	if _, out := do(t, srv, "GET", "/v1/guilds/"+guildA, key, ""); out["admin_discord_user_id"] != outsider {
		t.Fatalf("re-registering must keep the chosen admin: %v", out)
	}
	if code, _ := send(t, owner, "GET", srv.URL+"/web/guilds/"+guildA+"/access", "", ""); code != 404 {
		t.Fatalf("former admin: %d", code)
	}
}
