package http

import (
	"net/http"

	"github.com/happyx-app/HappyX.Api/internal/domain"
	"github.com/happyx-app/HappyX.Api/internal/usecase"
)

// The public demo shows the dashboard of a made-up server to anyone, without logging in. Its
// routes mirror the dashboard's read routes under /web/demo/{lang}, but they run on their own
// Service, which only knows the demo server (see repository/demo), and are always seen as the
// demo's admin. They never reach the real servers' data, and the demo has no write routes.

// Demo is the demo server, one Service per language, and who the demo is seen as.
type Demo struct {
	Services map[string]*usecase.Service // by language code, e.g. "en"
	Viewer   domain.Viewer
}

// WithDemo turns on the public demo. It needs WithWeb first.
func (s *Server) WithDemo(d Demo) *Server {
	s.demos = map[string]*Server{}
	for lang, svc := range d.Services {
		s.demos[lang] = &Server{svc: svc, log: s.log, web: s.web, metrics: s.metrics}
	}
	s.demoViewer = d.Viewer
	return s
}

type demoHandler func(s *Server, w http.ResponseWriter, r *http.Request, v domain.Viewer)

func (s *Server) demoRoutes(mux *http.ServeMux) {
	mux.Handle("GET /web/demo/{lang}/me", s.asDemoViewer((*Server).webMe))
	mux.Handle("GET /web/demo/{lang}/me/moods", s.asDemoViewer((*Server).webMyMoods))
	mux.Handle("GET /web/demo/{lang}/guilds/{guildID}/report", s.asDemoViewer((*Server).webReport))
	mux.Handle("GET /web/demo/{lang}/guilds/{guildID}/health", s.asDemoViewer((*Server).webHealth))
	mux.Handle("GET /web/demo/{lang}/guilds/{guildID}/roles", s.asDemoViewer((*Server).webRoles))
	mux.Handle("GET /web/demo/{lang}/guilds/{guildID}/channels", s.asDemoViewer((*Server).webChannels))
	mux.Handle("GET /web/demo/{lang}/guilds/{guildID}/access", s.asDemoViewer((*Server).webAccess))
	mux.Handle("GET /web/demo/{lang}/guilds/{guildID}/game", s.asDemoViewer((*Server).webGame))
}

// asDemoViewer runs a dashboard handler on the demo in the path's language, as the demo's
// admin. No cookie is read or set.
func (s *Server) asDemoViewer(next demoHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d := s.demos[r.PathValue("lang")]
		if d == nil {
			writeError(w, http.StatusNotFound, "no demo in that language")
			return
		}
		// The demo is the same for everyone and changes once a day at most.
		w.Header().Set("Cache-Control", "public, max-age=300")
		next(d, w, r, s.demoViewer)
	})
}
