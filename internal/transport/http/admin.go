package http

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/happyx-app/HappyX.Api/internal/domain"
	"github.com/happyx-app/HappyX.Api/internal/usecase"
)

// AdminServer is the owner's admin dashboard API (HappyX.Admin). It listens on its own port,
// which is never published, and takes its own key: neither the bot's key nor a dashboard
// session works here, and the admin key works nowhere else. It only answers counts, never a
// mood, a message or a member's name.
type AdminServer struct {
	admin   *usecase.Admin
	keyHash [][]byte
	metrics *Metrics
	version string
	log     *slog.Logger
}

// BotOnlineWithin is how recently the bot must have called for the dashboard to show it online.
// It calls every minute.
const BotOnlineWithin = 5 * time.Minute

func NewAdminServer(admin *usecase.Admin, keyHash []byte, metrics *Metrics, version string, log *slog.Logger) *AdminServer {
	return &AdminServer{admin: admin, keyHash: [][]byte{keyHash}, metrics: metrics, version: version, log: log}
}

func (s *AdminServer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /admin/overview", s.overview)
	mux.HandleFunc("GET /admin/guilds", s.guilds)
	mux.HandleFunc("GET /admin/guilds/{guildID}", s.guild)
	mux.HandleFunc("GET /admin/health", s.health)
	mux.HandleFunc("GET /admin/settings", s.settings)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || key == "" || !validKey(s.keyHash, key) {
			writeError(w, http.StatusUnauthorized, "missing or invalid admin key")
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		mux.ServeHTTP(w, r)
	})
}

func (s *AdminServer) fail(w http.ResponseWriter, r *http.Request, err error) {
	(&Server{log: s.log}).fail(w, r, err)
}

// --- responses ---

type adminWeekJSON struct {
	Week       string `json:"week"`
	Recipients int    `json:"recipients"`
	Answered   int    `json:"answered"`
	Answers    int    `json:"answers"`
	Joins      int    `json:"joins,omitempty"`
	Leaves     int    `json:"leaves,omitempty"`
	Guilds     int    `json:"guilds,omitempty"`
}

func adminWeeks(ws []domain.AdminWeek) []adminWeekJSON {
	out := make([]adminWeekJSON, len(ws))
	for i, w := range ws {
		out[i] = adminWeekJSON{w.Week.Format(dateLayout), w.Recipients, w.Answered, w.Answers, w.Joins, w.Leaves, w.Guilds}
	}
	return out
}

type adminGuildJSON struct {
	DiscordID  string  `json:"discord_guild_id"`
	Name       string  `json:"name"`
	Status     string  `json:"status"`
	CreatedAt  string  `json:"created_at"`
	LeftAt     *string `json:"left_at"`
	PurgeAt    *string `json:"purge_at"`
	Language   string  `json:"language"`
	Timezone   string  `json:"timezone"`
	Members    int     `json:"members"`
	OptedOut   int     `json:"opted_out"`
	Active7d   int     `json:"active_7d"`
	LastAnswer *string `json:"last_answer"`
	Recipients int     `json:"recipients_7d"`
	Answered   int     `json:"answered_7d"`
	Failed7d   int     `json:"failed_7d"`
}

func timePtr(t *time.Time, layout string) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format(layout)
	return &s
}

func adminGuild(g domain.AdminGuild) adminGuildJSON {
	return adminGuildJSON{
		DiscordID: g.DiscordID, Name: g.Name, Status: string(g.Status),
		CreatedAt: g.CreatedAt.UTC().Format(time.RFC3339), LeftAt: timePtr(g.LeftAt, time.RFC3339), PurgeAt: timePtr(g.PurgeAt, time.RFC3339),
		Language: g.Language, Timezone: g.Timezone, Members: g.Members, OptedOut: g.OptedOut, Active7d: g.Active7d,
		LastAnswer: timePtr(g.LastAnswer, dateLayout), Recipients: g.Recipients, Answered: g.Answered, Failed7d: g.Failed7d,
	}
}

func counts(cs []domain.Count) []map[string]any {
	out := make([]map[string]any, len(cs))
	for i, c := range cs {
		out[i] = map[string]any{"value": c.Value, "guilds": c.N}
	}
	return out
}

func kindCounts(m map[domain.DispatchKind]int) map[string]int {
	out := map[string]int{"survey": 0, "report": 0, "game": 0, "game_guess": 0}
	for k, n := range m {
		out[string(k)] = n
	}
	return out
}

// --- handlers ---

func (s *AdminServer) overview(w http.ResponseWriter, r *http.Request) {
	o, err := s.admin.Overview(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	days := make([]map[string]any, len(o.Days))
	for i, d := range o.Days {
		days[i] = map[string]any{"day": d.Day.Format(dateLayout), "answers": d.Answers, "with_message": d.WithMessage,
			"recipients": d.Recipients, "answered": d.Answered, "failed": d.Failed}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"generated_at": s.admin.Now().UTC().Format(time.RFC3339),
		"guilds": map[string]int{"with_bot": o.Guilds.WithBot, "idle": o.Guilds.Idle, "left": o.Guilds.Left,
			"deleted": o.Guilds.Deleted, "new_7d": o.Guilds.NewWeek, "left_7d": o.Guilds.LeftWeek},
		"people": map[string]int{"members": o.People.Members, "opted_out": o.People.OptedOut, "active_today": o.People.Today,
			"active_7d": o.People.Week, "active_30d": o.People.Month, "repeat_7d": o.People.Repeat},
		"usage": map[string]int{"answers_7d": o.Usage.Answers7d, "with_message_7d": o.Usage.WithMessage7d, "kudos_7d": o.Usage.Kudos7d,
			"games_on": o.Usage.GamesOn, "proposals_7d": o.Usage.Proposals7d, "game_answers_7d": o.Usage.GameAnswers7d},
		// The market study's Phase 0 goal counts active servers.
		"phase0":    map[string]int{"active_guilds": o.Phase0.ActiveGuilds, "over_free": o.Phase0.OverFree},
		"failed_7d": kindCounts(o.Failed7d),
		"days":      days,
		"weeks":     adminWeeks(o.Weeks),
	})
}

func (s *AdminServer) guilds(w http.ResponseWriter, r *http.Request) {
	gs, err := s.admin.Guilds(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]adminGuildJSON, len(gs))
	for i, g := range gs {
		out[i] = adminGuild(g)
	}
	writeJSON(w, http.StatusOK, map[string]any{"guilds": out})
}

func (s *AdminServer) guild(w http.ResponseWriter, r *http.Request) {
	d, err := s.admin.Guild(r.Context(), r.PathValue("guildID"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	st := d.Settings
	writeJSON(w, http.StatusOK, map[string]any{
		"guild": adminGuild(d.AdminGuild),
		"settings": map[string]any{
			"anonymous": st.Anonymous, "survey_hour": st.SurveyHour, "working_days": st.WorkingDays.ISO(),
			"report_frequency": st.ReportFrequency, "report_dm": st.ReportDM, "custom_message": st.CustomMessage,
			"survey_roles": st.SurveyRoles, "game_roles": st.GameRoles, "games_enabled": st.GamesEnabled, "kudos_channel": st.KudosChannel,
		},
		"weeks":            adminWeeks(d.Weeks),
		"kudos_7d":         d.Kudos7d,
		"answers_30d":      d.Answers30d,
		"with_message_30d": d.WithMessage,
		"last_report":      timePtr(d.LastReport, dateLayout),
		"game_answers_7d":  d.GameAnswers7,
	})
}

func (s *AdminServer) health(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	db, err := s.admin.Database(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	deliveries, err := s.admin.Deliveries(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	backups, configured, err := s.admin.Backups(ctx)
	if err != nil {
		// A folder that can't be read shouldn't hide the rest of the page.
		s.log.Error("list backups", "err", err)
	}

	now := s.admin.Now()
	var lastBot *string
	if t := s.metrics.LastBot(); !t.IsZero() {
		lastBot = timePtr(&t, time.RFC3339)
	}
	hours := []map[string]any{}
	for _, h := range s.metrics.Hours() {
		hours = append(hours, map[string]any{"hour": h.Hour.Format(time.RFC3339), "requests": h.Requests, "errors": h.Errors,
			"p50_ms": h.P50.Seconds() * 1000, "p95_ms": h.P95.Seconds() * 1000})
	}
	days := make([]map[string]any, len(deliveries))
	for i, d := range deliveries {
		days[i] = map[string]any{"day": d.Day.Format(dateLayout), "sent": kindCounts(d.Sent), "failed": kindCounts(d.Failed)}
	}
	files := make([]map[string]any, len(backups))
	for i, b := range backups {
		files[i] = map[string]any{"name": b.Name, "size_bytes": b.Size, "modified": b.Modified.Format(time.RFC3339)}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"generated_at": now.UTC().Format(time.RFC3339),
		"bot":          map[string]any{"last_seen": lastBot, "online": lastBot != nil && s.metrics.now().Sub(s.metrics.LastBot()) <= BotOnlineWithin},
		"api":          map[string]any{"version": s.version, "started_at": s.metrics.Started().UTC().Format(time.RFC3339), "hours": hours},
		"deliveries":   days,
		"database": map[string]any{"version": db.Version, "size_bytes": db.SizeBytes, "largest_table": db.LargestTable,
			"largest_table_bytes": db.LargestBytes, "connections": db.Connections, "max_connections": db.MaxConnections, "migration": db.Migration},
		"backups": map[string]any{"configured": configured, "files": files},
	})
}

func (s *AdminServer) settings(w http.ResponseWriter, r *http.Request) {
	st, err := s.admin.Settings(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"guilds": st.Guilds, "language": counts(st.Language), "timezone": counts(st.Timezone), "survey_hour": counts(st.SurveyHour),
		"anonymous": counts(st.Anonymous), "report_frequency": counts(st.ReportFrequency), "report_dm": counts(st.ReportDM),
		"survey_roles": counts(st.SurveyRoles), "extras": counts(st.Extras),
	})
}
