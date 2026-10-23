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

	"github.com/moixblau/HappyX.Api/internal/repository/postgres"
	httpapi "github.com/moixblau/HappyX.Api/internal/transport/http"
	"github.com/moixblau/HappyX.Api/internal/usecase"
)

const key = "test-key"

func newTestServer(t *testing.T) *httptest.Server {
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
	if _, err := pool.Exec(context.Background(), `TRUNCATE guilds, members, mood_entries, dispatches RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	svc := usecase.NewService(postgres.NewGuildRepo(pool), postgres.NewMemberRepo(pool), postgres.NewMoodRepo(pool), postgres.NewDispatchRepo(pool),
		func() time.Time { return time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC) })
	sum := sha256.Sum256([]byte(key))
	h := httpapi.NewServer(svc, [][]byte{sum[:]}, slog.New(slog.NewTextHandler(io.Discard, nil))).Handler()
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
