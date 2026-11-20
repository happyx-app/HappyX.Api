package http

import (
	"net/http"
	"strconv"
	"time"

	"github.com/happyx-app/HappyX.Api/internal/domain"
)

// teamEventDTO is an event on the chart, as the dashboard draws it.
type teamEventDTO struct {
	ID          string `json:"id"`
	From        string `json:"from"`
	To          string `json:"to"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Color       string `json:"color"`
}

func toTeamEventDTO(e domain.TeamEvent) teamEventDTO {
	return teamEventDTO{strconv.FormatInt(e.ID, 10), e.From.Format(dateLayout), e.To.Format(dateLayout), e.Title, e.Description, e.Color}
}

func toTeamEventDTOs(es []domain.TeamEvent) []teamEventDTO {
	out := make([]teamEventDTO, 0, len(es))
	for _, e := range es {
		out = append(out, toTeamEventDTO(e))
	}
	return out
}

// webTeamEvents lists the server's events that touch ?from=&to= (YYYY-MM-DD), for whoever can
// see its dashboard.
func (s *Server) webTeamEvents(w http.ResponseWriter, r *http.Request, v domain.Viewer) {
	from, to, ok := s.dayRange(w, r)
	if !ok {
		return
	}
	es, err := s.svc.TeamEvents(r.Context(), v, r.PathValue("guildID"), from, to)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Events []teamEventDTO `json:"events"`
	}{toTeamEventDTOs(es)})
}

// decodeTeamEvent reads {"from", "to", "title", "description", "color"}; color may be left out
// for the default.
func decodeTeamEvent(w http.ResponseWriter, r *http.Request) (domain.TeamEvent, bool) {
	var in struct {
		From        string `json:"from"`
		To          string `json:"to"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Color       string `json:"color"`
	}
	if !decode(w, r, &in) {
		return domain.TeamEvent{}, false
	}
	from, err1 := time.Parse(dateLayout, in.From)
	to, err2 := time.Parse(dateLayout, in.To)
	if err1 != nil || err2 != nil {
		writeError(w, http.StatusBadRequest, "from and to must be dates as YYYY-MM-DD")
		return domain.TeamEvent{}, false
	}
	return domain.TeamEvent{From: from, To: to, Title: in.Title, Description: in.Description, Color: in.Color}, true
}

func eventID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("eventID"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "event id must be a positive number")
		return 0, false
	}
	return id, true
}

// webCreateTeamEvent marks an event on the server's chart. It needs the "manage" level.
func (s *Server) webCreateTeamEvent(w http.ResponseWriter, r *http.Request, v domain.Viewer) {
	if !requireJSON(w, r) {
		return
	}
	e, ok := decodeTeamEvent(w, r)
	if !ok {
		return
	}
	saved, err := s.svc.CreateTeamEvent(r.Context(), v, r.PathValue("guildID"), e)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toTeamEventDTO(saved))
}

// webUpdateTeamEvent changes one of the server's events. It needs the "manage" level.
func (s *Server) webUpdateTeamEvent(w http.ResponseWriter, r *http.Request, v domain.Viewer) {
	if !requireJSON(w, r) {
		return
	}
	id, ok := eventID(w, r)
	if !ok {
		return
	}
	e, ok := decodeTeamEvent(w, r)
	if !ok {
		return
	}
	e.ID = id
	saved, err := s.svc.UpdateTeamEvent(r.Context(), v, r.PathValue("guildID"), e)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toTeamEventDTO(saved))
}

// webDeleteTeamEvent removes one of the server's events. It needs the "manage" level.
func (s *Server) webDeleteTeamEvent(w http.ResponseWriter, r *http.Request, v domain.Viewer) {
	if !requireJSON(w, r) {
		return
	}
	id, ok := eventID(w, r)
	if !ok {
		return
	}
	if err := s.svc.DeleteTeamEvent(r.Context(), v, r.PathValue("guildID"), id); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
