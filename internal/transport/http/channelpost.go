package http

import (
	"net/http"
	"time"

	"github.com/happyx-app/HappyX.Api/internal/domain"
)

// channelPostDTO is where the bot keeps one of its messages in a channel.
type channelPostDTO struct {
	Kind      string    `json:"kind"`
	ChannelID string    `json:"channel_id"`
	MessageID string    `json:"message_id"`
	CreatedAt time.Time `json:"created_at"`
}

func toChannelPostDTO(p domain.ChannelPost) channelPostDTO {
	return channelPostDTO{Kind: p.Kind, ChannelID: p.ChannelID, MessageID: p.MessageID, CreatedAt: p.CreatedAt}
}

func (s *Server) channelPost(w http.ResponseWriter, r *http.Request) {
	p, err := s.svc.ChannelPost(r.Context(), r.PathValue("guildID"), r.PathValue("kind"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toChannelPostDTO(p))
}

// putChannelPost records where the bot has just published the guild's post of that kind.
func (s *Server) putChannelPost(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ChannelID string `json:"channel_id"`
		MessageID string `json:"message_id"`
	}
	if !decode(w, r, &in) {
		return
	}
	p, err := s.svc.PutChannelPost(r.Context(), domain.ChannelPost{
		GuildDiscordID: r.PathValue("guildID"), Kind: r.PathValue("kind"), ChannelID: in.ChannelID, MessageID: in.MessageID,
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toChannelPostDTO(p))
}

func (s *Server) deleteChannelPost(w http.ResponseWriter, r *http.Request) {
	if err := s.svc.DeleteChannelPost(r.Context(), r.PathValue("guildID"), r.PathValue("kind")); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// memberMoods is the member's own answers in the server, for the bot to show only to them.
func (s *Server) memberMoods(w http.ResponseWriter, r *http.Request) {
	from, err1 := time.Parse(dateLayout, r.URL.Query().Get("from"))
	to, err2 := time.Parse(dateLayout, r.URL.Query().Get("to"))
	if err1 != nil || err2 != nil {
		writeError(w, http.StatusBadRequest, "from and to must both be YYYY-MM-DD")
		return
	}
	es, err := s.svc.MemberMoods(r.Context(), r.PathValue("guildID"), r.PathValue("userID"), from, to)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]moodDTO, 0, len(es))
	for _, e := range es {
		out = append(out, moodDTO{DiscordID: e.MemberDiscordID, Day: e.Day.Format(dateLayout), Mood: int(e.Mood), Message: e.Message})
	}
	writeJSON(w, http.StatusOK, out)
}

// userGuilds is the active servers the user is in, for /happyx in a DM with the bot.
func (s *Server) userGuilds(w http.ResponseWriter, r *http.Request) {
	gs, err := s.svc.UserGuilds(r.Context(), r.PathValue("userID"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	type userGuildDTO struct {
		guildDTO
		Paused bool `json:"paused"`
	}
	out := make([]userGuildDTO, 0, len(gs))
	for _, g := range gs {
		out = append(out, userGuildDTO{toGuildDTO(g.Guild), g.Paused})
	}
	writeJSON(w, http.StatusOK, out)
}
