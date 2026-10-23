package http

import (
	"net/http"
	"strconv"
	"time"

	"github.com/moixblau/HappyX.Api/internal/domain"
	"github.com/moixblau/HappyX.Api/internal/usecase"
)

const dateLayout = "2006-01-02"

// --- DTOs ---

type guildDTO struct {
	DiscordID       string    `json:"discord_guild_id"`
	Name            string    `json:"name"`
	AdminDiscordID  string    `json:"admin_discord_user_id"`
	Anonymous       bool      `json:"anonymous"`
	Timezone        string    `json:"timezone"`
	SurveyHour      int       `json:"survey_hour"`
	ReportFrequency string    `json:"report_frequency"`
	Active          bool      `json:"active"`
	CreatedAt       time.Time `json:"created_at"`
}

func toGuildDTO(g domain.Guild) guildDTO {
	return guildDTO{
		DiscordID: g.DiscordID, Name: g.Name, AdminDiscordID: g.AdminDiscordID,
		Anonymous: g.Settings.Anonymous, Timezone: g.Settings.Timezone,
		SurveyHour: g.Settings.SurveyHour, ReportFrequency: string(g.Settings.ReportFrequency),
		Active: g.Active, CreatedAt: g.CreatedAt,
	}
}

type memberDTO struct {
	DiscordID string    `json:"discord_user_id"`
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"created_at"`
}

type moodDTO struct {
	DiscordID string `json:"discord_user_id"`
	Day       string `json:"day"`
	Mood      int    `json:"mood"`
	Message   string `json:"message,omitempty"`
}

type reportDTO struct {
	DiscordGuildID string         `json:"discord_guild_id"`
	From           string         `json:"from"`
	To             string         `json:"to"`
	Anonymous      bool           `json:"anonymous"`
	Responses      int            `json:"responses"`
	Respondents    int            `json:"respondents"`
	Average        float64        `json:"average"`
	Distribution   map[string]int `json:"distribution"`
	Days           []dayDTO       `json:"days"`
	Messages       []reportMsgDTO `json:"messages"`
}

type dayDTO struct {
	Day       string  `json:"day"`
	Responses int     `json:"responses"`
	Average   float64 `json:"average"`
}

type reportMsgDTO struct {
	Day       string `json:"day"`
	Mood      int    `json:"mood"`
	Message   string `json:"message"`
	DiscordID string `json:"discord_user_id,omitempty"`
	Username  string `json:"username,omitempty"`
}

// --- Guilds ---

func (s *Server) listGuilds(w http.ResponseWriter, r *http.Request) {
	gs, err := s.svc.ListActiveGuilds(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]guildDTO, 0, len(gs))
	for _, g := range gs {
		out = append(out, toGuildDTO(g))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) registerGuild(w http.ResponseWriter, r *http.Request) {
	var in struct {
		DiscordID      string `json:"discord_guild_id"`
		Name           string `json:"name"`
		AdminDiscordID string `json:"admin_discord_user_id"`
	}
	if !decode(w, r, &in) {
		return
	}
	g, err := s.svc.RegisterGuild(r.Context(), usecase.RegisterGuildInput{
		DiscordID: in.DiscordID, Name: in.Name, AdminDiscordID: in.AdminDiscordID,
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toGuildDTO(g))
}

func (s *Server) getGuild(w http.ResponseWriter, r *http.Request) {
	g, err := s.svc.GetGuild(r.Context(), r.PathValue("guildID"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toGuildDTO(g))
}

func (s *Server) updateGuild(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Anonymous       *bool   `json:"anonymous"`
		Timezone        *string `json:"timezone"`
		SurveyHour      *int    `json:"survey_hour"`
		ReportFrequency *string `json:"report_frequency"`
		AdminDiscordID  *string `json:"admin_discord_user_id"`
	}
	if !decode(w, r, &in) {
		return
	}
	p := usecase.SettingsPatch{
		Anonymous: in.Anonymous, Timezone: in.Timezone, SurveyHour: in.SurveyHour,
		AdminDiscordID: in.AdminDiscordID,
	}
	if in.ReportFrequency != nil {
		f := domain.ReportFrequency(*in.ReportFrequency)
		p.ReportFrequency = &f
	}
	g, err := s.svc.UpdateGuildSettings(r.Context(), r.PathValue("guildID"), p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toGuildDTO(g))
}

func (s *Server) removeGuild(w http.ResponseWriter, r *http.Request) {
	if err := s.svc.RemoveGuild(r.Context(), r.PathValue("guildID")); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Members ---

func (s *Server) listMembers(w http.ResponseWriter, r *http.Request) {
	ms, err := s.svc.ListMembers(r.Context(), r.PathValue("guildID"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]memberDTO, 0, len(ms))
	for _, m := range ms {
		out = append(out, memberDTO{DiscordID: m.DiscordID, Username: m.Username, CreatedAt: m.CreatedAt})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) joinMember(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username string `json:"username"`
	}
	if !decode(w, r, &in) {
		return
	}
	m, err := s.svc.JoinMember(r.Context(), r.PathValue("guildID"), r.PathValue("userID"), in.Username)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, memberDTO{DiscordID: m.DiscordID, Username: m.Username, CreatedAt: m.CreatedAt})
}

func (s *Server) leaveMember(w http.ResponseWriter, r *http.Request) {
	if err := s.svc.LeaveMember(r.Context(), r.PathValue("guildID"), r.PathValue("userID")); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Moods ---

func (s *Server) recordMood(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Mood    int    `json:"mood"`
		Message string `json:"message"`
		Day     string `json:"day"` // optional, YYYY-MM-DD; defaults to today in the guild's timezone
	}
	if !decode(w, r, &in) {
		return
	}
	var day time.Time
	if in.Day != "" {
		var err error
		if day, err = time.Parse(dateLayout, in.Day); err != nil {
			writeError(w, http.StatusBadRequest, "day must be YYYY-MM-DD")
			return
		}
	}
	e, err := s.svc.RecordMood(r.Context(), usecase.RecordMoodInput{
		GuildID: r.PathValue("guildID"), UserID: r.PathValue("userID"),
		Mood: domain.Mood(in.Mood), Message: in.Message, Day: day,
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, moodDTO{
		DiscordID: e.MemberDiscordID, Day: e.Day.Format(dateLayout), Mood: int(e.Mood), Message: e.Message,
	})
}

// --- Reports ---

// report returns aggregated data for ?from=YYYY-MM-DD&to=YYYY-MM-DD. Without a range it
// returns the last complete period (week or month) according to the guild's settings.
func (s *Server) report(w http.ResponseWriter, r *http.Request) {
	guildID := r.PathValue("guildID")
	q := r.URL.Query()
	var from, to time.Time
	if q.Get("from") == "" && q.Get("to") == "" {
		g, err := s.svc.GetGuild(r.Context(), guildID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		from, to = s.svc.LastPeriod(g)
	} else {
		var err1, err2 error
		from, err1 = time.Parse(dateLayout, q.Get("from"))
		to, err2 = time.Parse(dateLayout, q.Get("to"))
		if err1 != nil || err2 != nil {
			writeError(w, http.StatusBadRequest, "from and to must both be YYYY-MM-DD")
			return
		}
	}
	rep, err := s.svc.Report(r.Context(), guildID, from, to)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := reportDTO{
		DiscordGuildID: rep.GuildDiscordID, From: rep.From.Format(dateLayout), To: rep.To.Format(dateLayout),
		Anonymous: rep.Anonymous, Responses: rep.Responses, Respondents: rep.Respondents,
		Average: rep.Average, Distribution: map[string]int{},
		Days: []dayDTO{}, Messages: []reportMsgDTO{},
	}
	for m, n := range rep.Distribution {
		out.Distribution[strconv.Itoa(int(m))] = n
	}
	for _, d := range rep.Days {
		out.Days = append(out.Days, dayDTO{Day: d.Day.Format(dateLayout), Responses: d.Responses, Average: d.Average})
	}
	for _, m := range rep.Messages {
		out.Messages = append(out.Messages, reportMsgDTO{
			Day: m.Day.Format(dateLayout), Mood: int(m.Mood), Message: m.Message,
			DiscordID: m.MemberDiscordID, Username: m.Username,
		})
	}
	writeJSON(w, http.StatusOK, out)
}
