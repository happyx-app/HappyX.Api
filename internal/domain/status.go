package domain

import "time"

// DispatchStat is one finished dispatch: how many people it went to and how many DMs failed.
type DispatchStat struct {
	Day        time.Time
	Recipients int
	Failed     int
}

// GuildHealth is what the dashboard needs to tell an admin whether HappyX is reaching their
// team: who the survey goes to today and how the last one went.
type GuildHealth struct {
	// Audience is how many members would get the survey now: opted in, still in the server
	// and allowed by the survey roles.
	Audience int
	// OptedOut is how many members still in the server said «don't ask me again».
	OptedOut int
	// LastSurvey is the latest survey the bot finished sending, nil before the first one.
	LastSurvey *DispatchStat
}

// The free plan the market study proposes: the survey reaches up to FreePlanPeople, and a
// guild counts as active with FreeLimitActive people answering in a week.
const (
	FreePlanPeople  = 15
	FreeLimitActive = 5
)
