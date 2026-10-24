package http_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	httpapi "github.com/moixblau/HappyX.Api/internal/transport/http"
)

func send(t *testing.T, c *http.Client, method, u, contentType, body string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest(method, u, strings.NewReader(body))
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

func TestGameFlow(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC) // Monday
	srv := newTestServerClock(t, func() time.Time { return now }, func(s *httpapi.Server) {
		s.WithWeb(httpapi.Web{Identity: fakeDiscord{}, SessionKey: []byte("0123456789abcdef0123456789abcdef"), PublicURL: "http://happyx.test/"})
	})
	const p2, p3 = "200000000000000002", "200000000000000003"
	if code, out := do(t, srv, "POST", "/v1/guilds", key, `{"discord_guild_id":"`+guildA+`","name":"A","admin_discord_user_id":"`+adminA+`"}`); code != 200 {
		t.Fatalf("register: %d %v", code, out)
	}
	for _, u := range []string{memberID, p2, p3} {
		if code, out := do(t, srv, "PUT", "/v1/guilds/"+guildA+"/members/"+u, key, `{"username":"user`+u[len(u)-1:]+`"}`); code != 200 {
			t.Fatalf("join: %d %v", code, out)
		}
	}
	round := `{"week":"2026-10-19","question":"¿Qué querías ser de pequeño?"}`
	if code, _ := do(t, srv, "POST", "/v1/guilds/"+guildA+"/game/rounds", key, round); code != 409 {
		t.Fatalf("round with games off: %d", code)
	}

	admin := newWebClient(t)
	login(t, srv, admin, "adminA")
	if code, g := patchSettings(t, srv, admin, guildA, "application/json", `{"games_enabled":true}`); code != 200 || g["games_enabled"] != true {
		t.Fatalf("turn games on: %d %v", code, g)
	}
	if code, out := do(t, srv, "POST", "/v1/guilds/"+guildA+"/game/rounds", key, round); code != 200 || out["phase"] != "answering" || out["guessing_starts"] != "2026-10-22" {
		t.Fatalf("start round: %d %v", code, out)
	}
	for u, a := range map[string]string{memberID: "Bombero", p2: "Astronauta", p3: "Pastelera"} {
		if code, out := do(t, srv, "PUT", "/v1/guilds/"+guildA+"/game/rounds/2026-10-19/answers/"+u, key, `{"answer":"`+a+`"}`); code != 200 {
			t.Fatalf("answer: %d %v", code, out)
		}
	}

	m := newWebClient(t)
	login(t, srv, m, "member")
	if code, _ := send(t, m, "PUT", srv.URL+"/web/guilds/"+guildA+"/game/proposal", "application/json", `{"question":"¿Pronto?"}`); code != 409 {
		t.Fatalf("proposing on Monday: %d", code)
	}
	if code, out := send(t, m, "PUT", srv.URL+"/web/guilds/"+guildA+"/game/answer", "application/json", `{"answer":"Bombera"}`); code != 200 || out["answer"] != "Bombera" {
		t.Fatalf("web answer: %d %v", code, out)
	}
	_, me := get(t, m, srv.URL+"/web/me")
	if games := me["games"].([]any); len(games) != 1 || games[0].(map[string]any)["discord_guild_id"] != guildA {
		t.Fatalf("member's games: %v", me)
	}
	// The admin isn't a member, so they configure the game but don't play it.
	if _, me := get(t, admin, srv.URL+"/web/me"); len(me["games"].([]any)) != 0 {
		t.Fatalf("admin's games: %v", me)
	}
	if code, _ := get(t, admin, srv.URL+"/web/guilds/"+guildA+"/game"); code.StatusCode != 404 {
		t.Fatalf("admin game page: %d", code.StatusCode)
	}

	// Thursday: guessing.
	now = time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	if code, out := do(t, srv, "GET", "/v1/guilds/"+guildA+"/game/rounds/2026-10-19", key, ""); code != 200 || out["phase"] != "guessing" || out["answers"].(float64) != 3 {
		t.Fatalf("bot round status: %d %v", code, out)
	}
	_, board := get(t, m, srv.URL+"/web/guilds/"+guildA+"/game")
	cur := board["current"].(map[string]any)
	ids := map[string]string{}
	for _, a := range cur["answers"].([]any) {
		a := a.(map[string]any)
		ids[a["answer"].(string)] = a["id"].(string)
		if _, named := a["discord_user_id"]; named != (a["answer"] == "Bombera") || a["mine"] != (a["answer"] == "Bombera") {
			t.Fatalf("answer while guessing: %v", a)
		}
	}
	if cur["phase"] != "guessing" || len(ids) != 3 || len(cur["players"].([]any)) != 3 {
		t.Fatalf("guessing board: %v", board)
	}
	guesses := `{"guesses":{"` + ids["Astronauta"] + `":"` + p2 + `","` + ids["Pastelera"] + `":"` + p2 + `"}}`
	if code, _ := send(t, m, "PUT", srv.URL+"/web/guilds/"+guildA+"/game/guesses", "text/plain", guesses); code != 415 {
		t.Fatalf("guesses without JSON content type: %d", code)
	}
	if code, out := send(t, m, "PUT", srv.URL+"/web/guilds/"+guildA+"/game/guesses", "application/json", guesses); code != 204 {
		t.Fatalf("guesses: %d %v", code, out)
	}
	if code, _ := send(t, m, "PUT", srv.URL+"/web/guilds/"+guildA+"/game/answer", "application/json", `{"answer":"tarde"}`); code != 409 {
		t.Fatalf("answering on Thursday: %d", code)
	}
	// Proposing next week's question, from the web and from Discord.
	if code, out := send(t, m, "PUT", srv.URL+"/web/guilds/"+guildA+"/game/proposal", "application/json", `{"question":"¿Tu primer concierto?"}`); code != 200 || out["week"] != "2026-10-26" {
		t.Fatalf("web proposal: %d %v", code, out)
	}
	if code, out := do(t, srv, "PUT", "/v1/guilds/"+guildA+"/game/proposals/"+p2, key, `{"question":"¿Tu olor favorito?"}`); code != 200 {
		t.Fatalf("bot proposal: %d %v", code, out)
	}
	if code, _ := do(t, srv, "PUT", "/v1/guilds/"+guildA+"/game/proposals/"+adminA, key, `{"question":"x"}`); code != 403 {
		t.Fatalf("proposal by a non-member: %d", code)
	}
	if code, _ := send(t, admin, "PUT", srv.URL+"/web/guilds/"+guildA+"/game/proposal", "application/json", `{"question":"x"}`); code != 404 {
		t.Fatalf("web proposal by a non-member: %d", code)
	}
	_, board = get(t, m, srv.URL+"/web/guilds/"+guildA+"/game")
	if p := board["proposal"].(map[string]any); p["open"] != true || p["week"] != "2026-10-26" || p["mine"] != "¿Tu primer concierto?" || p["count"].(float64) != 2 {
		t.Fatalf("proposal box: %v", p)
	}

	// Next Monday: last week's results.
	now = time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	_, board = get(t, m, srv.URL+"/web/guilds/"+guildA+"/game")
	prev := board["previous"].(map[string]any)
	ranking := prev["ranking"].([]any)
	if board["current"] != nil || prev["phase"] != "finished" || len(ranking) != 1 || ranking[0].(map[string]any)["correct"].(float64) != 1 {
		t.Fatalf("finished board: %v", board)
	}
	for _, a := range prev["answers"].([]any) {
		if a.(map[string]any)["username"] == nil {
			t.Fatalf("author hidden after the round: %v", a)
		}
	}
	if prev["proposed"] != false || prev["proposed_by"] != nil {
		t.Fatalf("the bot's question shows as proposed: %v", prev)
	}

	// This week's question is drawn from the proposals, and who proposed it stays secret.
	if code, out := do(t, srv, "POST", "/v1/guilds/"+guildA+"/game/rounds", key, `{"week":"2026-10-26","question":"La del bot"}`); code != 200 || out["proposed"] != true {
		t.Fatalf("start a proposed round: %d %v", code, out)
	}
	_, board = get(t, m, srv.URL+"/web/guilds/"+guildA+"/game")
	cur = board["current"].(map[string]any)
	mine := cur["question"] == "¿Tu primer concierto?"
	if !mine && cur["question"] != "¿Tu olor favorito?" || cur["proposed"] != true || cur["my_question"] != mine || cur["proposed_by"] != nil {
		t.Fatalf("proposed round: %v", cur)
	}
	if p := board["proposal"].(map[string]any); p["open"] != false || p["week"] != "2026-11-02" || p["count"].(float64) != 0 {
		t.Fatalf("next week's proposals: %v", p)
	}

	// Once it is over, the proposer comes out with the results.
	now = time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	_, board = get(t, m, srv.URL+"/web/guilds/"+guildA+"/game")
	by, _ := board["previous"].(map[string]any)["proposed_by"].(map[string]any)
	if want := map[bool]string{true: memberID, false: p2}[mine]; by["discord_user_id"] != want {
		t.Fatalf("proposer after the round: %v, want %s", board["previous"], want)
	}
}
