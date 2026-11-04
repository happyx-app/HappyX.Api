package http_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/happyx-app/HappyX.Api/internal/repository/demo"
	httpapi "github.com/happyx-app/HappyX.Api/internal/transport/http"
	"github.com/happyx-app/HappyX.Api/internal/usecase"
)

func TestWebCanvas(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	srv := newTestServerClock(t, func() time.Time { return now }, func(s *httpapi.Server) {
		s.WithWeb(httpapi.Web{Identity: fakeDiscord{}, SessionKey: []byte("0123456789abcdef0123456789abcdef"), PublicURL: "http://happyx.test/"})
	})
	const jsonCT = "application/json"
	const p2, outsider = "200000000000000002", "200000000000000099"
	if code, out := do(t, srv, "POST", "/v1/guilds", key, `{"discord_guild_id":"`+guildA+`","name":"A","admin_discord_user_id":"`+adminA+`"}`); code != 200 {
		t.Fatalf("register: %d %v", code, out)
	}
	for u, name := range map[string]string{adminA: "ana", memberID: "mia", p2: "pau"} {
		if code, out := do(t, srv, "PUT", "/v1/guilds/"+guildA+"/members/"+u, key, `{"username":"`+name+`"}`); code != 200 {
			t.Fatalf("join: %d %v", code, out)
		}
	}
	admin, m, m2, other := newWebClient(t), newWebClient(t), newWebClient(t), newWebClient(t)
	login(t, srv, admin, "adminA")
	login(t, srv, m, "member")
	login(t, srv, m2, "user:"+p2)
	login(t, srv, other, "user:"+outsider)
	canvasURL := srv.URL + "/web/guilds/" + guildA + "/canvas"
	stroke := func(drawing string, color int) string {
		return fmt.Sprintf(`{"drawing_id":"%s","color":%d,"anchor":[0.1,0.5],"points":[0,0,10,-5,20,3]}`, drawing, color)
	}
	add := func(c *http.Client, body string) (int, map[string]any) {
		t.Helper()
		return send(t, c, "POST", canvasURL+"/strokes", jsonCT, body)
	}

	// Every member sees the canvas, even with the game off; outsiders get a hidden server's 404.
	if res, c := get(t, m, canvasURL); res.StatusCode != 200 || c["enabled"] != false || c["can_moderate"] != false {
		t.Fatalf("member, game off: %d %v", res.StatusCode, c)
	}
	if res, _ := get(t, other, canvasURL); res.StatusCode != 404 {
		t.Fatalf("outsider: %d", res.StatusCode)
	}
	res, c := get(t, admin, canvasURL)
	if res.StatusCode != 200 || c["enabled"] != false || c["can_moderate"] != true || len(c["strokes"].([]any)) != 0 {
		t.Fatalf("admin, canvas off: %d %v", res.StatusCode, c)
	}
	if code, _ := add(admin, stroke("a1", 0)); code != 409 {
		t.Fatalf("drawing while off: %d", code)
	}
	if code, g := patchSettings(t, srv, admin, guildA, jsonCT, `{"canvas_enabled":true,"games_enabled":false}`); code != 200 || g["canvas_enabled"] != true {
		t.Fatalf("turn canvas on: %d %v", code, g)
	}

	// Members draw too, with the game off.
	if code, out := add(m, stroke("d1", 1)); code != 201 || out["version"].(float64) != 1 {
		t.Fatalf("member draws: %d %v", code, out)
	}
	if code, out := add(m, stroke("d1", 2)); code != 201 || out["stroke"].(map[string]any)["mine"] != true {
		t.Fatalf("member draws again: %d %v", code, out)
	}
	if code, out := add(m2, stroke("d2", 3)); code != 201 {
		t.Fatalf("second member draws: %d %v", code, out)
	}
	for name, body := range map[string]string{
		"bad colour":     stroke("x", 5),
		"bad drawing id": stroke("no spaces", 0),
		"odd points":     `{"drawing_id":"x","color":0,"anchor":[0.1,0.5],"points":[0,0,1]}`,
		"one point":      `{"drawing_id":"x","color":0,"anchor":[0.1,0.5],"points":[0,0]}`,
		"anchor off":     `{"drawing_id":"x","color":0,"anchor":[1.5,0.5],"points":[0,0,1,1]}`,
		"far point":      `{"drawing_id":"x","color":0,"anchor":[0.1,0.5],"points":[0,0,5000,1]}`,
	} {
		if code, _ := add(m, body); code != 400 {
			t.Fatalf("%s: %d", name, code)
		}
	}
	if code, _ := send(t, m, "POST", canvasURL+"/strokes", "text/plain", stroke("x", 0)); code != 415 {
		t.Fatalf("stroke without JSON content type: %d", code)
	}
	if code, _ := add(other, stroke("x", 0)); code != 404 {
		t.Fatalf("outsider draws: %d", code)
	}

	// Members see who drew only their own; moderators see everyone's names.
	res, c = get(t, m, canvasURL)
	strokes := list(c["strokes"])
	if res.StatusCode != 200 || c["enabled"] != true || c["can_moderate"] != false || len(strokes) != 3 {
		t.Fatalf("member's canvas: %d %v", res.StatusCode, c)
	}
	for _, s := range strokes {
		if _, named := s["author"]; named || s["mine"] != (s["drawing_id"] == "d1") {
			t.Fatalf("member sees: %v", s)
		}
		if pts := s["points"].([]any); len(pts) != 6 || pts[3].(float64) != -5 {
			t.Fatalf("points: %v", s)
		}
	}
	_, c = get(t, admin, canvasURL)
	if a := list(c["strokes"])[2]["author"].(map[string]any); a["username"] != "pau" || a["discord_user_id"] != p2 {
		t.Fatalf("moderator sees: %v", c)
	}

	// Pages ask again with the ETag and get 304 until something changes.
	etag := res.Header.Get("ETag")
	cond := func(c *http.Client) int {
		t.Helper()
		req, _ := http.NewRequest("GET", canvasURL, nil)
		req.Header.Set("If-None-Match", etag)
		r, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		return r.StatusCode
	}
	if etag == "" || cond(m) != 304 {
		t.Fatalf("unchanged canvas: %q", etag)
	}
	if cond(admin) != 200 {
		t.Fatal("a moderator's view has its own ETag")
	}

	// Members erase only their own drawings; moderators any.
	if code, _ := send(t, m, "DELETE", canvasURL+"/drawings/d2", jsonCT, ""); code != 404 {
		t.Fatalf("member erases someone else's: %d", code)
	}
	if code, out := send(t, m, "DELETE", canvasURL+"/drawings/d1", jsonCT, ""); code != 200 || out["version"].(float64) != 4 {
		t.Fatalf("member erases their own: %d %v", code, out)
	}
	if cond(m) != 200 {
		t.Fatal("erasing changes the ETag")
	}
	if code, _ := send(t, admin, "DELETE", canvasURL+"/drawings/d2", jsonCT, ""); code != 200 {
		t.Fatalf("moderator erases: %d", code)
	}
	if code, _ := send(t, admin, "DELETE", canvasURL+"/drawings/d2", jsonCT, ""); code != 404 {
		t.Fatalf("erasing twice: %d", code)
	}

	// Clearing is for moderators.
	if code, _ := add(m2, stroke("d3", 0)); code != 201 {
		t.Fatalf("draw: %d", code)
	}
	if code, _ := send(t, m, "DELETE", canvasURL, jsonCT, ""); code != 404 {
		t.Fatalf("member clears: %d", code)
	}
	if code, _ := send(t, admin, "DELETE", canvasURL, "", ""); code != 415 {
		t.Fatalf("clear without JSON content type: %d", code)
	}
	if code, _ := send(t, admin, "DELETE", canvasURL, jsonCT, ""); code != 200 {
		t.Fatalf("moderator clears: %d", code)
	}
	if _, c := get(t, m, canvasURL); len(c["strokes"].([]any)) != 0 {
		t.Fatalf("after clearing: %v", c)
	}

	// At most a minute's worth of strokes per person.
	for i := 0; i < 60; i++ {
		if code, out := add(m, stroke(fmt.Sprintf("r%d", i), 0)); code != 201 {
			t.Fatalf("stroke %d: %d %v", i, code, out)
		}
	}
	if code, _ := add(m, stroke("r60", 0)); code != 429 {
		t.Fatalf("too many strokes: %d", code)
	}

	// The server export has every drawing with its author.
	_, server := download(t, admin, srv.URL+"/web/guilds/"+guildA+"/export")
	if cs := list(server["canvas_strokes"]); len(cs) != 60 || cs[0]["author"].(map[string]any)["username"] != "mia" {
		t.Fatalf("server export: %v", server["canvas_strokes"])
	}

	// The personal data export has the drawings, and erasing the data erases them.
	_, mine := download(t, m, srv.URL+"/web/me/data")
	if s := list(mine["servers"])[0]; len(s["canvas_strokes"].([]any)) != 60 {
		t.Fatalf("export: %v", s["canvas_strokes"])
	}
	if code, _ := send(t, m, "DELETE", srv.URL+"/web/me/data", jsonCT, `{"confirm_name":"Mia"}`); code != 204 {
		t.Fatalf("erase: %d", code)
	}
	if _, c := get(t, admin, canvasURL); len(c["strokes"].([]any)) != 0 {
		t.Fatalf("after erasing: %v", c)
	}

	// Off: members see nothing, moderators still see what is there to clean up.
	if code, _ := add(m2, stroke("d4", 0)); code != 201 {
		t.Fatalf("draw: %d", code)
	}
	if code, _ := patchSettings(t, srv, admin, guildA, jsonCT, `{"canvas_enabled":false}`); code != 200 {
		t.Fatalf("turn off: %d", code)
	}
	res, c = get(t, m2, canvasURL)
	if c["enabled"] != false || len(c["strokes"].([]any)) != 0 {
		t.Fatalf("member, canvas off: %v", c)
	}
	if etag = res.Header.Get("ETag"); cond(m2) != 304 {
		t.Fatalf("unchanged canvas while off: %q", etag)
	}
	if _, c := get(t, admin, canvasURL); len(c["strokes"].([]any)) != 1 {
		t.Fatalf("moderator, canvas off: %v", c)
	}
}

func TestDemoCanvas(t *testing.T) {
	now := func() time.Time { return time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC) }
	srv := newTestServerClock(t, now, func(s *httpapi.Server) {
		s.WithWeb(httpapi.Web{Identity: fakeDiscord{}, SessionKey: []byte("0123456789abcdef0123456789abcdef"), PublicURL: "http://happyx.test/"})
		s.WithDemo(httpapi.Demo{Services: map[string]*usecase.Service{"es": demo.NewStore("es", now).NewService()}, Viewer: demo.Viewer()})
	})
	res, err := http.Get(srv.URL + "/web/demo/es/guilds/100000000000000001/canvas")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var c struct {
		Enabled bool `json:"enabled"`
		Strokes []struct {
			DrawingID string `json:"drawing_id"`
			Points    []int  `json:"points"`
		} `json:"strokes"`
	}
	if err := json.NewDecoder(res.Body).Decode(&c); err != nil || res.StatusCode != 200 || !c.Enabled || len(c.Strokes) < 10 {
		t.Fatalf("demo canvas: %d %v %+v", res.StatusCode, err, c)
	}
	for _, s := range c.Strokes {
		if !strings.HasPrefix(s.DrawingID, "demo-") || len(s.Points) < 4 {
			t.Fatalf("demo stroke: %+v", s)
		}
	}
	if res, _ := http.Post(srv.URL+"/web/demo/es/guilds/100000000000000001/canvas/strokes", "application/json", strings.NewReader("{}")); res.StatusCode != 404 && res.StatusCode != 405 {
		t.Fatalf("demo has no write routes: %d", res.StatusCode)
	}
}
