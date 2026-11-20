package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/happyx-app/HappyX.Api/internal/domain"
)

// Data exports for the GDPR: a JSON file the browser downloads. "format" and "version" let other
// tools recognise the file if its shape changes later.

const exportVersion = 1

// serverExportVersion went to 2 when anonymous answers stopped coming one by one: they come
// summed up by day, with their comments apart (anonymous_days, anonymous_comments).
const serverExportVersion = 2

type exportKudosDTO struct {
	Day             string `json:"day"`
	FromUsername    string `json:"from_username,omitempty"`
	FromDisplayName string `json:"from_display_name,omitempty"`
	ToUsername      string `json:"to_username,omitempty"`
	ToDisplayName   string `json:"to_display_name,omitempty"`
	Message         string `json:"message"`
}

func toExportKudos(ks []domain.Kudos, from, to bool) []exportKudosDTO {
	out := []exportKudosDTO{}
	for _, k := range ks {
		d := exportKudosDTO{Day: k.Day.Format(dateLayout), Message: k.Message}
		if from {
			d.FromUsername, d.FromDisplayName = k.GiverUsername, k.GiverDisplayName
		}
		if to {
			d.ToUsername, d.ToDisplayName = k.ReceiverUsername, k.ReceiverDisplayName
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
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
	TakingPart  bool      `json:"taking_part"`
	JoinedAt    time.Time `json:"joined_at"`
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
	Week               string `json:"week"`
	Question           string `json:"question"`
	Answer             string `json:"answer"`
	GuessedUsername    string `json:"guessed_username"`
	GuessedDisplayName string `json:"guessed_display_name"`
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
	CanvasStrokes []canvasStrokeDTO   `json:"canvas_strokes"`
	TeamNotes     []exportTeamNoteDTO `json:"team_notes"`
}

// exportTeamNoteDTO is what an admin told the team after a report.
type exportTeamNoteDTO struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Body   string `json:"body"`
	Author string `json:"author,omitempty"`
	// AuthorDisplayName is the name the server shows for the author.
	AuthorDisplayName string    `json:"author_display_name,omitempty"`
	ChannelID         string    `json:"channel_id"`
	PostedOn          string    `json:"posted_on"`
	UpdatedAt         time.Time `json:"updated_at"`
}

func toExportTeamNotes(ns []domain.TeamNote, withAuthor bool) []exportTeamNoteDTO {
	out := []exportTeamNoteDTO{}
	for _, n := range ns {
		d := exportTeamNoteDTO{From: n.From.Format(dateLayout), To: n.To.Format(dateLayout), Body: n.Body, ChannelID: n.ChannelID, PostedOn: n.PostedOn.Format(dateLayout), UpdatedAt: n.UpdatedAt}
		if withAuthor {
			d.Author, d.AuthorDisplayName = n.AuthorUsername, n.AuthorDisplayName
		}
		out = append(out, d)
	}
	return out
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
		Vacations  []myVacationDTO   `json:"vacations"`
	}{"happyx-personal-data", exportVersion, s.web.Now().UTC().Truncate(time.Second), viewerDTO{v.DiscordID, v.Username, v.Avatar}, []exportServerDTO{}, []myServerDataDTO{}, []myVacationDTO{}}
	for _, vac := range d.Vacations {
		out.Vacations = append(out.Vacations, toMyVacationDTO(vac))
	}
	for _, g := range d.ReportsFor {
		out.ReportsFor = append(out.ReportsFor, exportServerDTO{g.DiscordID, g.Name})
	}
	for _, md := range d.Guilds {
		sd := myServerDataDTO{
			exportServerDTO: exportServerDTO{md.Guild.DiscordID, md.Guild.Name},
			Member:          myMemberDTO{md.Member.Username, md.Member.DisplayName, md.Member.Active, md.Member.CreatedAt},
			Moods:           []myExportMoodDTO{},
			KudosGiven:      toExportKudos(md.KudosGiven, false, true),
			KudosReceived:   toExportKudos(md.KudosReceived, true, false),
			GameAnswers:     []myGameAnswerDTO{},
			GameGuesses:     []myGameGuessDTO{},
			GameProposals:   []myGameProposalDTO{},
			CanvasStrokes:   []canvasStrokeDTO{},
			TeamNotes:       toExportTeamNotes(md.TeamNotes, false),
		}
		for _, e := range md.Moods {
			sd.Moods = append(sd.Moods, myExportMoodDTO{e.Day.Format(dateLayout), int(e.Mood), e.Message, e.Anonymous, e.CreatedAt, e.UpdatedAt})
		}
		for _, a := range md.GameAnswers {
			sd.GameAnswers = append(sd.GameAnswers, myGameAnswerDTO{a.Week.Format(dateLayout), a.Question, a.Text})
		}
		for _, g := range md.GameGuesses {
			sd.GameGuesses = append(sd.GameGuesses, myGameGuessDTO{g.Week.Format(dateLayout), g.Question, g.Answer, g.GuessedUsername, g.GuessedDisplayName})
		}
		for _, p := range md.GameProposals {
			sd.GameProposals = append(sd.GameProposals, myGameProposalDTO{p.Week.Format(dateLayout), p.Text})
		}
		for _, st := range md.CanvasStrokes {
			sd.CanvasStrokes = append(sd.CanvasStrokes, toCanvasStrokeDTO(st, v.DiscordID, false))
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
	Anonymous       bool          `json:"anonymous"`
	Timezone        string        `json:"timezone"`
	Language        string        `json:"language"`
	SurveyHour      int           `json:"survey_hour"`
	WorkingDays     []int         `json:"working_days"`
	ReportFrequency string        `json:"report_frequency"`
	ReportDM        bool          `json:"report_dm"`
	GamesEnabled    bool          `json:"games_enabled"`
	SurveyMessage   string        `json:"survey_message,omitempty"`
	SurveyRoles     roleFilterDTO `json:"survey_roles"`
	GameRoles       roleFilterDTO `json:"game_roles"`
	CanvasEnabled   bool          `json:"canvas_enabled"`
	SurveyEnabled   bool          `json:"survey_enabled"`
	AdminDiscordID  string        `json:"admin_discord_user_id"`
	CreatedAt       time.Time     `json:"created_at"`
}

type exportMemberDTO struct {
	DiscordID   string    `json:"discord_user_id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
	TakingPart  bool      `json:"taking_part"`
	JoinedAt    time.Time `json:"joined_at"`
}

type exportMoodDTO struct {
	Day         string `json:"day"`
	Mood        int    `json:"mood"`
	Message     string `json:"message,omitempty"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
}

type exportDayDTO struct {
	Day      string   `json:"day"`
	Answers  int      `json:"answers"`
	Average  *float64 `json:"average,omitempty"` // left out when withheld
	Withheld bool     `json:"withheld"`
}

type exportAnswerDTO struct {
	Answer      string `json:"answer"`
	Username    string `json:"username,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
}

type exportRoundDTO struct {
	Week       string `json:"week"`
	Question   string `json:"question"`
	ProposedBy string `json:"proposed_by,omitempty"`
	// ProposedByDisplayName is the name the server shows for whoever proposed the question.
	ProposedByDisplayName string            `json:"proposed_by_display_name,omitempty"`
	Answers               []exportAnswerDTO `json:"answers"`
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
		// Moods are the answers that show their author; the anonymous ones are summed up by day.
		Moods             []exportMoodDTO     `json:"moods"`
		AnonymousDays     []exportDayDTO      `json:"anonymous_days"`
		AnonymousComments []string            `json:"anonymous_comments"`
		Kudos             []exportKudosDTO    `json:"kudos"`
		Games             []exportRoundDTO    `json:"games"`
		Canvas            []canvasStrokeDTO   `json:"canvas_strokes"`
		TeamNotes         []exportTeamNoteDTO `json:"team_notes"`
		TeamEvents        []teamEventDTO      `json:"team_events"`
	}{
		Format: "happyx-server-data", Version: serverExportVersion, ExportedAt: s.web.Now().UTC().Truncate(time.Second),
		Server: serverExportDTO{
			exportServerDTO: exportServerDTO{g.DiscordID, g.Name},
			Anonymous:       g.Settings.Anonymous, Timezone: g.Settings.Timezone, Language: g.Settings.Language,
			SurveyHour: g.Settings.SurveyHour, WorkingDays: g.Settings.WorkingDays.ISO(),
			ReportFrequency: string(g.Settings.ReportFrequency), ReportDM: g.Settings.ReportDM, GamesEnabled: g.Settings.GamesEnabled,
			SurveyMessage: g.Settings.SurveyMessage, SurveyRoles: toRoleFilterDTO(g.Settings.SurveyRoles),
			GameRoles: toRoleFilterDTO(g.Settings.GameRoles), CanvasEnabled: g.Settings.CanvasEnabled, SurveyEnabled: g.Settings.SurveyEnabled, AdminDiscordID: g.AdminDiscordID, CreatedAt: g.CreatedAt,
		},
		Members: []exportMemberDTO{}, Moods: []exportMoodDTO{}, AnonymousDays: []exportDayDTO{}, AnonymousComments: x.AnonymousComments, Kudos: toExportKudos(x.Kudos, true, true), Games: []exportRoundDTO{},
		Canvas: []canvasStrokeDTO{}, TeamNotes: toExportTeamNotes(x.TeamNotes, true), TeamEvents: toTeamEventDTOs(x.TeamEvents),
	}
	for _, st := range x.CanvasStrokes {
		out.Canvas = append(out.Canvas, toCanvasStrokeDTO(st, v.DiscordID, true))
	}
	for _, m := range x.Members {
		out.Members = append(out.Members, exportMemberDTO{m.DiscordID, m.Username, m.DisplayName, m.Active, m.CreatedAt})
	}
	for _, e := range x.Moods {
		out.Moods = append(out.Moods, exportMoodDTO{e.Day.Format(dateLayout), int(e.Mood), e.Message, e.Username, e.DisplayName})
	}
	for _, d := range x.AnonymousDays {
		dd := exportDayDTO{Day: d.Day.Format(dateLayout), Answers: d.Answers, Withheld: d.Withheld}
		if !d.Withheld {
			dd.Average = &d.Average
		}
		out.AnonymousDays = append(out.AnonymousDays, dd)
	}
	for _, rd := range x.Rounds {
		er := exportRoundDTO{Week: rd.Week.Format(dateLayout), Question: rd.Question, ProposedBy: rd.ProposedBy, ProposedByDisplayName: rd.ProposedByDisplayName, Answers: []exportAnswerDTO{}}
		for _, a := range rd.Answers {
			er.Answers = append(er.Answers, exportAnswerDTO{a.Text, a.Username, a.DisplayName})
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
