package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/moixblau/HappyX.Api/internal/domain"
)

// Data exports for the GDPR: a JSON file the browser downloads. "format" and "version" let other
// tools recognise the file if its shape changes later.

const exportVersion = 1

type exportKudosDTO struct {
	Day          string `json:"day"`
	FromUsername string `json:"from_username,omitempty"`
	ToUsername   string `json:"to_username,omitempty"`
	Message      string `json:"message"`
}

func toExportKudos(ks []domain.Kudos, from, to bool) []exportKudosDTO {
	out := []exportKudosDTO{}
	for _, k := range ks {
		d := exportKudosDTO{Day: k.Day.Format(dateLayout), Message: k.Message}
		if from {
			d.FromUsername = k.GiverUsername
		}
		if to {
			d.ToUsername = k.ReceiverUsername
		}
		out = append(out, d)
	}
	return out
}

type exportServerDTO struct {
	DiscordID string `json:"discord_guild_id"`
	Name      string `json:"name"`
}

type myMemberDTO struct {
	Username   string    `json:"username"`
	TakingPart bool      `json:"taking_part"`
	JoinedAt   time.Time `json:"joined_at"`
}

type myExportMoodDTO struct {
	Day     string `json:"day"`
	Mood    int    `json:"mood"`
	Message string `json:"message,omitempty"`
	// Anonymous: the server was anonymous when this was answered, so it never shows who wrote it.
	Anonymous  bool      `json:"anonymous"`
	AnsweredAt time.Time `json:"answered_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type myGameAnswerDTO struct {
	Week     string `json:"week"`
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

type myGameGuessDTO struct {
	Week            string `json:"week"`
	Question        string `json:"question"`
	Answer          string `json:"answer"`
	GuessedUsername string `json:"guessed_username"`
}

type myGameProposalDTO struct {
	Week     string `json:"week"`
	Question string `json:"question"`
}

type myServerDataDTO struct {
	exportServerDTO
	Member        myMemberDTO         `json:"member"`
	Moods         []myExportMoodDTO   `json:"moods"`
	KudosGiven    []exportKudosDTO    `json:"kudos_given"`
	KudosReceived []exportKudosDTO    `json:"kudos_received"`
	GameAnswers   []myGameAnswerDTO   `json:"game_answers"`
	GameGuesses   []myGameGuessDTO    `json:"game_guesses"`
	GameProposals []myGameProposalDTO `json:"game_proposals"`
}

// webMyData downloads everything HappyX keeps about the viewer.
func (s *Server) webMyData(w http.ResponseWriter, r *http.Request, v domain.Viewer) {
	d, err := s.svc.MyData(r.Context(), v)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := struct {
		Format     string            `json:"format"`
		Version    int               `json:"version"`
		ExportedAt time.Time         `json:"exported_at"`
		User       viewerDTO         `json:"user"`
		ReportsFor []exportServerDTO `json:"receives_reports_for"`
		Servers    []myServerDataDTO `json:"servers"`
	}{"happyx-personal-data", exportVersion, s.web.Now().UTC().Truncate(time.Second), viewerDTO{v.DiscordID, v.Username, v.Avatar}, []exportServerDTO{}, []myServerDataDTO{}}
	for _, g := range d.ReportsFor {
		out.ReportsFor = append(out.ReportsFor, exportServerDTO{g.DiscordID, g.Name})
	}
	for _, md := range d.Guilds {
		sd := myServerDataDTO{
			exportServerDTO: exportServerDTO{md.Guild.DiscordID, md.Guild.Name},
			Member:          myMemberDTO{md.Member.Username, md.Member.Active, md.Member.CreatedAt},
			Moods:           []myExportMoodDTO{},
			KudosGiven:      toExportKudos(md.KudosGiven, false, true),
			KudosReceived:   toExportKudos(md.KudosReceived, true, false),
			GameAnswers:     []myGameAnswerDTO{},
			GameGuesses:     []myGameGuessDTO{},
			GameProposals:   []myGameProposalDTO{},
		}
		for _, e := range md.Moods {
			sd.Moods = append(sd.Moods, myExportMoodDTO{e.Day.Format(dateLayout), int(e.Mood), e.Message, e.Anonymous, e.CreatedAt, e.UpdatedAt})
		}
		for _, a := range md.GameAnswers {
			sd.GameAnswers = append(sd.GameAnswers, myGameAnswerDTO{a.Week.Format(dateLayout), a.Question, a.Text})
		}
		for _, g := range md.GameGuesses {
			sd.GameGuesses = append(sd.GameGuesses, myGameGuessDTO{g.Week.Format(dateLayout), g.Question, g.Answer, g.GuessedUsername})
		}
		for _, p := range md.GameProposals {
			sd.GameProposals = append(sd.GameProposals, myGameProposalDTO{p.Week.Format(dateLayout), p.Text})
		}
		out.Servers = append(out.Servers, sd)
	}
	s.log.Info("personal data exported", "user", v.DiscordID)
	writeDownload(w, "happyx-my-data-"+s.web.Now().UTC().Format(dateLayout)+".json", out)
}

// webEraseMyData deletes the viewer's data in every server. The body must hold their username
// ({"confirm_name": "..."}) so a stray request can't erase it.
func (s *Server) webEraseMyData(w http.ResponseWriter, r *http.Request, v domain.Viewer) {
	if !requireJSON(w, r) {
		return
	}
	var in struct {
		ConfirmName string `json:"confirm_name"`
	}
	if !decode(w, r, &in) {
		return
	}
	if err := s.svc.EraseMyData(r.Context(), v, in.ConfirmName); err != nil {
		s.fail(w, r, err)
		return
	}
	s.log.Info("personal data erased", "user", v.DiscordID)
	w.WriteHeader(http.StatusNoContent)
}

type serverExportDTO struct {
	exportServerDTO
	Anonymous       bool      `json:"anonymous"`
	Timezone        string    `json:"timezone"`
	Language        string    `json:"language"`
	SurveyHour      int       `json:"survey_hour"`
	WorkingDays     []int     `json:"working_days"`
	ReportFrequency string    `json:"report_frequency"`
	ReportDM        bool      `json:"report_dm"`
	GamesEnabled    bool      `json:"games_enabled"`
	SurveyMessage   string    `json:"survey_message,omitempty"`
	AdminDiscordID  string    `json:"admin_discord_user_id"`
	CreatedAt       time.Time `json:"created_at"`
}

type exportMemberDTO struct {
	DiscordID  string    `json:"discord_user_id"`
	Username   string    `json:"username"`
	TakingPart bool      `json:"taking_part"`
	JoinedAt   time.Time `json:"joined_at"`
}

type exportMoodDTO struct {
	Day      string `json:"day"`
	Mood     int    `json:"mood"`
	Message  string `json:"message,omitempty"`
	Username string `json:"username,omitempty"`
}

type exportAnswerDTO struct {
	Answer   string `json:"answer"`
	Username string `json:"username,omitempty"`
}

type exportRoundDTO struct {
	Week       string            `json:"week"`
	Question   string            `json:"question"`
	ProposedBy string            `json:"proposed_by,omitempty"`
	Answers    []exportAnswerDTO `json:"answers"`
}

// webGuildExport downloads a server's data for those who may see its dashboard. Survey answers
// keep the dashboard's anonymity.
func (s *Server) webGuildExport(w http.ResponseWriter, r *http.Request, v domain.Viewer) {
	x, err := s.svc.GuildExport(r.Context(), v, r.PathValue("guildID"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	g := x.Guild
	out := struct {
		Format     string            `json:"format"`
		Version    int               `json:"version"`
		ExportedAt time.Time         `json:"exported_at"`
		Server     serverExportDTO   `json:"server"`
		Members    []exportMemberDTO `json:"members"`
		Moods      []exportMoodDTO   `json:"moods"`
		Kudos      []exportKudosDTO  `json:"kudos"`
		Games      []exportRoundDTO  `json:"games"`
	}{
		Format: "happyx-server-data", Version: exportVersion, ExportedAt: s.web.Now().UTC().Truncate(time.Second),
		Server: serverExportDTO{
			exportServerDTO: exportServerDTO{g.DiscordID, g.Name},
			Anonymous:       g.Settings.Anonymous, Timezone: g.Settings.Timezone, Language: g.Settings.Language,
			SurveyHour: g.Settings.SurveyHour, WorkingDays: g.Settings.WorkingDays.ISO(),
			ReportFrequency: string(g.Settings.ReportFrequency), ReportDM: g.Settings.ReportDM, GamesEnabled: g.Settings.GamesEnabled,
			SurveyMessage: g.Settings.SurveyMessage, AdminDiscordID: g.AdminDiscordID, CreatedAt: g.CreatedAt,
		},
		Members: []exportMemberDTO{}, Moods: []exportMoodDTO{}, Kudos: toExportKudos(x.Kudos, true, true), Games: []exportRoundDTO{},
	}
	for _, m := range x.Members {
		out.Members = append(out.Members, exportMemberDTO{m.DiscordID, m.Username, m.Active, m.CreatedAt})
	}
	for _, e := range x.Moods {
		out.Moods = append(out.Moods, exportMoodDTO{e.Day.Format(dateLayout), int(e.Mood), e.Message, e.Username})
	}
	for _, rd := range x.Rounds {
		er := exportRoundDTO{Week: rd.Week.Format(dateLayout), Question: rd.Question, ProposedBy: rd.ProposedBy, Answers: []exportAnswerDTO{}}
		for _, a := range rd.Answers {
			er.Answers = append(er.Answers, exportAnswerDTO{a.Text, a.Username})
		}
		out.Games = append(out.Games, er)
	}
	s.log.Info("server data exported", "guild", g.DiscordID, "by", v.DiscordID)
	writeDownload(w, fmt.Sprintf("happyx-%s-%s.json", fileSlug(g.Name, g.DiscordID), s.web.Now().UTC().Format(dateLayout)), out)
}

// writeDownload sends v as an indented JSON file the browser saves as name.
func writeDownload(w http.ResponseWriter, name string, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

// fileSlug turns a server name into something safe for a file name, e.g. "Equipo Ñu!" into
// "equipo-u". Names with nothing usable fall back to the server's ID.
func fileSlug(name, fallback string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r)
			dash = false
		} else {
			dash = true
		}
		if b.Len() >= 40 {
			break
		}
	}
	if b.Len() == 0 {
		return fallback
	}
	return b.String()
}
