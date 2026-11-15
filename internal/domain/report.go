package domain

import (
	"crypto/sha256"
	"slices"
	"strings"
	"time"
)

// Report summarises a guild's moods over a period. It is the data the bot turns into
// the image sent to the guild admin.
type Report struct {
	GuildDiscordID string
	From, To       time.Time // inclusive dates
	Anonymous      bool
	// AnonymityThreshold is the guild's: with anonymous answers from fewer people than this,
	// Withheld is set and the report keeps only how many answered.
	AnonymityThreshold int
	Withheld           bool
	Responses          int
	Respondents        int
	Average            float64      // 0 when there are no responses or Withheld
	Distribution       map[Mood]int // count per mood
	Days               []DayStat
	Messages           []ReportMessage
	Kudos              KudosSummary
	// Participation compares answers with surveys sent, on the days the bot reported how many
	// people it sent the survey to. Nil when it reported none in the period.
	Participation *Participation
}

// Participation is how many survey DMs went out and how many of them got an answer.
type Participation struct {
	Recipients int
	Answered   int
}

// AddParticipation fills each day's recipients from the survey dispatches and sums the
// days that have them. Answers on days without a known number of recipients are left out
// of both sides, so the rate is never above what the bot actually sent.
func (r *Report) AddParticipation(sent []DispatchStat) {
	byDay := map[time.Time]int{}
	for _, d := range sent {
		byDay[TruncateDay(d.Day)] = d.Recipients
	}
	for i, d := range r.Days {
		n, ok := byDay[TruncateDay(d.Day)]
		if !ok {
			continue
		}
		r.Days[i].Recipients = &n
		if r.Participation == nil {
			r.Participation = &Participation{}
		}
		r.Participation.Recipients += n
		r.Participation.Answered += min(d.Responses, n)
	}
}

// KudosSummary counts the kudos given in a report's period. Kudos are public, so they keep
// their names even when the guild's moods are anonymous.
type KudosSummary struct {
	Count     int
	Givers    int
	Receivers int
	// Highlights are the latest kudos, newest first, at most MaxKudosHighlights.
	Highlights []Kudos
	// TopReceivers are who got the most kudos, most first, at most MaxKudosTopReceivers.
	TopReceivers []KudosCount
}

// KudosCount is how many kudos one member received.
type KudosCount struct {
	DiscordID string
	Username  string
	Count     int
}

const (
	MaxKudosHighlights   = 5
	MaxKudosTopReceivers = 3
)

// SummarizeKudos counts kudos (ordered oldest first) and keeps the latest as highlights.
func SummarizeKudos(kudos []Kudos) KudosSummary {
	s := KudosSummary{Count: len(kudos), Highlights: []Kudos{}, TopReceivers: []KudosCount{}}
	givers := map[string]struct{}{}
	received := map[string]*KudosCount{}
	for _, k := range kudos {
		givers[k.GiverDiscordID] = struct{}{}
		c := received[k.ReceiverDiscordID]
		if c == nil {
			c = &KudosCount{DiscordID: k.ReceiverDiscordID}
			received[k.ReceiverDiscordID] = c
		}
		c.Count++
		c.Username = k.ReceiverUsername // the latest name wins
	}
	s.Givers, s.Receivers = len(givers), len(received)
	for _, c := range received {
		s.TopReceivers = append(s.TopReceivers, *c)
	}
	slices.SortFunc(s.TopReceivers, func(a, b KudosCount) int {
		if a.Count != b.Count {
			return b.Count - a.Count
		}
		return strings.Compare(strings.ToLower(a.Username), strings.ToLower(b.Username))
	})
	s.TopReceivers = s.TopReceivers[:min(len(s.TopReceivers), MaxKudosTopReceivers)]
	for i := len(kudos) - 1; i >= 0 && len(s.Highlights) < MaxKudosHighlights; i-- {
		s.Highlights = append(s.Highlights, kudos[i])
	}
	return s
}

type DayStat struct {
	Day       time.Time
	Responses int
	// Average is 0 when Withheld: that day's anonymous answers came from fewer people than
	// the guild's threshold.
	Average  float64
	Withheld bool
	// Recipients is how many people the bot sent that day's survey to; nil when unknown.
	Recipients *int
}

// ReportMessage is an optional comment left with a mood. An anonymous comment carries only
// its text: its day and mood are left out, since in a small team either can point to its
// author. Day, Mood, MemberDiscordID and Username are set only for a comment whose author is
// shown: the guild is not anonymous now and was not when it was written.
type ReportMessage struct {
	Day             time.Time
	Mood            Mood
	Message         string
	MemberDiscordID string
	Username        string
}

// Anonymous reports whether the comment hides its author.
func (m ReportMessage) Anonymous() bool { return m.MemberDiscordID == "" }

// BuildReport aggregates entries into a report, hiding identities when anonymous. An entry
// only shows its author when the guild is not anonymous now and was not when it was written.
// When any answer is anonymous and fewer people than the guild's threshold answered, the
// report is withheld: it keeps the counts and drops the average, the moods and the comments.
// A day with anonymous answers from fewer people than the threshold drops its average too.
// usernames maps member Discord IDs to display names.
func BuildReport(guild Guild, from, to time.Time, entries []MoodEntry, usernames map[string]string) Report {
	k := max(guild.Settings.AnonymityThreshold, MinAnonymityThreshold)
	r := Report{
		GuildDiscordID:     guild.DiscordID,
		From:               from,
		To:                 to,
		Anonymous:          guild.Settings.Anonymous,
		AnonymityThreshold: k,
		Distribution:       map[Mood]int{},
	}
	for m := MoodAwful; m <= MoodGreat; m++ {
		r.Distribution[m] = 0
	}

	type acc struct {
		n, sum    int
		people    map[string]struct{}
		anonymous bool
	}
	perDay := map[time.Time]*acc{}
	people := map[string]struct{}{}
	anonymous := guild.Settings.Anonymous
	total := 0
	var named, unnamed []ReportMessage
	for _, e := range entries {
		r.Responses++
		total += int(e.Mood)
		r.Distribution[e.Mood]++
		people[e.MemberDiscordID] = struct{}{}
		d := TruncateDay(e.Day)
		a := perDay[d]
		if a == nil {
			a = &acc{people: map[string]struct{}{}}
			perDay[d] = a
		}
		a.n++
		a.sum += int(e.Mood)
		a.people[e.MemberDiscordID] = struct{}{}
		shown := !guild.Settings.Anonymous && !e.Anonymous
		if !shown {
			anonymous = true
			a.anonymous = true
		}
		switch {
		case e.Message == "":
		case shown:
			named = append(named, ReportMessage{Day: d, Mood: e.Mood, Message: e.Message,
				MemberDiscordID: e.MemberDiscordID, Username: usernames[e.MemberDiscordID]})
		default:
			unnamed = append(unnamed, ReportMessage{Message: e.Message})
		}
	}
	r.Respondents = len(people)
	r.Withheld = anonymous && r.Respondents < k
	if r.Responses > 0 && !r.Withheld {
		r.Average = float64(total) / float64(r.Responses)
	}
	if r.Withheld {
		for m := range r.Distribution {
			r.Distribution[m] = 0
		}
	} else {
		// Anonymous comments come in an order that has nothing to do with when they were
		// written, yet stays the same each time the report is opened.
		key := func(m ReportMessage) []byte {
			h := sha256.Sum256([]byte(guild.DiscordID + "|" + from.Format("2006-01-02") + "|" + m.Message))
			return h[:]
		}
		slices.SortStableFunc(unnamed, func(a, b ReportMessage) int { return slices.Compare(key(a), key(b)) })
		r.Messages = append(named, unnamed...)
	}
	// Non-working days only show up when someone answered anyway.
	for d := TruncateDay(from); !d.After(TruncateDay(to)); d = d.AddDate(0, 0, 1) {
		a := perDay[d]
		if a == nil && !guild.Settings.IsWorkingDay(d) {
			continue
		}
		ds := DayStat{Day: d}
		if a != nil {
			ds.Responses = a.n
			ds.Withheld = r.Withheld || (a.anonymous && len(a.people) < k)
			if !ds.Withheld {
				ds.Average = float64(a.sum) / float64(a.n)
			}
		}
		r.Days = append(r.Days, ds)
	}
	return r
}
