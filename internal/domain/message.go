package domain

import (
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

// MaxSurveyMessage is the longest custom survey text, in characters. The service adds the
// anonymity notice and the buttons below it.
const MaxSurveyMessage = 500

// SurveyMessageVars are the {{placeholders}} a custom survey message may use. The bot fills
// them in for each person when it sends the survey.
var SurveyMessageVars = []string{"user.name", "user.mention", "guild.name", "date", "weekday"}

// RetiredSurveyMessageVars are placeholders a message saved earlier may still hold, and that the
// bot still fills in, but that a new or changed message can't use. {{user.streak}} turned a
// wellbeing check-in into a streak to keep.
var RetiredSurveyMessageVars = []string{"user.streak"}

// CheckNewSurveyMessage is ValidateSurveyMessage for a message being saved now: it also refuses
// the retired placeholders.
func CheckNewSurveyMessage(msg string) error {
	for _, m := range placeholder.FindAllStringSubmatch(msg, -1) {
		if slices.Contains(RetiredSurveyMessageVars, m[1]) {
			return invalid("survey_message can no longer use {{%s}}", m[1])
		}
	}
	return ValidateSurveyMessage(msg)
}

var placeholder = regexp.MustCompile(`\{\{\s*([^{}]*?)\s*\}\}`)

// ValidateSurveyMessage checks an admin's custom survey text: empty means the default text,
// otherwise it must fit MaxSurveyMessage and only use known placeholders.
func ValidateSurveyMessage(msg string) error {
	if utf8.RuneCountInString(msg) > MaxSurveyMessage {
		return invalid("survey_message must be at most %d characters", MaxSurveyMessage)
	}
	for _, m := range placeholder.FindAllStringSubmatch(msg, -1) {
		if !knownVar(m[1]) {
			return invalid("survey_message uses an unknown variable {{%s}}", m[1])
		}
	}
	if rest := placeholder.ReplaceAllString(msg, ""); strings.Contains(rest, "{{") || strings.Contains(rest, "}}") {
		return invalid("survey_message has a variable that is not closed, write it like {{user.name}}")
	}
	return nil
}

func knownVar(name string) bool {
	if slices.Contains(RetiredSurveyMessageVars, name) {
		return true // in a message saved before it was retired
	}
	for _, v := range SurveyMessageVars {
		if v == name {
			return true
		}
	}
	return false
}
