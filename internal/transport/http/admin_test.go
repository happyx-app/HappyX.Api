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

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/happyx-app/HappyX.Api/internal/repository/backups"
	"github.com/happyx-app/HappyX.Api/internal/repository/postgres"
	httpapi "github.com/happyx-app/HappyX.Api/internal/transport/http"
	"github.com/happyx-app/HappyX.Api/internal/usecase"
)

const adminKey = "admin-test-key"

// newAdminServers starts the bot API and the admin API on the same database, as main does.
func newAdminServers(t *testing.T, now usecase.Clock) (api, admin *httptest.Server, pool *pgxpool.Pool) {
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
	if _, err := pool.Exec(context.Background(), `TRUNCATE guilds, guild_removals, guild_deletions, members, mood_entries, dispatches, kudos, game_rounds, game_answers, game_guesses RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := usecase.NewService(postgres.NewGuildRepo(pool), postgres.NewMemberRepo(pool), postgres.NewMoodRepo(pool), postgres.NewDispatchRepo(pool), postgres.NewKudosRepo(pool), postgres.NewGameRepo(pool), postgres.NewAccessRepo(pool), postgres.NewPrivacyRepo(pool), now)
	sum := sha256.Sum256([]byte(key))
	main := httpapi.NewServer(svc, [][]byte{sum[:]}, log)
	adminSum := sha256.Sum256([]byte(adminKey))
	a := usecase.NewAdmin(postgres.NewAdminRepo(pool), backups.Dir{}, now)
	api = httptest.NewServer(main.Handler())
	admin = httptest.NewServer(httpapi.NewAdminServer(a, adminSum[:], main.Metrics(), "1.2.3", log).Handler())
	t.Cleanup(api.Close)
	t.Cleanup(admin.Close)
	return api, admin, pool
}

func getRaw(t *testing.T, srv *httptest.Server, path, auth string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest("GET", srv.URL+path, nil)
	if auth != "" {
		req.Header.Set("Authorization", "Bearer "+auth)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

func TestAdmin(t *testing.T) {
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	api, admin, pool := newAdminServers(t, func() time.Time { return now })
	const a, b, c = "100000000000000001", "100000000000000002", "100000000000000003"
	const ana, bea, cid = "200000000000000001", "200000000000000002", "200000000000000003"

	steps := []struct {
		method, path, body string
		want               int
	}{
		{"POST", "/v1/guilds", `{"discord_guild_id":"` + a + `","name":"Alpha","admin_discord_user_id":"` + ana + `"}`, 200},
		{"POST", "/v1/guilds", `{"discord_guild_id":"` + b + `","name":"Beta","admin_discord_user_id":"` + cid + `"}`, 200},
		{"POST", "/v1/guilds", `{"discord_guild_id":"` + c + `","name":"Gamma","admin_discord_user_id":"` + cid + `"}`, 200},
		{"PUT", "/v1/guilds/" + a + "/members/" + ana, `{"username":"ana"}`, 200},
		{"PUT", "/v1/guilds/" + a + "/members/" + bea, `{"username":"bea"}`, 200},
		{"PUT", "/v1/guilds/" + b + "/members/" + cid, `{"username":"cid"}`, 200},
		{"PUT", "/v1/guilds/" + a + "/dispatches/survey/2026-10-22", `{"recipients":2,"failed":1}`, 404}, // not claimed yet
		{"POST", "/v1/guilds/" + a + "/dispatches", `{"kind":"survey","period":"2026-10-22"}`, 201},
		{"PUT", "/v1/guilds/" + a + "/dispatches/survey/2026-10-22", `{"recipients":2,"failed":3}`, 400},
		{"PUT", "/v1/guilds/" + a + "/dispatches/survey/2026-10-22", `{"failed":1}`, 400},
		{"PUT", "/v1/guilds/" + a + "/dispatches/nope/2026-10-22", `{"recipients":2}`, 400},
		{"PUT", "/v1/guilds/" + a + "/dispatches/survey/2026-10-22", `{"recipients":2,"failed":1}`, 204},
		{"PUT", "/v1/guilds/" + a + "/members/" + ana + "/mood", `{"mood":2,"message":"a secret worry","day":"2026-10-22"}`, 200},
		{"DELETE", "/v1/guilds/" + c, ``, 204},
	}
	for _, s := range steps {
		if code, body := do(t, api, s.method, s.path, key, s.body); code != s.want {
			t.Fatalf("%s %s: got %d (%v), want %d", s.method, s.path, code, body, s.want)
		}
	}
	// Beta has had the bot for weeks without an answer.
	if _, err := pool.Exec(context.Background(), `UPDATE guilds SET created_at = '2026-09-01' WHERE discord_id = $1`, b); err != nil {
		t.Fatal(err)
	}

	for _, auth := range []string{"", key, "wrong"} {
		if code, _ := getRaw(t, admin, "/admin/overview", auth); code != 401 {
			t.Fatalf("admin API with key %q: got %d, want 401", auth, code)
		}
	}
	if code, _ := do(t, api, "GET", "/v1/guilds", adminKey, ""); code != 401 {
		t.Fatalf("the admin key must not open the bot API, got %d", code)
	}

	var o struct {
		Guilds   map[string]int `json:"guilds"`
		People   map[string]int `json:"people"`
		Usage    map[string]int `json:"usage"`
		Failed7d map[string]int `json:"failed_7d"`
		Days     []struct {
			Day                                                string
			Answers, WithMessage, Recipients, Answered, Failed int
		} `json:"days"`
		Weeks []map[string]any `json:"weeks"`
	}
	getJSON(t, admin, "/admin/overview", &o)
	if o.Guilds["with_bot"] != 2 || o.Guilds["idle"] != 1 || o.Guilds["left"] != 1 {
		t.Fatalf("guilds: %v", o.Guilds)
	}
	if o.People["members"] != 3 || o.People["active_7d"] != 1 || o.People["active_today"] != 1 {
		t.Fatalf("people: %v", o.People)
	}
	if o.Usage["answers_7d"] != 1 || o.Usage["with_message_7d"] != 1 || o.Failed7d["survey"] != 1 {
		t.Fatalf("usage %v, failed %v", o.Usage, o.Failed7d)
	}
	last := o.Days[len(o.Days)-1]
	if len(o.Days) != 30 || last.Day != "2026-10-22" || last.Recipients != 2 || last.Answered != 1 || last.Failed != 1 {
		t.Fatalf("days: %d, last %+v", len(o.Days), last)
	}
	if len(o.Weeks) != 12 || o.Weeks[11]["week"] != "2026-10-19" {
		t.Fatalf("weeks: %v", o.Weeks)
	}

	code, raw := getRaw(t, admin, "/admin/guilds", adminKey)
	if code != 200 {
		t.Fatalf("guilds: %d %s", code, raw)
	}
	for _, secret := range []string{"secret worry", "\"ana\"", "\"bea\"", "mood"} {
		if strings.Contains(raw, secret) {
			t.Fatalf("the admin API must only give counts, found %q in %s", secret, raw)
		}
	}
	var gs struct {
		Guilds []map[string]any `json:"guilds"`
	}
	_ = json.Unmarshal([]byte(raw), &gs)
	status := map[string]string{}
	for _, g := range gs.Guilds {
		status[g["name"].(string)] = g["status"].(string)
	}
	if status["Alpha"] != "active" || status["Beta"] != "idle" || status["Gamma"] != "left" {
		t.Fatalf("statuses: %v", status)
	}
	if g := gs.Guilds[0]; g["recipients_7d"] != 2.0 || g["answered_7d"] != 1.0 || g["members"] != 2.0 || g["purge_at"] != nil {
		t.Fatalf("Alpha: %v", g)
	}

	var d struct {
		Guild    map[string]any `json:"guild"`
		Settings map[string]any `json:"settings"`
		Weeks    []map[string]any
	}
	getJSON(t, admin, "/admin/guilds/"+a, &d)
	if d.Guild["name"] != "Alpha" || d.Settings["anonymous"] != true || len(d.Weeks) != 8 || d.Weeks[7]["answers"] != 1.0 {
		t.Fatalf("guild detail: %+v", d)
	}
	if code, _ := getRaw(t, admin, "/admin/guilds/999999999999999999", adminKey); code != 404 {
		t.Fatalf("unknown guild: got %d, want 404", code)
	}

	var h struct {
		Bot map[string]any `json:"bot"`
		API struct {
			Version string
			Hours   []map[string]any
		} `json:"api"`
		Database   map[string]any `json:"database"`
		Backups    map[string]any `json:"backups"`
		Deliveries []struct {
			Day    string
			Failed map[string]int
		} `json:"deliveries"`
	}
	getJSON(t, admin, "/admin/health", &h)
	if h.Bot["online"] != true || h.API.Version != "1.2.3" || len(h.API.Hours) != 24 || h.API.Hours[23]["requests"].(float64) < 1 {
		t.Fatalf("health bot %v api %+v", h.Bot, h.API)
	}
	if h.Database["size_bytes"].(float64) <= 0 || h.Database["migration"].(float64) < 15 || h.Backups["configured"] != false {
		t.Fatalf("health database %v backups %v", h.Database, h.Backups)
	}
	if n := len(h.Deliveries); n != 14 || h.Deliveries[n-1].Failed["survey"] != 1 {
		t.Fatalf("deliveries: %+v", h.Deliveries)
	}

	var st map[string]any
	getJSON(t, admin, "/admin/settings", &st)
	if st["guilds"] != 2.0 {
		t.Fatalf("settings: %v", st)
	}
	if lang := st["language"].([]any)[0].(map[string]any); lang["value"] != "en" || lang["guilds"] != 2.0 {
		t.Fatalf("languages: %v", st["language"])
	}
}

func getJSON(t *testing.T, srv *httptest.Server, path string, v any) {
	t.Helper()
	code, raw := getRaw(t, srv, path, adminKey)
	if code != 200 {
		t.Fatalf("GET %s: %d %s", path, code, raw)
	}
	if err := json.Unmarshal([]byte(raw), v); err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
}
