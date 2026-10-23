package usecase

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/moixblau/HappyX.Api/internal/domain"
)

// --- «Who's who?» game ---

// gameGuild returns the guild if it is active and has games turned on.
func (s *Service) gameGuild(ctx context.Context, guildID string) (domain.Guild, error) {
	g, err := s.activeGuild(ctx, guildID)
	if err != nil {
		return domain.Guild{}, err
	}
	if !g.Settings.GamesEnabled {
		return domain.Guild{}, domain.ErrGamesOff
	}
	return g, nil
}

// StartGameRound opens the guild's round for week with the question the bot picked, in the
// guild's language. Starting a round that exists returns it unchanged, so the bot can retry.
// A round can only start while its answering days are still ahead.
func (s *Service) StartGameRound(ctx context.Context, guildID string, week time.Time, question string) (domain.GameRound, error) {
	g, err := s.gameGuild(ctx, guildID)
	if err != nil {
		return domain.GameRound{}, err
	}
	r := domain.GameRound{GuildDiscordID: guildID, Week: domain.TruncateDay(week), Question: strings.TrimSpace(question)}
	if err := r.Validate(); err != nil {
		return domain.GameRound{}, err
	}
	today := s.today(g)
	if r.Week.After(today) || r.Phase(today) != domain.GameAnswering {
		return domain.GameRound{}, domain.ErrClosed
	}
	return s.games.CreateRound(ctx, r)
}

// GameRoundStatus is what the bot needs to know about a round.
type GameRoundStatus struct {
	Round   domain.GameRound
	Phase   domain.GamePhase
	Answers int
}

func (s *Service) GameRound(ctx context.Context, guildID string, week time.Time) (GameRoundStatus, error) {
	g, err := s.activeGuild(ctx, guildID)
	if err != nil {
		return GameRoundStatus{}, err
	}
	r, err := s.games.GetRound(ctx, guildID, domain.TruncateDay(week))
	if err != nil {
		return GameRoundStatus{}, err
	}
	answers, err := s.games.ListAnswers(ctx, guildID, r.Week)
	if err != nil {
		return GameRoundStatus{}, err
	}
	return GameRoundStatus{Round: r, Phase: r.Phase(s.today(g)), Answers: len(answers)}, nil
}

type AnswerGameInput struct {
	GuildID string
	UserID  string
	// Week is the round's Monday; zero means this week's round.
	Week time.Time
	Text string
}

// AnswerGame stores a member's answer while the round takes answers. Members who paused
// HappyX in the guild can't play.
func (s *Service) AnswerGame(ctx context.Context, in AnswerGameInput) (domain.GameAnswer, error) {
	g, err := s.gameGuild(ctx, in.GuildID)
	if err != nil {
		return domain.GameAnswer{}, err
	}
	if err := s.activeMember(ctx, in.GuildID, in.UserID); err != nil {
		return domain.GameAnswer{}, err
	}
	today := s.today(g)
	week := domain.TruncateDay(in.Week)
	if week.IsZero() {
		week = domain.WeekStart(today)
	}
	r, err := s.games.GetRound(ctx, in.GuildID, week)
	if err != nil {
		return domain.GameAnswer{}, err
	}
	if r.Phase(today) != domain.GameAnswering {
		return domain.GameAnswer{}, domain.ErrClosed
	}
	a := domain.GameAnswer{GuildDiscordID: in.GuildID, Week: r.Week, MemberDiscordID: in.UserID, Text: strings.TrimSpace(in.Text)}
	if err := a.Validate(); err != nil {
		return domain.GameAnswer{}, err
	}
	return s.games.UpsertAnswer(ctx, a)
}

// GameGuilds lists the guilds where the viewer can play: games are on and they take part.
func (s *Service) GameGuilds(ctx context.Context, v domain.Viewer) ([]domain.Guild, error) {
	if v.DiscordID == "" {
		return nil, nil
	}
	gs, err := s.guilds.ListJoined(ctx, v.DiscordID)
	if err != nil {
		return nil, err
	}
	out := gs[:0]
	for _, g := range gs {
		if g.Settings.GamesEnabled {
			out = append(out, g)
		}
	}
	return out, nil
}

// GamePlayer is someone who answered a round, as offered to guessers.
type GamePlayer struct {
	DiscordID string
	Username  string
}

// GameRoundView is a round as one player sees it. Who wrote what stays hidden until the round
// is finished: before that, answers carry no author except the viewer's own.
type GameRoundView struct {
	Round    domain.GameRound
	Phase    domain.GamePhase
	Count    int  // answers so far
	Enough   bool // at least domain.MinGameAnswers, so there is something to guess
	MyAnswer domain.GameAnswer
	// Answers are empty while answering and when there aren't enough of them.
	Answers []domain.GameAnswer
	// Players are the authors, sorted by name, while guessing.
	Players   []GamePlayer
	MyGuesses map[string]string // answer ID -> guessed user ID
	// Finished rounds only: how many guessed each answer right, and the ranking.
	Right   map[string]int
	Ranking []domain.GameScore
}

// GameBoard is the game page for one guild: this week's round and last week's results.
type GameBoard struct {
	Guild    domain.Guild
	Current  *GameRoundView
	Previous *GameRoundView
}

// GameBoard shows the game to one of the guild's members. Guilds without games, or where the
// viewer doesn't take part, are reported as not found.
func (s *Service) GameBoard(ctx context.Context, v domain.Viewer, guildID string) (GameBoard, error) {
	g, err := s.playerGuild(ctx, v, guildID)
	if err != nil {
		return GameBoard{}, err
	}
	today := s.today(g)
	week := domain.WeekStart(today)
	b := GameBoard{Guild: g}
	if b.Current, err = s.roundView(ctx, v, g, week, today); err != nil {
		return GameBoard{}, err
	}
	if b.Previous, err = s.roundView(ctx, v, g, week.AddDate(0, 0, -7), today); err != nil {
		return GameBoard{}, err
	}
	return b, nil
}

// roundView returns nil when the guild had no round that week.
func (s *Service) roundView(ctx context.Context, v domain.Viewer, g domain.Guild, week, today time.Time) (*GameRoundView, error) {
	r, err := s.games.GetRound(ctx, g.DiscordID, week)
	if err == domain.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	answers, err := s.games.ListAnswers(ctx, g.DiscordID, week)
	if err != nil {
		return nil, err
	}
	rv := &GameRoundView{
		Round: r, Phase: r.Phase(today), Count: len(answers), Enough: len(answers) >= domain.MinGameAnswers,
		Answers: []domain.GameAnswer{}, MyGuesses: map[string]string{},
	}
	for _, a := range answers {
		if a.MemberDiscordID == v.DiscordID {
			rv.MyAnswer = a
		}
	}
	if rv.Phase == domain.GameAnswering || !rv.Enough {
		return rv, nil
	}
	guesses, err := s.games.ListGuesses(ctx, g.DiscordID, week)
	if err != nil {
		return nil, err
	}
	for _, gs := range guesses {
		if gs.GuesserDiscordID == v.DiscordID {
			rv.MyGuesses[gs.AnswerID] = gs.GuessedDiscordID
		}
	}
	if rv.Phase == domain.GameGuessing {
		for _, a := range answers {
			rv.Players = append(rv.Players, GamePlayer{DiscordID: a.MemberDiscordID, Username: a.Username})
			if a.MemberDiscordID != v.DiscordID {
				a.MemberDiscordID, a.Username = "", ""
			}
			rv.Answers = append(rv.Answers, a)
		}
		sort.Slice(rv.Players, func(i, j int) bool {
			if a, b := strings.ToLower(rv.Players[i].Username), strings.ToLower(rv.Players[j].Username); a != b {
				return a < b
			}
			return rv.Players[i].DiscordID < rv.Players[j].DiscordID
		})
		return rv, nil
	}
	rv.Answers = answers
	usernames := map[string]string{}
	ms, err := s.members.ListActive(ctx, g.DiscordID)
	if err != nil {
		return nil, err
	}
	for _, m := range ms {
		usernames[m.DiscordID] = m.Username
	}
	author := map[string]string{}
	for _, a := range answers {
		author[a.ID] = a.MemberDiscordID
	}
	rv.Right = map[string]int{}
	for _, gs := range guesses {
		if author[gs.AnswerID] == gs.GuessedDiscordID {
			rv.Right[gs.AnswerID]++
		}
	}
	rv.Ranking = domain.Ranking(answers, guesses, usernames)
	return rv, nil
}

// GuessGame replaces the viewer's guesses for this week's round: answer ID -> author's user ID.
// Guesses are only taken while guessing is open; a player can't guess their own answer or name
// themselves, and every name must be one of the round's authors.
func (s *Service) GuessGame(ctx context.Context, v domain.Viewer, guildID string, guesses map[string]string) error {
	g, err := s.playerGuild(ctx, v, guildID)
	if err != nil {
		return err
	}
	today := s.today(g)
	week := domain.WeekStart(today)
	r, err := s.games.GetRound(ctx, guildID, week)
	if err != nil {
		return err
	}
	answers, err := s.games.ListAnswers(ctx, guildID, week)
	if err != nil {
		return err
	}
	if r.Phase(today) != domain.GameGuessing || len(answers) < domain.MinGameAnswers {
		return domain.ErrClosed
	}
	author, authors := map[string]string{}, map[string]bool{}
	for _, a := range answers {
		author[a.ID] = a.MemberDiscordID
		authors[a.MemberDiscordID] = true
	}
	out := make([]domain.GameGuess, 0, len(guesses))
	for answerID, who := range guesses {
		owner, ok := author[answerID]
		switch {
		case !ok:
			return domain.ValidationError{Reason: "unknown answer " + answerID}
		case owner == v.DiscordID:
			return domain.ValidationError{Reason: "you can't guess your own answer"}
		case who == v.DiscordID || !authors[who]:
			return domain.ValidationError{Reason: "guesses must name someone else who answered"}
		}
		out = append(out, domain.GameGuess{GuesserDiscordID: v.DiscordID, AnswerID: answerID, GuessedDiscordID: who})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AnswerID < out[j].AnswerID })
	return s.games.ReplaceGuesses(ctx, guildID, week, v.DiscordID, out)
}

// playerGuild returns the guild if the viewer can play there.
func (s *Service) playerGuild(ctx context.Context, v domain.Viewer, guildID string) (domain.Guild, error) {
	if v.DiscordID == "" {
		return domain.Guild{}, domain.ErrNotFound
	}
	g, err := s.gameGuild(ctx, guildID)
	if err != nil {
		if err == domain.ErrInactive || err == domain.ErrGamesOff {
			return domain.Guild{}, domain.ErrNotFound
		}
		return domain.Guild{}, err
	}
	if err := s.activeMember(ctx, guildID, v.DiscordID); err != nil {
		if err == domain.ErrNotAMember {
			return domain.Guild{}, domain.ErrNotFound
		}
		return domain.Guild{}, err
	}
	return g, nil
}

func (s *Service) activeMember(ctx context.Context, guildID, userID string) error {
	m, err := s.members.Get(ctx, guildID, userID)
	if err == domain.ErrNotFound || (err == nil && !m.Active) {
		return domain.ErrNotAMember
	}
	return err
}
