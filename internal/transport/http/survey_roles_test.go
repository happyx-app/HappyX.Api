package http_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	httpapi "github.com/moixblau/HappyX.Api/internal/transport/http"
)

// memberIDs lists who GET /v1/guilds/{guildID}/members{query} returns, sorted.
func memberIDs(t *testing.T, srv *httptest.Server, guild, query string) string {
	t.Helper()
	req, _ := http.NewRequest("GET", srv.URL+"/v1/guilds/"+guild+"/members"+query, nil)
	req.Header.Set("Authorization", "Bearer "+key)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var ms []struct {
		ID string `json:"discord_user_id"`
	}
	if err := json.NewDecoder(res.Body).Decode(&ms); err != nil {
		t.Fatalf("members%s: %d %v", query, res.StatusCode, err)
	}
	ids := make([]string, 0, len(ms))
	for _, m := range ms {
		ids = append(ids, m.ID)
	}
	sort.Strings(ids)
	return strings.Join(ids, ",")
}

// TestSurveyRoles checks a server can leave members out of the survey by role, or send it only to
// some roles, and that deleted roles drop off the list.
func TestSurveyRoles(t *testing.T) {
	srv := newTestServer(t, func(s *httpapi.Server) {
		s.WithWeb(httpapi.Web{Identity: fakeDiscord{}, SessionKey: []byte("0123456789abcdef0123456789abcdef"), PublicURL: "http://happyx.test/"})
	})
	const (
		staff, ext, both, viewer = "200000000000000021", "200000000000000022", "200000000000000023", "200000000000000024"
		staffRole, extRole       = "400000000000000011", "400000000000000012"
	)
	for _, s := range []struct{ method, path, body string }{
		{"POST", "/v1/guilds", `{"discord_guild_id":"` + guildA + `","name":"A","admin_discord_user_id":"` + adminA + `"}`},
		{"PUT", "/v1/guilds/" + guildA + "/members/" + adminA, `{"username":"ana"}`},
		{"PUT", "/v1/guilds/" + guildA + "/members/" + staff, `{"username":"sam","role_ids":["` + staffRole + `"]}`},
		{"PUT", "/v1/guilds/" + guildA + "/members/" + ext, `{"username":"eve","role_ids":["` + extRole + `"]}`},
		{"PUT", "/v1/guilds/" + guildA + "/members/" + both, `{"username":"bo","role_ids":["` + staffRole + `","` + extRole + `"]}`},
		{"PUT", "/v1/guilds/" + guildA + "/members/" + viewer, `{"username":"vic","role_ids":["` + staffRole + `"]}`},
	} {
		if code, body := do(t, srv, s.method, s.path, key, s.body); code != 200 {
			t.Fatalf("%s %s: %d %v", s.method, s.path, code, body)
		}
	}
	syncRoles := func(roles string) {
		t.Helper()
		if code, body := do(t, srv, "PUT", "/v1/guilds/"+guildA+"/roles", key, `{"roles":[`+roles+`]}`); code != 204 {
			t.Fatalf("sync roles: %d %v", code, body)
		}
	}
	syncRoles(`{"id":"` + staffRole + `","name":"Staff","position":2},{"id":"` + extRole + `","name":"Externals","position":1}`)

	everyone := strings.Join([]string{adminA, staff, ext, both, viewer}, ",")
	sorted := func(ids ...string) string { sort.Strings(ids); return strings.Join(ids, ",") }
	if _, g := do(t, srv, "GET", "/v1/guilds/"+guildA, key, ""); g["survey_roles"].(map[string]any)["mode"] != "all" {
		t.Fatalf("default survey roles: %v", g["survey_roles"])
	}
	if got := memberIDs(t, srv, guildA, "?for=survey"); got != sorted(strings.Split(everyone, ",")...) {
		t.Fatalf("everyone by default: %s", got)
	}

	owner := newWebClient(t)
	login(t, srv, owner, "adminA")
	if code, roles := send(t, owner, "GET", srv.URL+"/web/guilds/"+guildA+"/roles", "", ""); code != 200 {
		t.Fatalf("roles: %d %v", code, roles)
	}
	for name, body := range map[string]string{
		"unknown role":  `{"survey_roles":{"mode":"except","role_ids":["400000000000000099"]}}`,
		"no roles":      `{"survey_roles":{"mode":"only","role_ids":[]}}`,
		"unknown mode":  `{"survey_roles":{"mode":"some","role_ids":["` + extRole + `"]}}`,
		"all and roles": `{"survey_roles":{"mode":"all","role_ids":["` + extRole + `"]}}`,
	} {
		if code, _ := patchSettings(t, srv, owner, guildA, "application/json", body); code != 400 {
			t.Errorf("%s: %d, want 400", name, code)
		}
	}

	// Leaving externals out: one excluded role is enough, even with an included one.
	code, g := patchSettings(t, srv, owner, guildA, "application/json", `{"survey_roles":{"mode":"except","role_ids":["`+extRole+`"]}}`)
	if code != 200 || g["survey_roles"].(map[string]any)["mode"] != "except" {
		t.Fatalf("except: %d %v", code, g)
	}
	if got := memberIDs(t, srv, guildA, "?for=survey"); got != sorted(adminA, staff, viewer) {
		t.Fatalf("except externals: %s", got)
	}
	// Other DMs, like the game, still go to everyone who hasn't opted out.
	if got := memberIDs(t, srv, guildA, ""); got != sorted(strings.Split(everyone, ",")...) {
		t.Fatalf("members without for=survey: %s", got)
	}

	// Only staff. The bot can set it too.
	if code, g := do(t, srv, "PATCH", "/v1/guilds/"+guildA, key, `{"survey_roles":{"mode":"only","role_ids":["`+staffRole+`"]}}`); code != 200 {
		t.Fatalf("only via /v1: %d %v", code, g)
	}
	if got := memberIDs(t, srv, guildA, "?for=survey"); got != sorted(staff, both, viewer) {
		t.Fatalf("only staff: %s", got)
	}
	// Opting out still wins.
	if code, _ := do(t, srv, "DELETE", "/v1/guilds/"+guildA+"/members/"+viewer, key, ""); code != 204 {
		t.Fatal("opt out")
	}
	if got := memberIDs(t, srv, guildA, "?for=survey"); got != sorted(staff, both) {
		t.Fatalf("only staff after opting out: %s", got)
	}

	// Deleting the only listed role in Discord goes back to everyone rather than nobody.
	syncRoles(`{"id":"` + extRole + `","name":"Externals","position":1}`)
	if _, g := do(t, srv, "GET", "/v1/guilds/"+guildA, key, ""); g["survey_roles"].(map[string]any)["mode"] != "all" ||
		len(g["survey_roles"].(map[string]any)["role_ids"].([]any)) != 0 {
		t.Fatalf("after deleting the role: %v", g["survey_roles"])
	}

	// A deleted role is pruned from a longer list, which keeps its mode.
	syncRoles(`{"id":"` + staffRole + `","name":"Staff","position":2},{"id":"` + extRole + `","name":"Externals","position":1}`)
	patchSettings(t, srv, owner, guildA, "application/json", `{"survey_roles":{"mode":"except","role_ids":["`+extRole+`","`+staffRole+`"]}}`)
	syncRoles(`{"id":"` + extRole + `","name":"Externals","position":1}`)
	if _, g := do(t, srv, "GET", "/v1/guilds/"+guildA, key, ""); g["survey_roles"].(map[string]any)["mode"] != "except" ||
		len(g["survey_roles"].(map[string]any)["role_ids"].([]any)) != 1 {
		t.Fatalf("after deleting one of two roles: %v", g["survey_roles"])
	}
}
