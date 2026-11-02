package http

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/happyx-app/HappyX.Api/internal/domain"
)

// --- Team canvas, on the dashboard ---

type canvasAuthorDTO struct {
	DiscordID string `json:"discord_user_id"`
	Username  string `json:"username"`
}

type canvasStrokeDTO struct {
	ID        string     `json:"id"`
	DrawingID string     `json:"drawing_id"`
	Color     int        `json:"color"`
	Anchor    [2]float64 `json:"anchor"`
	// Points are flat px offsets from the anchor: x0, y0, x1, y1...
	Points []int16 `json:"points"`
	Mine   bool    `json:"mine"`
	// Author is only shown to moderators.
	Author    *canvasAuthorDTO `json:"author,omitempty"`
	CreatedAt time.Time        `json:"created_at"`
}

func toCanvasStrokeDTO(s domain.CanvasStroke, viewerID string, moderator bool) canvasStrokeDTO {
	d := canvasStrokeDTO{
		ID: strconv.FormatInt(s.ID, 10), DrawingID: s.DrawingID, Color: s.Color,
		Anchor: [2]float64{s.AnchorX, s.AnchorY}, Points: make([]int16, 0, 2*len(s.Points)),
		Mine: s.AuthorDiscordID == viewerID, CreatedAt: s.CreatedAt,
	}
	for _, p := range s.Points {
		d.Points = append(d.Points, p.X, p.Y)
	}
	if moderator {
		d.Author = &canvasAuthorDTO{s.AuthorDiscordID, s.AuthorUsername}
	}
	return d
}

// canvasETag names one viewer's view of a canvas: the same version looks different to a
// moderator, and while the canvas is off.
func canvasETag(enabled, moderator bool, version int64) string {
	return fmt.Sprintf(`W/"canvas-%d-%t-%t"`, version, enabled, moderator)
}

// webCanvas returns the server's team canvas to whoever can open one of its dashboard pages.
// Pages ask again every so often with If-None-Match, and get 304 when nothing changed.
func (s *Server) webCanvas(w http.ResponseWriter, r *http.Request, v domain.Viewer) {
	guildID := r.PathValue("guildID")
	if inm := r.Header.Get("If-None-Match"); inm != "" {
		enabled, moderator, version, err := s.svc.CanvasVersion(r.Context(), v, guildID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if inm == canvasETag(enabled, moderator, version) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}
	c, err := s.svc.Canvas(r.Context(), v, guildID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := struct {
		Enabled     bool              `json:"enabled"`
		Version     int64             `json:"version"`
		CanModerate bool              `json:"can_moderate"`
		MaxStrokes  int               `json:"max_strokes"`
		Strokes     []canvasStrokeDTO `json:"strokes"`
	}{c.Enabled, c.Version, c.CanModerate, domain.MaxCanvasStrokes, make([]canvasStrokeDTO, 0, len(c.Strokes))}
	for _, st := range c.Strokes {
		out.Strokes = append(out.Strokes, toCanvasStrokeDTO(st, v.DiscordID, c.CanModerate))
	}
	w.Header().Set("ETag", canvasETag(c.Enabled, c.CanModerate, c.Version))
	writeJSON(w, http.StatusOK, out)
}

// webAddCanvasStroke adds one stroke by the viewer. 409 while the canvas is off or full, 429
// for more than domain.MaxCanvasStrokesPerMinute a minute.
func (s *Server) webAddCanvasStroke(w http.ResponseWriter, r *http.Request, v domain.Viewer) {
	if !requireJSON(w, r) {
		return
	}
	var in struct {
		DrawingID string     `json:"drawing_id"`
		Color     int        `json:"color"`
		Anchor    [2]float64 `json:"anchor"`
		Points    []int16    `json:"points"`
	}
	if !decode(w, r, &in) {
		return
	}
	if len(in.Points)%2 != 0 {
		s.fail(w, r, domain.ValidationError{Reason: "points must be x, y pairs"})
		return
	}
	st := domain.CanvasStroke{DrawingID: in.DrawingID, Color: in.Color, AnchorX: in.Anchor[0], AnchorY: in.Anchor[1]}
	for i := 0; i+1 < len(in.Points); i += 2 {
		st.Points = append(st.Points, domain.CanvasPoint{X: in.Points[i], Y: in.Points[i+1]})
	}
	out, version, err := s.svc.AddCanvasStroke(r.Context(), v, r.PathValue("guildID"), st)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, struct {
		Stroke  canvasStrokeDTO `json:"stroke"`
		Version int64           `json:"version"`
	}{toCanvasStrokeDTO(out, v.DiscordID, false), version})
}

// webDeleteCanvasDrawing erases one drawing: the viewer's own, or any for moderators.
func (s *Server) webDeleteCanvasDrawing(w http.ResponseWriter, r *http.Request, v domain.Viewer) {
	if !requireJSON(w, r) {
		return
	}
	guildID := r.PathValue("guildID")
	version, err := s.svc.DeleteCanvasDrawing(r.Context(), v, guildID, r.PathValue("drawingID"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.logCanvasErase(v, guildID, "drawing")
	writeJSON(w, http.StatusOK, map[string]int64{"version": version})
}

// webClearCanvas erases the whole canvas. It needs the "manage" level.
func (s *Server) webClearCanvas(w http.ResponseWriter, r *http.Request, v domain.Viewer) {
	if !requireJSON(w, r) {
		return
	}
	guildID := r.PathValue("guildID")
	version, err := s.svc.ClearCanvas(r.Context(), v, guildID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.logCanvasErase(v, guildID, "all")
	writeJSON(w, http.StatusOK, map[string]int64{"version": version})
}

func (s *Server) logCanvasErase(v domain.Viewer, guildID, what string) {
	s.log.Info("canvas erased from the dashboard", "guild", guildID, "by", v.DiscordID, "what", what)
}
