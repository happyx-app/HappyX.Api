package http

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/moixblau/HappyX.Api/internal/domain"
	"github.com/moixblau/HappyX.Api/internal/usecase"
)

// The web dashboard's routes live under /web. The dashboard's nginx forwards
// https://<web>/api/web/... here, so the browser only ever talks to the web's origin and the
// session cookie is first-party.

// Identity logs users in with Discord.
type Identity interface {
	AuthURL(state string) string
	InviteURL() string
	Viewer(ctx context.Context, code string) (domain.Viewer, error)
}

type Web struct {
	Identity   Identity
	SessionKey []byte
	// PublicURL is the dashboard's URL, e.g. https://happyx.example.com. Users land there
	// after logging in or out.
	PublicURL  string
	SessionTTL time.Duration
	Now        func() time.Time
}

const (
	sessionCookie = "happyx_session"
	stateCookie   = "happyx_oauth_state"
)

// WithWeb enables the dashboard routes.
func (s *Server) WithWeb(w Web) *Server {
	if w.SessionTTL == 0 {
		w.SessionTTL = 12 * time.Hour
	}
	if w.Now == nil {
		w.Now = time.Now
	}
	w.PublicURL = strings.TrimRight(w.PublicURL, "/")
	s.web = &w
	return s
}

func (s *Server) webRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /web/auth/login", s.webLogin)
	mux.HandleFunc("GET /web/auth/invite", s.webInvite)
	mux.HandleFunc("GET /web/auth/callback", s.webCallback)
	mux.HandleFunc("POST /web/auth/logout", s.webLogout)
	mux.Handle("GET /web/me", s.requireViewer(s.webMe))
	mux.Handle("GET /web/me/moods", s.requireViewer(s.webMyMoods))
	mux.Handle("GET /web/me/data", s.requireViewer(s.webMyData))
	mux.Handle("DELETE /web/me/data", s.requireViewer(s.webEraseMyData))
	mux.Handle("GET /web/guilds/{guildID}/report", s.requireViewer(s.webReport))
	mux.Handle("PATCH /web/guilds/{guildID}/settings", s.requireViewer(s.webUpdateSettings))
	mux.Handle("DELETE /web/guilds/{guildID}", s.requireViewer(s.webDeleteGuild))
	mux.Handle("GET /web/guilds/{guildID}/access", s.requireViewer(s.webAccess))
	mux.Handle("PUT /web/guilds/{guildID}/access", s.requireViewer(s.webSetAccess))
	mux.Handle("GET /web/guilds/{guildID}/export", s.requireViewer(s.webGuildExport))
	mux.Handle("GET /web/guilds/{guildID}/game", s.requireViewer(s.webGame))
	mux.Handle("PUT /web/guilds/{guildID}/game/answer", s.requireViewer(s.webAnswerGame))
	mux.Handle("PUT /web/guilds/{guildID}/game/guesses", s.requireViewer(s.webGuessGame))
	mux.Handle("PUT /web/guilds/{guildID}/game/proposal", s.requireViewer(s.webProposeQuestion))
}

func (s *Server) secureCookies() bool { return strings.HasPrefix(s.web.PublicURL, "https://") }

func (s *Server) setCookie(w http.ResponseWriter, name, value string, maxAge time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name: name, Value: value, Path: "/", MaxAge: int(maxAge.Seconds()),
		HttpOnly: true, Secure: s.secureCookies(), SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name: name, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: s.secureCookies(), SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) webLogin(w http.ResponseWriter, r *http.Request) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		s.fail(w, r, err)
		return
	}
	state := hex.EncodeToString(b)
	s.setCookie(w, stateCookie, state, 10*time.Minute)
	http.Redirect(w, r, s.web.Identity.AuthURL(state), http.StatusFound)
}

// webInvite sends the landing page's "Add to Discord" button to Discord, which needs no login.
func (s *Server) webInvite(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, s.web.Identity.InviteURL(), http.StatusFound)
}

func (s *Server) webCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	c, err := r.Cookie(stateCookie)
	s.clearCookie(w, stateCookie)
	if err != nil || q.Get("state") == "" || subtle.ConstantTimeCompare([]byte(c.Value), []byte(q.Get("state"))) != 1 {
		http.Redirect(w, r, s.web.PublicURL+"/?login=failed", http.StatusFound)
		return
	}
	if q.Get("code") == "" { // the user cancelled on Discord
		http.Redirect(w, r, s.web.PublicURL+"/", http.StatusFound)
		return
	}
	v, err := s.web.Identity.Viewer(r.Context(), q.Get("code"))
	if err != nil {
		s.log.Warn("discord login failed", "err", err)
		http.Redirect(w, r, s.web.PublicURL+"/?login=failed", http.StatusFound)
		return
	}
	s.setCookie(w, sessionCookie, signSession(s.web.SessionKey, v, s.web.Now().Add(s.web.SessionTTL)), s.web.SessionTTL)
	http.Redirect(w, r, s.web.PublicURL+"/", http.StatusFound)
}

func (s *Server) webLogout(w http.ResponseWriter, _ *http.Request) {
	s.clearCookie(w, sessionCookie)
	w.WriteHeader(http.StatusNoContent)
}

type viewerHandler func(w http.ResponseWriter, r *http.Request, v domain.Viewer)

func (s *Server) requireViewer(next viewerHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookie)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "not logged in")
			return
		}
		v, err := verifySession(s.web.SessionKey, c.Value, s.web.Now())
		if err != nil {
			s.clearCookie(w, sessionCookie)
			writeError(w, http.StatusUnauthorized, "not logged in")
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		next(w, r, v)
	})
}

type viewerDTO struct {
	DiscordID string `json:"discord_user_id"`
	Username  string `json:"username"`
	Avatar    string `json:"avatar,omitempty"`
}

func (s *Server) webMe(w http.ResponseWriter, r *http.Request, v domain.Viewer) {
	gs, err := s.svc.VisibleGuilds(r.Context(), v)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	games, err := s.svc.GameGuilds(r.Context(), v)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	// Games are open to every member, so they only name the server, not its settings.
	type gameGuildDTO struct {
		DiscordID string `json:"discord_guild_id"`
		Name      string `json:"name"`
	}
	// access is "view" (reports), "manage" (reports and settings) or "owner" (also who has
	// access, and deleting the server).
	type myGuildDTO struct {
		guildDTO
		Access string `json:"access"`
	}
	out := struct {
		User   viewerDTO      `json:"user"`
		Guilds []myGuildDTO   `json:"guilds"`
		Games  []gameGuildDTO `json:"games"`
	}{viewerDTO{v.DiscordID, v.Username, v.Avatar}, []myGuildDTO{}, []gameGuildDTO{}}
	for _, g := range gs {
		out.Guilds = append(out.Guilds, myGuildDTO{toGuildDTO(g.Guild), g.Level.String()})
	}
	for _, g := range games {
		out.Games = append(out.Games, gameGuildDTO{g.DiscordID, g.Name})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) webReport(w http.ResponseWriter, r *http.Request, v domain.Viewer) {
	g, err := s.svc.ViewerGuild(r.Context(), v, r.PathValue("guildID"), domain.AccessView)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeReport(w, r, g)
}

// webUpdateSettings changes a server's settings from the dashboard: the same options as the
// bot's /happyx config, except who receives the reports (see webSetAccess). It needs the
// "manage" level.
func (s *Server) webUpdateSettings(w http.ResponseWriter, r *http.Request, v domain.Viewer) {
	if !requireJSON(w, r) {
		return
	}
	g, err := s.svc.ViewerGuild(r.Context(), v, r.PathValue("guildID"), domain.AccessManage)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var in struct {
		Anonymous       *bool   `json:"anonymous"`
		Timezone        *string `json:"timezone"`
		Language        *string `json:"language"`
		SurveyHour      *int    `json:"survey_hour"`
		ReportFrequency *string `json:"report_frequency"`
		GamesEnabled    *bool   `json:"games_enabled"`
		WorkingDays     *[]int  `json:"working_days"`
	}
	if !decode(w, r, &in) {
		return
	}
	p := usecase.SettingsPatch{
		Anonymous: in.Anonymous, Timezone: in.Timezone, Language: in.Language, SurveyHour: in.SurveyHour,
		GamesEnabled: in.GamesEnabled,
	}
	if in.ReportFrequency != nil {
		f := domain.ReportFrequency(*in.ReportFrequency)
		p.ReportFrequency = &f
	}
	if !s.patchWorkingDays(w, r, in.WorkingDays, &p) {
		return
	}
	g, err = s.svc.UpdateGuildSettings(r.Context(), g.DiscordID, p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.log.Info("settings changed from the dashboard", "guild", g.DiscordID, "by", v.DiscordID)
	writeJSON(w, http.StatusOK, toGuildDTO(g))
}

// webDeleteGuild deletes a server and all its data from HappyX. The body must name the server
// ({"confirm_name": "..."}) so a stray request can't delete it. The bot leaves the server shortly
// after.
func (s *Server) webDeleteGuild(w http.ResponseWriter, r *http.Request, v domain.Viewer) {
	if !requireJSON(w, r) {
		return
	}
	var in struct {
		ConfirmName string `json:"confirm_name"`
	}
	if !decode(w, r, &in) {
		return
	}
	guildID := r.PathValue("guildID")
	if err := s.svc.DeleteGuild(r.Context(), v, guildID, in.ConfirmName); err != nil {
		s.fail(w, r, err)
		return
	}
	s.log.Info("guild deleted from the dashboard", "guild", guildID, "by", v.DiscordID)
	w.WriteHeader(http.StatusNoContent)
}

type grantDTO struct {
	Kind  string `json:"kind"` // "user" or "role"
	ID    string `json:"id"`
	Level string `json:"level"` // "view" or "manage"
}

type accessDTO struct {
	AdminDiscordID string      `json:"admin_discord_user_id"`
	Grants         []grantDTO  `json:"grants"`
	Roles          []roleDTO   `json:"roles"`
	Members        []memberDTO `json:"members"`
}

func toAccessDTO(a usecase.AccessSettings) accessDTO {
	out := accessDTO{AdminDiscordID: a.Guild.AdminDiscordID, Grants: []grantDTO{}, Roles: []roleDTO{}, Members: []memberDTO{}}
	for _, g := range a.Grants {
		out.Grants = append(out.Grants, grantDTO{Kind: string(g.Kind), ID: g.TargetID, Level: g.Level.String()})
	}
	for _, ro := range a.Roles {
		out.Roles = append(out.Roles, roleDTO{DiscordID: ro.DiscordID, Name: ro.Name, Color: ro.Color, Position: ro.Position})
	}
	for _, m := range a.Members {
		out.Members = append(out.Members, memberDTO{DiscordID: m.DiscordID, Username: m.Username, Active: m.Active, CreatedAt: m.CreatedAt})
	}
	return out
}

// webAccess returns who has access to a server, with the roles and members to choose from.
// Only owners (the main admin, the server's owner and its Discord Administrators) may see it.
func (s *Server) webAccess(w http.ResponseWriter, r *http.Request, v domain.Viewer) {
	a, err := s.svc.GuildAccessSettings(r.Context(), v, r.PathValue("guildID"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toAccessDTO(a))
}

// webSetAccess changes the main admin, who receives the reports, and replaces the grants.
func (s *Server) webSetAccess(w http.ResponseWriter, r *http.Request, v domain.Viewer) {
	if !requireJSON(w, r) {
		return
	}
	var in struct {
		AdminDiscordID string     `json:"admin_discord_user_id"`
		Grants         []grantDTO `json:"grants"`
	}
	if !decode(w, r, &in) {
		return
	}
	grants := make([]domain.AccessGrant, 0, len(in.Grants))
	for _, g := range in.Grants {
		level, err := domain.ParseGrantLevel(g.Level)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		grants = append(grants, domain.AccessGrant{Kind: domain.GrantKind(g.Kind), TargetID: g.ID, Level: level})
	}
	a, err := s.svc.SetGuildAccess(r.Context(), v, r.PathValue("guildID"), in.AdminDiscordID, grants)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.log.Info("access changed from the dashboard", "guild", a.Guild.DiscordID, "by", v.DiscordID,
		"admin", a.Guild.AdminDiscordID, "grants", len(a.Grants))
	writeJSON(w, http.StatusOK, toAccessDTO(a))
}

// requireJSON guards the dashboard's writes. A JSON body can't be sent cross-site without a CORS
// preflight, which this API never grants; together with the SameSite cookie this keeps other
// sites from writing on a viewer's behalf.
func requireJSON(w http.ResponseWriter, r *http.Request) bool {
	if ct, _, _ := strings.Cut(r.Header.Get("Content-Type"), ";"); strings.TrimSpace(ct) != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return false
	}
	return true
}

type evolutionDTO struct {
	DiscordGuildID string      `json:"discord_guild_id"`
	Name           string      `json:"name"`
	Timezone       string      `json:"timezone"`
	Responses      int         `json:"responses"`
	Average        float64     `json:"average"`
	Streak         int         `json:"streak"`
	Entries        []myMoodDTO `json:"entries"`
}

type myMoodDTO struct {
	Day     string `json:"day"`
	Mood    int    `json:"mood"`
	Message string `json:"message,omitempty"`
}

// webMyMoods returns the viewer's own answers in every server for ?from=&to= (YYYY-MM-DD),
// by default the last 30 days.
func (s *Server) webMyMoods(w http.ResponseWriter, r *http.Request, v domain.Viewer) {
	q := r.URL.Query()
	to := domain.TruncateDay(s.web.Now().UTC())
	from := to.AddDate(0, 0, -29)
	if q.Get("from") != "" || q.Get("to") != "" {
		var err1, err2 error
		from, err1 = time.Parse(dateLayout, q.Get("from"))
		to, err2 = time.Parse(dateLayout, q.Get("to"))
		if err1 != nil || err2 != nil {
			writeError(w, http.StatusBadRequest, "from and to must both be YYYY-MM-DD")
			return
		}
	}
	evs, err := s.svc.Evolution(r.Context(), v, from, to)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := struct {
		From   string         `json:"from"`
		To     string         `json:"to"`
		Guilds []evolutionDTO `json:"guilds"`
	}{from.Format(dateLayout), to.Format(dateLayout), []evolutionDTO{}}
	for _, ev := range evs {
		d := evolutionDTO{
			DiscordGuildID: ev.Guild.DiscordID, Name: ev.Guild.Name, Timezone: ev.Guild.Settings.Timezone,
			Responses: ev.Responses, Average: ev.Average, Streak: ev.Streak, Entries: []myMoodDTO{},
		}
		for _, e := range ev.Entries {
			d.Entries = append(d.Entries, myMoodDTO{Day: e.Day.Format(dateLayout), Mood: int(e.Mood), Message: e.Message})
		}
		out.Guilds = append(out.Guilds, d)
	}
	writeJSON(w, http.StatusOK, out)
}
