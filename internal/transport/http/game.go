package http

import (
	"net/http"
	"time"

	"github.com/moixblau/HappyX.Api/internal/domain"
	"github.com/moixblau/HappyX.Api/internal/usecase"
)

// --- «Who's who?» game, for the bot ---

type gameRoundDTO struct {
	Week           string `json:"week"`
	Question       string `json:"question"`
	Phase          string `json:"phase"`
	GuessingStarts string `json:"guessing_starts"`
	LastDay        string `json:"last_day"`
	Answers        int    `json:"answers"`
	MinAnswers     int    `json:"min_answers"`
}

func toGameRoundDTO(st usecase.GameRoundStatus) gameRoundDTO {
	r := st.Round
	return gameRoundDTO{
		Week: r.Week.Format(dateLayout), Question: r.Question, Phase: string(st.Phase),
		GuessingStarts: r.GuessingStarts().Format(dateLayout), LastDay: r.LastDay().Format(dateLayout),
		Answers: st.Answers, MinAnswers: domain.MinGameAnswers,
	}
}

func parseWeek(w http.ResponseWriter, v string) (time.Time, bool) {
	week, err := time.Parse(dateLayout, v)
	if err != nil {
		writeError(w, http.StatusBadRequest, "week must be YYYY-MM-DD")
		return time.Time{}, false
	}
	return week, true
}

// startGameRound opens this week's round with the bot's question. It answers 200 with the round
// (new or existing), and 409 when games are off or the week's answering days are over.
func (s *Server) startGameRound(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Week     string `json:"week"`
		Question string `json:"question"`
	}
	if !decode(w, r, &in) {
		return
	}
	week, ok := parseWeek(w, in.Week)
	if !ok {
		return
	}
	guildID := r.PathValue("guildID")
	if _, err := s.svc.StartGameRound(r.Context(), guildID, week, in.Question); err != nil {
		s.fail(w, r, err)
		return
	}
	st, err := s.svc.GameRound(r.Context(), guildID, week)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toGameRoundDTO(st))
}

func (s *Server) gameRound(w http.ResponseWriter, r *http.Request) {
	week, ok := parseWeek(w, r.PathValue("week"))
	if !ok {
		return
	}
	st, err := s.svc.GameRound(r.Context(), r.PathValue("guildID"), week)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toGameRoundDTO(st))
}

// answerGame stores a member's answer from Discord: 403 for someone who isn't taking part and
// 409 once answers are closed.
func (s *Server) answerGame(w http.ResponseWriter, r *http.Request) {
	week, ok := parseWeek(w, r.PathValue("week"))
	if !ok {
		return
	}
	var in struct {
		Answer string `json:"answer"`
	}
	if !decode(w, r, &in) {
		return
	}
	a, err := s.svc.AnswerGame(r.Context(), usecase.AnswerGameInput{
		GuildID: r.PathValue("guildID"), UserID: r.PathValue("userID"), Week: week, Text: in.Answer,
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"week": a.Week.Format(dateLayout), "answer": a.Text})
}

// --- «Who's who?» game, on the dashboard ---

type gamePlayerDTO struct {
	DiscordID string `json:"discord_user_id"`
	Username  string `json:"username"`
}

type gameAnswerDTO struct {
	ID     string `json:"id"`
	Answer string `json:"answer"`
	Mine   bool   `json:"mine"`
	// Author is only set once the round is finished, and on the viewer's own answer.
	DiscordID string `json:"discord_user_id,omitempty"`
	Username  string `json:"username,omitempty"`
	// Right is how many guessed it, in finished rounds.
	Right *int `json:"right,omitempty"`
}

type gameScoreDTO struct {
	DiscordID string `json:"discord_user_id"`
	Username  string `json:"username"`
	Correct   int    `json:"correct"`
	Guesses   int    `json:"guesses"`
}

type gameRoundViewDTO struct {
	Week           string            `json:"week"`
	Question       string            `json:"question"`
	Phase          string            `json:"phase"`
	GuessingStarts string            `json:"guessing_starts"`
	LastDay        string            `json:"last_day"`
	Count          int               `json:"answer_count"`
	MinAnswers     int               `json:"min_answers"`
	MyAnswer       string            `json:"my_answer"`
	Answers        []gameAnswerDTO   `json:"answers"`
	Players        []gamePlayerDTO   `json:"players"`
	MyGuesses      map[string]string `json:"my_guesses"`
	Ranking        []gameScoreDTO    `json:"ranking"`
}

func toGameRoundViewDTO(rv *usecase.GameRoundView) *gameRoundViewDTO {
	if rv == nil {
		return nil
	}
	r := rv.Round
	out := &gameRoundViewDTO{
		Week: r.Week.Format(dateLayout), Question: r.Question, Phase: string(rv.Phase),
		GuessingStarts: r.GuessingStarts().Format(dateLayout), LastDay: r.LastDay().Format(dateLayout),
		Count: rv.Count, MinAnswers: domain.MinGameAnswers, MyAnswer: rv.MyAnswer.Text,
		Answers: []gameAnswerDTO{}, Players: []gamePlayerDTO{}, MyGuesses: rv.MyGuesses, Ranking: []gameScoreDTO{},
	}
	for _, a := range rv.Answers {
		d := gameAnswerDTO{ID: a.ID, Answer: a.Text, Mine: a.ID == rv.MyAnswer.ID, DiscordID: a.MemberDiscordID, Username: a.Username}
		if rv.Right != nil {
			n := rv.Right[a.ID]
			d.Right = &n
		}
		out.Answers = append(out.Answers, d)
	}
	for _, p := range rv.Players {
		out.Players = append(out.Players, gamePlayerDTO{p.DiscordID, p.Username})
	}
	for _, sc := range rv.Ranking {
		out.Ranking = append(out.Ranking, gameScoreDTO{sc.DiscordID, sc.Username, sc.Correct, sc.Guesses})
	}
	return out
}

// webGame is the game page of one server for one of its members: this week's round and last
// week's results. Servers without games, or where the viewer doesn't take part, are not found.
func (s *Server) webGame(w http.ResponseWriter, r *http.Request, v domain.Viewer) {
	b, err := s.svc.GameBoard(r.Context(), v, r.PathValue("guildID"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		DiscordGuildID string            `json:"discord_guild_id"`
		Name           string            `json:"name"`
		Current        *gameRoundViewDTO `json:"current"`
		Previous       *gameRoundViewDTO `json:"previous"`
	}{b.Guild.DiscordID, b.Guild.Name, toGameRoundViewDTO(b.Current), toGameRoundViewDTO(b.Previous)})
}

// webAnswerGame answers this week's question from the dashboard, for whoever missed the DM.
func (s *Server) webAnswerGame(w http.ResponseWriter, r *http.Request, v domain.Viewer) {
	if !requireJSON(w, r) {
		return
	}
	var in struct {
		Answer string `json:"answer"`
	}
	if !decode(w, r, &in) {
		return
	}
	guildID := r.PathValue("guildID")
	// Members who can't play get the same 404 as for a server they can't see.
	if _, err := s.svc.GameBoard(r.Context(), v, guildID); err != nil {
		s.fail(w, r, err)
		return
	}
	a, err := s.svc.AnswerGame(r.Context(), usecase.AnswerGameInput{GuildID: guildID, UserID: v.DiscordID, Text: in.Answer})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"week": a.Week.Format(dateLayout), "answer": a.Text})
}

// webGuessGame replaces the viewer's guesses for this week: {"guesses": {answerID: userID}}.
func (s *Server) webGuessGame(w http.ResponseWriter, r *http.Request, v domain.Viewer) {
	if !requireJSON(w, r) {
		return
	}
	var in struct {
		Guesses map[string]string `json:"guesses"`
	}
	if !decode(w, r, &in) {
		return
	}
	if err := s.svc.GuessGame(r.Context(), v, r.PathValue("guildID"), in.Guesses); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
