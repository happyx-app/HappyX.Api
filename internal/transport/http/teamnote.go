package http

import (
	"net/http"
	"strconv"
	"time"

	"github.com/happyx-app/HappyX.Api/internal/domain"
)

// teamNoteDTO is what an admin told the team after a report, and where the bot published it.
type teamNoteDTO struct {
	ID              int64  `json:"id"`
	AuthorDiscordID string `json:"author_discord_user_id"`
	AuthorUsername  string `json:"author_username"`
	// AuthorDisplayName is the name the server shows for the author.
	AuthorDisplayName string    `json:"author_display_name"`
	From              string    `json:"from"`
	To                string    `json:"to"`
	Body              string    `json:"body"`
	ChannelID         string    `json:"channel_id"`
	MessageID         string    `json:"message_id"`
	PostedOn          string    `json:"posted_on"`
	UpdatedAt         time.Time `json:"updated_at"`
}

func toTeamNoteDTO(n domain.TeamNote) teamNoteDTO {
	return teamNoteDTO{
		ID: n.ID, AuthorDiscordID: n.AuthorDiscordID, AuthorUsername: n.AuthorUsername, AuthorDisplayName: n.AuthorDisplayName,
		From: n.From.Format(dateLayout), To: n.To.Format(dateLayout), Body: n.Body,
		ChannelID: n.ChannelID, MessageID: n.MessageID, PostedOn: n.PostedOn.Format(dateLayout), UpdatedAt: n.UpdatedAt,
	}
}

// publishTeamNote records a note the bot has just published in a channel.
func (s *Server) publishTeamNote(w http.ResponseWriter, r *http.Request) {
	var in struct {
		AuthorDiscordID string `json:"author_discord_user_id"`
		From            string `json:"from"`
		To              string `json:"to"`
		Body            string `json:"body"`
		ChannelID       string `json:"channel_id"`
		MessageID       string `json:"message_id"`
	}
	if !decode(w, r, &in) {
		return
	}
	from, err1 := time.Parse(dateLayout, in.From)
	to, err2 := time.Parse(dateLayout, in.To)
	if err1 != nil || err2 != nil {
		writeError(w, http.StatusBadRequest, "from and to must both be YYYY-MM-DD")
		return
	}
	n, err := s.svc.PublishTeamNote(r.Context(), domain.TeamNote{
		GuildDiscordID: r.PathValue("guildID"), AuthorDiscordID: in.AuthorDiscordID,
		From: from, To: to, Body: in.Body, ChannelID: in.ChannelID, MessageID: in.MessageID,
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toTeamNoteDTO(n))
}

// teamNoteID reads the {noteID} path value; it writes a 404 when it isn't a number.
func teamNoteID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("noteID"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusNotFound, "not found")
		return 0, false
	}
	return id, true
}

func (s *Server) teamNote(w http.ResponseWriter, r *http.Request) {
	id, ok := teamNoteID(w, r)
	if !ok {
		return
	}
	n, err := s.svc.TeamNote(r.Context(), r.PathValue("guildID"), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toTeamNoteDTO(n))
}

func (s *Server) editTeamNote(w http.ResponseWriter, r *http.Request) {
	id, ok := teamNoteID(w, r)
	if !ok {
		return
	}
	var in struct {
		Body string `json:"body"`
	}
	if !decode(w, r, &in) {
		return
	}
	n, err := s.svc.EditTeamNote(r.Context(), r.PathValue("guildID"), id, in.Body)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toTeamNoteDTO(n))
}

func (s *Server) deleteTeamNote(w http.ResponseWriter, r *http.Request) {
	id, ok := teamNoteID(w, r)
	if !ok {
		return
	}
	if err := s.svc.DeleteTeamNote(r.Context(), r.PathValue("guildID"), id); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// currentTeamNote is the note the thanks card points to for answers on ?day=YYYY-MM-DD: 404 when
// there is none.
func (s *Server) currentTeamNote(w http.ResponseWriter, r *http.Request) {
	day, err := time.Parse(dateLayout, r.URL.Query().Get("day"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "day must be YYYY-MM-DD")
		return
	}
	n, err := s.svc.CurrentTeamNote(r.Context(), r.PathValue("guildID"), day)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toTeamNoteDTO(n))
}

// periodTeamNote is the note about the report period ?from=&to= (YYYY-MM-DD): 404 when there is
// none.
func (s *Server) periodTeamNote(w http.ResponseWriter, r *http.Request) {
	from, err1 := time.Parse(dateLayout, r.URL.Query().Get("from"))
	to, err2 := time.Parse(dateLayout, r.URL.Query().Get("to"))
	if err1 != nil || err2 != nil {
		writeError(w, http.StatusBadRequest, "from and to must both be YYYY-MM-DD")
		return
	}
	n, err := s.svc.PeriodTeamNote(r.Context(), r.PathValue("guildID"), from, to)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toTeamNoteDTO(n))
}
