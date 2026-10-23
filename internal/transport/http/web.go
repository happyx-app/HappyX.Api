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
)

// The web dashboard's routes live under /web. The dashboard's nginx forwards
// https://<web>/api/web/... here, so the browser only ever talks to the web's origin and the
// session cookie is first-party.

// Identity logs users in with Discord.
type Identity interface {
	AuthURL(state string) string
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
	mux.HandleFunc("GET /web/auth/callback", s.webCallback)
	mux.HandleFunc("POST /web/auth/logout", s.webLogout)
	mux.Handle("GET /web/me", s.requireViewer(s.webMe))
	mux.Handle("GET /web/me/moods", s.requireViewer(s.webMyMoods))
	mux.Handle("GET /web/guilds/{guildID}/report", s.requireViewer(s.webReport))
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
	out := struct {
		User   viewerDTO  `json:"user"`
		Guilds []guildDTO `json:"guilds"`
	}{viewerDTO{v.DiscordID, v.Username, v.Avatar}, []guildDTO{}}
	for _, g := range gs {
		out.Guilds = append(out.Guilds, toGuildDTO(g))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) webReport(w http.ResponseWriter, r *http.Request, v domain.Viewer) {
	g, err := s.svc.ViewerGuild(r.Context(), v, r.PathValue("guildID"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeReport(w, r, g)
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
