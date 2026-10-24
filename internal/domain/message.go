package domain

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// MaxSurveyMessage is the longest custom survey text, in characters. The service adds the
// anonymity notice and the buttons below it.
const MaxSurveyMessage = 500

// SurveyMessageVars are the {{placeholders}} a custom survey message may use. The bot fills
// them in for each person when it sends the survey.
var SurveyMessageVars = []string{"user.name", "user.mention", "user.streak", "guild.name", "date", "weekday"}

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
	for _, v := range SurveyMessageVars {
		if v == name {
			return true
		}
	}
	return false
}
