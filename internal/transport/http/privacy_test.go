package http_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	httpapi "github.com/moixblau/HappyX.Api/internal/transport/http"
)

// download GETs a dashboard export and checks it comes as a JSON file.
func download(t *testing.T, c *http.Client, u string) (int, map[string]any) {
	t.Helper()
	res, err := c.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return res.StatusCode, nil
	}
	if cd := res.Header.Get("Content-Disposition"); !strings.HasPrefix(cd, `attachment; filename="happyx-`) || !strings.HasSuffix(cd, `.json"`) {
		t.Fatalf("export should download as a file: %q", cd)
	}
	var out map[string]any
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, out
}

func list(v any) []map[string]any {
	var out []map[string]any
	for _, x := range v.([]any) {
		out = append(out, x.(map[string]any))
	}
	return out
}

func TestWebPrivacy(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC) // Monday
	srv := newTestServerClock(t, func() time.Time { return now }, func(s *httpapi.Server) {
		s.WithWeb(httpapi.Web{Identity: fakeDiscord{}, SessionKey: []byte("0123456789abcdef0123456789abcdef"), PublicURL: "http://happyx.test/"})
	})
	const bea, cris = "200000000000000002", "200000000000000003"
	const jsonCT = "application/json"
	bot := func(method, path, body string) {
		t.Helper()
		if code, out := do(t, srv, method, path, key, body); code != 200 && code != 201 {
			t.Fatalf("%s %s: %d %v", method, path, code, out)
		}
	}
	bot("POST", "/v1/guilds", `{"discord_guild_id":"`+guildA+`","name":"Equipo A","admin_discord_user_id":"`+adminA+`"}`)
	bot("POST", "/v1/guilds", `{"discord_guild_id":"`+guildB+`","name":"B","admin_discord_user_id":"`+adminB+`"}`)
	for u, name := range map[string]string{memberID: "mia", bea: "bea", cris: "cris"} {
		bot("PUT", "/v1/guilds/"+guildA+"/members/"+u, `{"username":"`+name+`"}`)
	}
	bot("PUT", "/v1/guilds/"+guildB+"/members/"+memberID, `{"username":"mia"}`)
	bot("PUT", "/v1/guilds/"+guildA+"/members/"+memberID+"/mood", `{"mood":2,"message":"secreto"}`)
	bot("PUT", "/v1/guilds/"+guildB+"/members/"+memberID+"/mood", `{"mood":4}`)
	bot("POST", "/v1/guilds/"+guildA+"/kudos", `{"from_discord_user_id":"`+memberID+`","to_discord_user_id":"`+bea+`","message":"gracias"}`)

	admin := newWebClient(t)
	login(t, srv, admin, "adminA")
	// Mia answered while anonymous; Bea after the server went public.
	if code, out := patchSettings(t, srv, admin, guildA, jsonCT, `{"anonymous":false,"games_enabled":true}`); code != 200 {
		t.Fatalf("settings: %d %v", code, out)
	}
	bot("PUT", "/v1/guilds/"+guildA+"/members/"+bea+"/mood", `{"mood":5,"message":"público"}`)
	bot("POST", "/v1/guilds/"+guildA+"/game/rounds", `{"week":"2026-10-19","question":"¿Qué querías ser?"}`)
	for u, a := range map[string]string{memberID: "Bombera", bea: "Astronauta", cris: "Pastelero"} {
		bot("PUT", "/v1/guilds/"+guildA+"/game/rounds/2026-10-19/answers/"+u, `{"answer":"`+a+`"}`)
	}

	// Only people who see the server's dashboard can export it.
	exportA := srv.URL + "/web/guilds/" + guildA + "/export"
	if code, _ := download(t, newWebClient(t), exportA); code != 401 {
		t.Fatalf("export without a session: %d", code)
	}
	m := newWebClient(t)
	login(t, srv, m, "member")
	if code, _ := download(t, m, exportA); code != 404 {
		t.Fatalf("a plain member exporting the server: %d", code)
	}
	b := newWebClient(t)
	login(t, srv, b, "managerB")
	if code, _ := download(t, b, exportA); code != 404 {
		t.Fatalf("another server's manager exporting it: %d", code)
	}

	code, x := download(t, admin, exportA)
	if code != 200 || x["format"] != "happyx-server-data" || x["server"].(map[string]any)["name"] != "Equipo A" {
		t.Fatalf("server export: %d %v", code, x)
	}
	if n := len(list(x["members"])); n != 3 {
		t.Fatalf("members: %d", n)
	}
	moods := list(x["moods"])
	if len(moods) != 2 {
		t.Fatalf("moods: %v", moods)
	}
	for _, e := range moods {
		switch e["message"] {
		case "secreto":
			if e["username"] != nil {
				t.Fatalf("an answer given while anonymous must stay anonymous: %v", e)
			}
		case "público":
			if e["username"] != "bea" {
				t.Fatalf("a public answer names its author: %v", e)
			}
		default:
			t.Fatalf("unexpected mood: %v", e)
		}
	}
	if k := list(x["kudos"]); len(k) != 1 || k[0]["from_username"] != "mia" || k[0]["to_username"] != "bea" {
		t.Fatalf("kudos: %v", k)
	}
	answers := func(x map[string]any) []map[string]any {
		games := list(x["games"])
		if len(games) != 1 || games[0]["question"] != "¿Qué querías ser?" {
			t.Fatalf("games: %v", games)
		}
		return list(games[0]["answers"])
	}
	if a := answers(x); len(a) != 0 {
		t.Fatalf("answers must stay hidden while members answer: %v", a)
	}
	now = time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC) // Thursday: guessing
	_, x = download(t, admin, exportA)
	if a := answers(x); len(a) != 3 || a[0]["username"] != nil {
		t.Fatalf("while guessing, answers come without authors: %v", a)
	}
	now = time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC) // next Monday: results are out
	_, x = download(t, admin, exportA)
	if a := answers(x); len(a) != 3 || a[0]["username"] == nil {
		t.Fatalf("finished rounds name the authors: %v", a)
	}

	// Mia downloads her own data, in every server.
	if code, _ := download(t, newWebClient(t), srv.URL+"/web/me/data"); code != 401 {
		t.Fatalf("my data without a session: %d", code)
	}
	code, mine := download(t, m, srv.URL+"/web/me/data")
	if code != 200 || mine["format"] != "happyx-personal-data" || mine["user"].(map[string]any)["discord_user_id"] != memberID {
		t.Fatalf("my data: %d %v", code, mine)
	}
	servers := list(mine["servers"])
	if len(servers) != 2 || servers[0]["name"] != "B" || servers[1]["name"] != "Equipo A" {
		t.Fatalf("my servers: %v", servers)
	}
	inA := servers[1]
	if mo := list(inA["moods"]); len(mo) != 1 || mo[0]["message"] != "secreto" || mo[0]["anonymous"] != true {
		t.Fatalf("my moods in A: %v", mo)
	}
	if k := list(inA["kudos_given"]); len(k) != 1 || k[0]["to_username"] != "bea" {
		t.Fatalf("my kudos: %v", k)
	}
	if a := list(inA["game_answers"]); len(a) != 1 || a[0]["answer"] != "Bombera" {
		t.Fatalf("my game answers: %v", a)
	}
	if strings.Contains(mustJSON(t, mine), "público") {
		t.Fatal("my export must not hold other people's answers")
	}
	// The admin receives A's reports, and their export says so.
	if _, ad := download(t, admin, srv.URL+"/web/me/data"); len(list(ad["receives_reports_for"])) != 1 {
		t.Fatalf("admin's export: %v", ad)
	}

	// Erasing asks for the username.
	for _, c := range []struct {
		name, ct, body string
		want           int
	}{
		{"no content type", "", `{"confirm_name":"Mia"}`, 415},
		{"no confirmation", jsonCT, `{}`, 400},
		{"wrong name", jsonCT, `{"confirm_name":"Ana"}`, 400},
	} {
		if code, _ := send(t, m, "DELETE", srv.URL+"/web/me/data", c.ct, c.body); code != c.want {
			t.Fatalf("%s: got %d, want %d", c.name, code, c.want)
		}
	}
	if code, out := send(t, m, "DELETE", srv.URL+"/web/me/data", jsonCT, `{"confirm_name":"Mia"}`); code != 204 {
		t.Fatalf("erase: %d %v", code, out)
	}
	_, mine = download(t, m, srv.URL+"/web/me/data")
	for _, s := range list(mine["servers"]) {
		for _, k := range []string{"moods", "kudos_given", "kudos_received", "game_answers", "game_guesses", "game_proposals"} {
			if n := len(list(s[k])); n != 0 {
				t.Fatalf("%s left after erasing: %v", k, s)
			}
		}
		if mem := s["member"].(map[string]any); mem["taking_part"] != false || mem["username"] != "" {
			t.Fatalf("member after erasing: %v", mem)
		}
	}
	// Everyone else's data stays.
	_, x = download(t, admin, exportA)
	if mo := list(x["moods"]); len(mo) != 1 || mo[0]["message"] != "público" {
		t.Fatalf("moods after Mia erased hers: %v", mo)
	}
	if k := list(x["kudos"]); len(k) != 0 {
		t.Fatalf("kudos involving Mia: %v", k)
	}
	if a := answers(x); len(a) != 2 {
		t.Fatalf("game answers after Mia erased hers: %v", a)
	}
	// The bot's member sync doesn't sign her up for the survey again.
	bot("PUT", "/v1/guilds/"+guildA+"/members/"+memberID, `{"username":"mia"}`)
	if res, _ := doRaw(t, srv, "GET", "/v1/guilds/"+guildA+"/members"); strings.Contains(res, memberID) {
		t.Fatalf("erased member is taking part again: %s", res)
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
