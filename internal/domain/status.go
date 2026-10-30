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

// ProPrices are the answers to «How much would you pay?» on the Pro waitlist, in euros a
// month; "0" means nothing.
var ProPrices = []string{"0", "5", "9", "29"}

// ProInterest is a server's answer to the Pro waitlist on the dashboard. It names no person:
// it belongs to the server, and goes when the server is deleted.
type ProInterest struct {
	Interested bool
	Price      string // one of ProPrices, or "" when not answered
	UpdatedAt  time.Time
}

func (p ProInterest) Validate() error {
	if p.Price == "" {
		return nil
	}
	for _, v := range ProPrices {
		if p.Price == v {
			return nil
		}
	}
	return ValidationError{Reason: "price must be one of 0, 5, 9, 29"}
}
