package http_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/happyx-app/HappyX.Api/internal/repository/postgres"
	httpapi "github.com/happyx-app/HappyX.Api/internal/transport/http"
	"github.com/happyx-app/HappyX.Api/internal/usecase"
)

const key = "test-key"

func newTestServer(t *testing.T, opts ...func(*httpapi.Server)) *httptest.Server {
	t.Helper()
	return newTestServerClock(t, func() time.Time { return time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC) }, opts...)
}

func newTestServerClock(t *testing.T, now usecase.Clock, opts ...func(*httpapi.Server)) *httptest.Server {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	pool, err := postgres.Connect(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(context.Background(), `TRUNCATE guilds, guild_removals, members, mood_entries, dispatches, kudos, game_rounds, game_answers, game_guesses RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	svc := usecase.NewService(postgres.NewGuildRepo(pool), postgres.NewMemberRepo(pool), postgres.NewMoodRepo(pool), postgres.NewDispatchRepo(pool), postgres.NewKudosRepo(pool), postgres.NewGameRepo(pool), postgres.NewAccessRepo(pool), postgres.NewPrivacyRepo(pool), now)
	sum := sha256.Sum256([]byte(key))
	api := httpapi.NewServer(svc, [][]byte{sum[:]}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	for _, opt := range opts {
		opt(api)
	}
	h := api.Handler()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func do(t *testing.T, srv *httptest.Server, method, path, auth, body string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
	if auth != "" {
		req.Header.Set("Authorization", "Bearer "+auth)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

func TestAuth(t *testing.T) {
	srv := newTestServer(t)
	if code, _ := do(t, srv, "GET", "/health", "", ""); code != 200 {
		t.Fatalf("health should be public, got %d", code)
	}
	if code, _ := do(t, srv, "GET", "/v1/guilds", "", ""); code != 401 {
		t.Fatalf("no key: got %d, want 401", code)
	}
	if code, _ := do(t, srv, "GET", "/v1/guilds", "wrong", ""); code != 401 {
		t.Fatalf("wrong key: got %d, want 401", code)
	}
}

func TestMoodFlow(t *testing.T) {
	srv := newTestServer(t)
	const g, u = "100000000000000001", "200000000000000001"

	steps := []struct {
		method, path, body string
		want               int
	}{
		{"POST", "/v1/guilds", `{"discord_guild_id":"` + g + `","name":"Test","admin_discord_user_id":"300000000000000001"}`, 200},
		{"PUT", "/v1/guilds/" + g + "/members/" + u, `{"username":"ana"}`, 200},
		{"PUT", "/v1/guilds/" + g + "/members/" + u + "/mood", `{"mood":4,"message":"bien"}`, 200},
		{"PUT", "/v1/guilds/" + g + "/members/" + u + "/mood", `{"mood":9}`, 400},
		{"PATCH", "/v1/guilds/" + g, `{"survey_hour":25}`, 400},
		{"PATCH", "/v1/guilds/" + g, `{"language":"pt-BR"}`, 200},
		{"PATCH", "/v1/guilds/" + g, `{"language":"klingon"}`, 400},
		{"GET", "/v1/guilds/999999999999999999", ``, 404},
		{"POST", "/v1/guilds/" + g + "/dispatches", `{"kind":"survey","period":"2026-10-22"}`, 201},
		{"POST", "/v1/guilds/" + g + "/dispatches", `{"kind":"survey","period":"2026-10-22"}`, 409},
	}
	for _, s := range steps {
		if code, body := do(t, srv, s.method, s.path, key, s.body); code != s.want {
			t.Fatalf("%s %s: got %d (%v), want %d", s.method, s.path, code, body, s.want)
		}
	}

	code, rep := do(t, srv, "GET", "/v1/guilds/"+g+"/report?from=2026-10-22&to=2026-10-22", key, "")
	if code != 200 || rep["responses"].(float64) != 1 || rep["average"].(float64) != 4 {
		t.Fatalf("report: %d %v", code, rep)
	}
	msgs := rep["messages"].([]any)
	if m := msgs[0].(map[string]any); m["discord_user_id"] != nil || m["message"] != "bien" {
		t.Fatalf("anonymous report should hide the author: %v", m)
	}
}

func TestKudosFlow(t *testing.T) {
	srv := newTestServer(t)
	const g, ana, bea, eve = "100000000000000001", "200000000000000001", "200000000000000002", "200000000000000009"
	kudos := func(from, to, msg string) string {
		return `{"from_discord_user_id":"` + from + `","to_discord_user_id":"` + to + `","message":"` + msg + `"}`
	}

	steps := []struct {
		method, path, body string
		want               int
	}{
		{"POST", "/v1/guilds", `{"discord_guild_id":"` + g + `","name":"Test","admin_discord_user_id":"300000000000000001"}`, 200},
		{"PUT", "/v1/guilds/" + g + "/members/" + ana, `{"username":"ana"}`, 200},
		{"PUT", "/v1/guilds/" + g + "/members/" + bea, `{"username":"bea","active":false}`, 200},
		{"POST", "/v1/guilds/" + g + "/kudos", kudos(ana, bea, "gracias por el cable"), 201},
		{"POST", "/v1/guilds/" + g + "/kudos", kudos(ana, ana, "yo mismo"), 400},
		{"POST", "/v1/guilds/" + g + "/kudos", kudos(ana, bea, ""), 400},
		{"POST", "/v1/guilds/" + g + "/kudos", kudos(ana, eve, "no está"), 403},
		{"PATCH", "/v1/guilds/" + g, `{"kudos_channel_id":"400000000000000001"}`, 200},
		{"PATCH", "/v1/guilds/" + g, `{"kudos_channel_id":"general"}`, 400},
	}
	for _, s := range steps {
		if code, body := do(t, srv, s.method, s.path, key, s.body); code != s.want {
			t.Fatalf("%s %s: got %d (%v), want %d", s.method, s.path, code, body, s.want)
		}
	}
	if _, gd := do(t, srv, "GET", "/v1/guilds/"+g, key, ""); gd["kudos_channel_id"] != "400000000000000001" {
		t.Fatalf("kudos channel not saved: %v", gd)
	}

	// The daily limit counts the one already given.
	for i := 1; i < 10; i++ {
		if code, body := do(t, srv, "POST", "/v1/guilds/"+g+"/kudos", key, kudos(ana, bea, "otra")); code != 201 {
			t.Fatalf("kudos %d: %d %v", i, code, body)
		}
	}
	if code, _ := do(t, srv, "POST", "/v1/guilds/"+g+"/kudos", key, kudos(ana, bea, "una más")); code != 429 {
		t.Fatalf("11th kudos of the day: got %d, want 429", code)
	}

	code, rep := do(t, srv, "GET", "/v1/guilds/"+g+"/report?from=2026-10-22&to=2026-10-22", key, "")
	if code != 200 {
		t.Fatalf("report: %d %v", code, rep)
	}
	k := rep["kudos"].(map[string]any)
	if k["count"].(float64) != 10 || k["givers"].(float64) != 1 || k["receivers"].(float64) != 1 {
		t.Fatalf("kudos summary: %v", k)
	}
	hl := k["highlights"].([]any)
	if len(hl) != 5 {
		t.Fatalf("want 5 highlights, got %d", len(hl))
	}
	if h := hl[0].(map[string]any); h["from_username"] != "ana" || h["to_username"] != "bea" || h["day"] != "2026-10-22" {
		t.Fatalf("kudos keep their names even in anonymous guilds: %v", h)
	}
}
