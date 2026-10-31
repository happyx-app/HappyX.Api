package demo

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/happyx-app/HappyX.Api/internal/domain"
	"github.com/happyx-app/HappyX.Api/internal/usecase"
)

// ErrReadOnly is what every write gets: the demo can be looked at, not changed.
var ErrReadOnly = domain.ErrForbidden

// Repositories returns the demo's repositories, in usecase.NewService's order.
func (s *Store) Repositories() (usecase.GuildRepository, usecase.MemberRepository, usecase.MoodRepository,
	usecase.DispatchRepository, usecase.KudosRepository, usecase.GameRepository, usecase.AccessRepository,
	usecase.PrivacyRepository) {
	return guilds{s}, members{s}, moods{s}, dispatches{s}, kudos{s}, games{s}, access{s}, privacy{s}
}

// NewService is a usecase.Service that only knows the demo server.
func (s *Store) NewService() *usecase.Service {
	g, m, mo, d, k, ga, a, p := s.Repositories()
	return usecase.NewService(g, m, mo, d, k, ga, a, p, s.now)
}

func inRange(day, from, to time.Time) bool { return !day.Before(from) && !day.After(to) }

type guilds struct{ s *Store }

func (r guilds) Get(_ context.Context, guildID string) (domain.Guild, error) {
	if guildID != GuildID {
		return domain.Guild{}, domain.ErrNotFound
	}
	return r.s.current().guild, nil
}

func (r guilds) ListActive(context.Context) ([]domain.Guild, error) {
	return []domain.Guild{r.s.current().guild}, nil
}

func (r guilds) ListVisible(_ context.Context, userID string, guildIDs []string) ([]domain.Guild, error) {
	g := r.s.current().guild
	if userID == g.AdminDiscordID {
		return []domain.Guild{g}, nil
	}
	for _, id := range guildIDs {
		if id == GuildID {
			return []domain.Guild{g}, nil
		}
	}
	return nil, nil
}

func (r guilds) ListJoined(_ context.Context, userID string) ([]domain.Guild, error) {
	d := r.s.current()
	for _, m := range d.members {
		if m.DiscordID == userID && m.Active && m.InServer {
			return []domain.Guild{d.guild}, nil
		}
	}
	return nil, nil
}

func (guilds) Upsert(context.Context, domain.Guild) (domain.Guild, error) {
	return domain.Guild{}, ErrReadOnly
}
func (guilds) UpdateConfig(context.Context, string, string, domain.GuildSettings) (domain.Guild, error) {
	return domain.Guild{}, ErrReadOnly
}
func (guilds) Deactivate(context.Context, string, time.Time) error { return ErrReadOnly }
func (guilds) Delete(context.Context, string) error                { return ErrReadOnly }
func (guilds) DeleteLeftBefore(context.Context, time.Time) ([]string, error) {
	return nil, nil
}
func (guilds) ListRemovals(context.Context) ([]string, error) { return nil, nil }
func (guilds) Removing(context.Context, string) (bool, error) { return false, nil }
func (guilds) DoneRemoving(context.Context, string) error     { return ErrReadOnly }

type members struct{ s *Store }

func (r members) Get(_ context.Context, guildID, userID string) (domain.Member, error) {
	if guildID == GuildID {
		for _, m := range r.s.current().members {
			if m.DiscordID == userID {
				return m, nil
			}
		}
	}
	return domain.Member{}, domain.ErrNotFound
}

func (r members) ListActive(_ context.Context, guildID string) ([]domain.Member, error) {
	var out []domain.Member
	if guildID == GuildID {
		for _, m := range r.s.current().members {
			if m.Active {
				out = append(out, m)
			}
		}
	}
	return out, nil
}

func (r members) ListInServer(_ context.Context, guildID string) ([]domain.Member, error) {
	var out []domain.Member
	if guildID == GuildID {
		for _, m := range r.s.current().members {
			if m.InServer {
				out = append(out, m)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Username) < strings.ToLower(out[j].Username) })
	return out, nil
}

func (members) Upsert(context.Context, domain.Member, *bool) (domain.Member, error) {
	return domain.Member{}, ErrReadOnly
}
func (members) SyncMany(context.Context, string, []domain.Member) error { return ErrReadOnly }
func (members) Deactivate(context.Context, string, string) error        { return ErrReadOnly }
func (members) Left(context.Context, string, string) error              { return ErrReadOnly }

type moods struct{ s *Store }

func (r moods) ListBetween(_ context.Context, guildID string, from, to time.Time) ([]domain.MoodEntry, error) {
	var out []domain.MoodEntry
	if guildID == GuildID {
		for _, e := range r.s.current().moods {
			if inRange(e.Day, from, to) {
				out = append(out, e)
			}
		}
	}
	return out, nil
}

func (r moods) ListForUser(_ context.Context, userID string, from, to time.Time) ([]domain.MoodEntry, error) {
	var out []domain.MoodEntry
	for _, e := range r.s.current().moods {
		if e.MemberDiscordID == userID && inRange(e.Day, from, to) {
			out = append(out, e)
		}
	}
	return out, nil
}

func (r moods) Streaks(_ context.Context, guildID string, _ domain.Weekdays, from, today time.Time) (map[string]int, error) {
	out := map[string]int{}
	if guildID != GuildID {
		return out, nil
	}
	d := r.s.current()
	answered := map[string]map[time.Time]bool{}
	for _, e := range d.moods {
		if inRange(e.Day, from, today) {
			if answered[e.MemberDiscordID] == nil {
				answered[e.MemberDiscordID] = map[time.Time]bool{}
			}
			answered[e.MemberDiscordID][e.Day] = true
		}
	}
	for id, days := range answered {
		if n := domain.Streak(d.guild, days, today); n > 0 {
			out[id] = n
		}
	}
	return out, nil
}

func (moods) Upsert(context.Context, domain.MoodEntry) (domain.MoodEntry, error) {
	return domain.MoodEntry{}, ErrReadOnly
}

type dispatches struct{ s *Store }

func (r dispatches) ListFinished(_ context.Context, guildID string, kind domain.DispatchKind, from, to time.Time) ([]domain.DispatchStat, error) {
	var out []domain.DispatchStat
	if guildID == GuildID && kind == domain.DispatchSurvey {
		for _, d := range r.s.current().dispatches {
			if inRange(d.Day, from, to) {
				out = append(out, d)
			}
		}
	}
	return out, nil
}

func (dispatches) Claim(context.Context, string, domain.DispatchKind, time.Time, int) (bool, error) {
	return false, ErrReadOnly
}
func (dispatches) Release(context.Context, string, domain.DispatchKind, time.Time) error {
	return ErrReadOnly
}
func (dispatches) Finish(context.Context, string, domain.DispatchKind, time.Time, int, int) error {
	return ErrReadOnly
}

type kudos struct{ s *Store }

func (r kudos) ListBetween(_ context.Context, guildID string, from, to time.Time) ([]domain.Kudos, error) {
	var out []domain.Kudos
	if guildID == GuildID {
		for _, k := range r.s.current().kudos {
			if inRange(k.Day, from, to) {
				out = append(out, k)
			}
		}
	}
	return out, nil
}

func (r kudos) CountGivenOn(_ context.Context, guildID, giverID string, day time.Time) (int, error) {
	n := 0
	if guildID == GuildID {
		for _, k := range r.s.current().kudos {
			if k.GiverDiscordID == giverID && k.Day.Equal(day) {
				n++
			}
		}
	}
	return n, nil
}

func (kudos) Create(context.Context, domain.Kudos) (domain.Kudos, error) {
	return domain.Kudos{}, ErrReadOnly
}

type games struct{ s *Store }

func (r games) GetRound(_ context.Context, guildID string, week time.Time) (domain.GameRound, error) {
	if guildID == GuildID {
		if round, ok := r.s.current().rounds[domain.TruncateDay(week)]; ok {
			return round, nil
		}
	}
	return domain.GameRound{}, domain.ErrNotFound
}

func (r games) ListAnswers(_ context.Context, guildID string, week time.Time) ([]domain.GameAnswer, error) {
	if guildID != GuildID {
		return nil, nil
	}
	return append([]domain.GameAnswer(nil), r.s.current().answers[domain.TruncateDay(week)]...), nil
}

func (r games) ListGuesses(_ context.Context, guildID string, week time.Time) ([]domain.GameGuess, error) {
	if guildID != GuildID {
		return nil, nil
	}
	return append([]domain.GameGuess(nil), r.s.current().guesses[domain.TruncateDay(week)]...), nil
}

func (r games) ListProposals(_ context.Context, guildID string, week time.Time) ([]domain.GameProposal, error) {
	if guildID != GuildID {
		return nil, nil
	}
	return append([]domain.GameProposal(nil), r.s.current().proposals[domain.TruncateDay(week)]...), nil
}

func (games) CreateRound(context.Context, domain.GameRound) (domain.GameRound, error) {
	return domain.GameRound{}, ErrReadOnly
}
func (games) UpsertAnswer(context.Context, domain.GameAnswer) (domain.GameAnswer, error) {
	return domain.GameAnswer{}, ErrReadOnly
}
func (games) ReplaceGuesses(context.Context, string, time.Time, string, []domain.GameGuess) error {
	return ErrReadOnly
}
func (games) UpsertProposal(context.Context, domain.GameProposal) (domain.GameProposal, error) {
	return domain.GameProposal{}, ErrReadOnly
}

type access struct{ s *Store }

func (access) ListRoles(_ context.Context, guildID string) ([]domain.Role, error) {
	if guildID != GuildID {
		return nil, nil
	}
	return append([]domain.Role(nil), roles...), nil
}

func (access) ListChannels(_ context.Context, guildID string) ([]domain.Channel, error) {
	if guildID != GuildID {
		return nil, nil
	}
	return append([]domain.Channel(nil), channels...), nil
}

func (access) ListGrants(_ context.Context, guildID string) ([]domain.AccessGrant, error) {
	if guildID != GuildID {
		return nil, nil
	}
	return append([]domain.AccessGrant(nil), grants...), nil
}

func (access) ReplaceRoles(context.Context, string, []domain.Role) error       { return ErrReadOnly }
func (access) ReplaceChannels(context.Context, string, []domain.Channel) error { return ErrReadOnly }
func (access) SetAccess(context.Context, string, string, []domain.AccessGrant) error {
	return ErrReadOnly
}

// privacy has nothing to export or erase: nobody in the demo is real.
type privacy struct{ s *Store }

func (privacy) UserData(context.Context, string) (domain.UserData, error) {
	return domain.UserData{}, domain.ErrNotFound
}
func (privacy) EraseUser(context.Context, string) error { return ErrReadOnly }
func (privacy) GuildData(context.Context, string) (domain.GuildData, error) {
	return domain.GuildData{}, domain.ErrNotFound
}
