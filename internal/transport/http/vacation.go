package http

import (
	"net/http"
	"strconv"
	"time"

	"github.com/happyx-app/HappyX.Api/internal/domain"
)

// vacationDTO is one vacation. discord_guild_id is empty for the person's own, which applies in
// every server; set_by_admin says someone who manages the server set it for them.
type vacationDTO struct {
	From       string `json:"from"`
	To         string `json:"to"`
	GuildID    string `json:"discord_guild_id,omitempty"`
	GuildName  string `json:"guild_name,omitempty"`
	SetByAdmin bool   `json:"set_by_admin"`
}

func toVacationDTO(v domain.Vacation) vacationDTO {
	return vacationDTO{v.From.Format(dateLayout), v.To.Format(dateLayout), v.GuildDiscordID, v.GuildName, v.SetByAdmin()}
}

func toVacationDTOs(vs []domain.Vacation) []vacationDTO {
	out := make([]vacationDTO, 0, len(vs))
	for _, v := range vs {
		out = append(out, toVacationDTO(v))
	}
	return out
}

// myVacationDTO is a vacation in the personal data export.
type myVacationDTO struct {
	vacationDTO
	CreatedAt time.Time `json:"created_at"`
}

func toMyVacationDTO(v domain.Vacation) myVacationDTO {
	return myVacationDTO{toVacationDTO(v), v.CreatedAt}
}

// decodeVacationDates reads {"from": "YYYY-MM-DD", "to": "YYYY-MM-DD"}.
func decodeVacationDates(w http.ResponseWriter, r *http.Request) (from, to time.Time, ok bool) {
	var in struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if !decode(w, r, &in) {
		return from, to, false
	}
	from, err1 := time.Parse(dateLayout, in.From)
	to, err2 := time.Parse(dateLayout, in.To)
	if err1 != nil || err2 != nil {
		writeError(w, http.StatusBadRequest, "from and to must be dates as YYYY-MM-DD")
		return from, to, false
	}
	return from, to, true
}

// --- Bot ---

// memberVacations lists the vacations that apply to the member in the server and haven't ended.
func (s *Server) memberVacations(w http.ResponseWriter, r *http.Request) {
	vs, err := s.svc.MemberVacations(r.Context(), r.PathValue("guildID"), r.PathValue("userID"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toVacationDTOs(vs))
}

// setMemberVacation is /happyx vacation: the member's own vacation, for every server.
func (s *Server) setMemberVacation(w http.ResponseWriter, r *http.Request) {
	from, to, ok := decodeVacationDates(w, r)
	if !ok {
		return
	}
	v, err := s.svc.SetMemberOwnVacation(r.Context(), r.PathValue("guildID"), r.PathValue("userID"), from, to)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toVacationDTO(v))
}

// endMemberVacations removes the member's own vacation and the one set for them in the server.
func (s *Server) endMemberVacations(w http.ResponseWriter, r *http.Request) {
	if err := s.svc.EndMemberVacations(r.Context(), r.PathValue("guildID"), r.PathValue("userID")); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// vacationNotices lists the vacations admins set that the bot has to tell the person about by DM.
func (s *Server) vacationNotices(w http.ResponseWriter, r *http.Request) {
	vs, err := s.svc.VacationNotices(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	type noticeDTO struct {
		ID      int64  `json:"id"`
		GuildID string `json:"discord_guild_id"`
		UserID  string `json:"discord_user_id"`
		SetByID string `json:"set_by_discord_user_id"`
		From    string `json:"from"`
		To      string `json:"to"`
	}
	out := make([]noticeDTO, 0, len(vs))
	for _, v := range vs {
		out = append(out, noticeDTO{v.ID, v.GuildDiscordID, v.UserDiscordID, v.SetByDiscordID, v.From.Format(dateLayout), v.To.Format(dateLayout)})
	}
	writeJSON(w, http.StatusOK, out)
}

// doneVacationNotice records the bot told the person, or gave up because their DMs are closed.
func (s *Server) doneVacationNotice(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "id must be a number")
		return
	}
	if err := s.svc.DoneVacationNotice(r.Context(), id); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Web: the person's own ---

// webMyVacations returns the viewer's own vacation (null when none) and the ones admins set for
// them in their servers, all that haven't ended.
func (s *Server) webMyVacations(w http.ResponseWriter, r *http.Request, v domain.Viewer) {
	vs, err := s.svc.MyVacations(r.Context(), v)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := struct {
		Own         *vacationDTO  `json:"own"`
		SetByAdmins []vacationDTO `json:"set_by_admins"`
	}{SetByAdmins: []vacationDTO{}}
	for _, vac := range vs {
		d := toVacationDTO(vac)
		if vac.GuildDiscordID == "" {
			out.Own = &d
		} else {
			out.SetByAdmins = append(out.SetByAdmins, d)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) webSetMyVacation(w http.ResponseWriter, r *http.Request, v domain.Viewer) {
	if !requireJSON(w, r) {
		return
	}
	from, to, ok := decodeVacationDates(w, r)
	if !ok {
		return
	}
	vac, err := s.svc.SetMyVacation(r.Context(), v, from, to)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toVacationDTO(vac))
}

// webDeleteMyVacation removes the viewer's own vacation; under /web/guilds/{guildID}/ it removes
// the one an admin set for them in that server.
func (s *Server) webDeleteMyVacation(w http.ResponseWriter, r *http.Request, v domain.Viewer) {
	if !requireJSON(w, r) {
		return
	}
	if err := s.svc.DeleteMyVacation(r.Context(), v, r.PathValue("guildID")); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Web: admins ---

// guildVacationDTO is a member's vacation as the server's admins see it. set_here is false for
// the member's own vacation, which admins can see but not change.
type guildVacationDTO struct {
	UserID   string `json:"discord_user_id"`
	Username string `json:"username"`
	From     string `json:"from"`
	To       string `json:"to"`
	SetHere  bool   `json:"set_here"`
}

func toGuildVacationDTO(v domain.Vacation) guildVacationDTO {
	return guildVacationDTO{v.UserDiscordID, v.Username, v.From.Format(dateLayout), v.To.Format(dateLayout), v.GuildDiscordID != ""}
}

// webGuildVacations lists the vacations of the server's members that haven't ended, and the
// members who can be given one. It needs the "manage" level.
func (s *Server) webGuildVacations(w http.ResponseWriter, r *http.Request, v domain.Viewer) {
	gv, err := s.svc.ListGuildVacations(r.Context(), v, r.PathValue("guildID"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	type memberDTO struct {
		ID       string `json:"discord_user_id"`
		Username string `json:"username"`
	}
	out := struct {
		Vacations []guildVacationDTO `json:"vacations"`
		Members   []memberDTO        `json:"members"`
	}{[]guildVacationDTO{}, []memberDTO{}}
	for _, vac := range gv.Vacations {
		out.Vacations = append(out.Vacations, toGuildVacationDTO(vac))
	}
	for _, m := range gv.Members {
		out.Members = append(out.Members, memberDTO{m.DiscordID, m.Username})
	}
	writeJSON(w, http.StatusOK, out)
}

// webSetGuildVacation sets a member's vacation in this server; the bot tells them by DM.
func (s *Server) webSetGuildVacation(w http.ResponseWriter, r *http.Request, v domain.Viewer) {
	if !requireJSON(w, r) {
		return
	}
	from, to, ok := decodeVacationDates(w, r)
	if !ok {
		return
	}
	vac, err := s.svc.SetGuildVacation(r.Context(), v, r.PathValue("guildID"), r.PathValue("userID"), from, to)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.log.Info("vacation set from the dashboard", "guild", vac.GuildDiscordID, "by", v.DiscordID, "user", vac.UserDiscordID)
	writeJSON(w, http.StatusOK, toGuildVacationDTO(vac))
}

// webDeleteGuildVacation removes the vacation set for a member in this server.
func (s *Server) webDeleteGuildVacation(w http.ResponseWriter, r *http.Request, v domain.Viewer) {
	if !requireJSON(w, r) {
		return
	}
	if err := s.svc.DeleteGuildVacation(r.Context(), v, r.PathValue("guildID"), r.PathValue("userID")); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
