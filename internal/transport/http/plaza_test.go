package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	httpapi "github.com/moixblau/HappyX.Api/internal/transport/http"
)

func dialPlaza(t *testing.T, srvURL string, c *http.Client, guild string, header http.Header) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	u := "ws" + strings.TrimPrefix(srvURL, "http") + "/web/guilds/" + guild + "/plaza"
	return websocket.Dial(ctx, u, &websocket.DialOptions{HTTPClient: c, HTTPHeader: header})
}

func readUntil(t *testing.T, ws *websocket.Conn, typ string) map[string]any {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	for {
		_, b, err := ws.Read(ctx)
		if err != nil {
			t.Fatalf("waiting for %q: %v", typ, err)
		}
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		if m["t"] == typ {
			return m
		}
	}
}

func TestPlaza(t *testing.T) {
	srv := newTestServer(t, func(s *httpapi.Server) {
		s.WithWeb(httpapi.Web{Identity: fakeDiscord{}, SessionKey: []byte("0123456789abcdef0123456789abcdef"), PublicURL: "http://happyx.test/"})
	})
	const p2 = "200000000000000002"
	for _, g := range []struct{ id, admin string }{{guildA, adminA}, {guildB, adminB}} {
		if code, out := do(t, srv, "POST", "/v1/guilds", key, `{"discord_guild_id":"`+g.id+`","name":"x","admin_discord_user_id":"`+g.admin+`"}`); code != 200 {
			t.Fatalf("register: %d %v", code, out)
		}
	}
	for _, u := range []string{memberID, p2} {
		if code, out := do(t, srv, "PUT", "/v1/guilds/"+guildA+"/members/"+u, key, `{"username":"x"}`); code != 200 {
			t.Fatalf("join: %d %v", code, out)
		}
	}
	mia := newWebClient(t)
	login(t, srv, mia, "member")

	// Without a session, or with games off, there is no square.
	if _, res, err := dialPlaza(t, srv.URL, newWebClient(t), guildA, nil); err == nil || res.StatusCode != 401 {
		t.Fatalf("anonymous: %v", err)
	}
	if _, res, err := dialPlaza(t, srv.URL, mia, guildA, nil); err == nil || res.StatusCode != 404 {
		t.Fatalf("games off: %v", err)
	}
	admin := newWebClient(t)
	login(t, srv, admin, "adminA")
	if code, g := patchSettings(t, srv, admin, guildA, "application/json", `{"games_enabled":true}`); code != 200 {
		t.Fatalf("turn games on: %d %v", code, g)
	}
	// The admin isn't a member of the server, so they can't walk in it; nor can a member of
	// another server.
	if _, res, err := dialPlaza(t, srv.URL, admin, guildA, nil); err == nil || res.StatusCode != 404 {
		t.Fatalf("non-member: %v", err)
	}
	if _, res, err := dialPlaza(t, srv.URL, mia, guildB, nil); err == nil || res.StatusCode != 404 {
		t.Fatalf("other server: %v", err)
	}
	// Another site can't open the square with Mia's cookie.
	if _, res, err := dialPlaza(t, srv.URL, mia, guildA, http.Header{"Origin": {"https://evil.test"}}); err == nil || res.StatusCode != 403 {
		t.Fatalf("cross-site: %v", err)
	}

	a, _, err := dialPlaza(t, srv.URL, mia, guildA, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer a.CloseNow()
	w := readUntil(t, a, "welcome")
	if w["you"] != memberID || len(w["players"].([]any)) != 1 {
		t.Fatalf("welcome: %v", w)
	}
	other := newWebClient(t)
	login(t, srv, other, "user:"+p2)
	b, _, err := dialPlaza(t, srv.URL, other, guildA, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer b.CloseNow()
	if j := readUntil(t, a, "join"); j["player"].(map[string]any)["id"] != p2 {
		t.Fatalf("join: %v", j)
	}
	ctx := context.Background()
	_ = b.Write(ctx, websocket.MessageText, []byte(`{"t":"emote","e":"wave"}`))
	if e := readUntil(t, a, "emote"); e["id"] != p2 || e["e"] != "wave" {
		t.Fatalf("emote: %v", e)
	}
	_ = b.Write(ctx, websocket.MessageText, []byte(`{"t":"move","dx":1,"dy":0}`))
	if st := readUntil(t, a, "state"); len(st["p"].([]any)) != 2 {
		t.Fatalf("state: %v", st)
	}

	// Opening the square in a second tab closes the first one.
	a2, _, err := dialPlaza(t, srv.URL, mia, guildA, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer a2.CloseNow()
	rctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	for {
		if _, _, err := a.Read(rctx); err != nil {
			if websocket.CloseStatus(err) != 4000 {
				t.Fatalf("first tab closed with %v", err)
			}
			break
		}
	}
	b.Close(websocket.StatusNormalClosure, "")
	if l := readUntil(t, a2, "leave"); l["id"] != p2 {
		t.Fatalf("leave: %v", l)
	}
}
