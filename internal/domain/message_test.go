package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateSurveyMessage(t *testing.T) {
	ok := []string{
		"",
		"👋 Hola! Como te encuentras hoy {{user.name}}? ({{guild.name}})",
		"{{ user.mention }}, {{weekday}} {{date}}: llevas {{user.streak}} días 🔥",
		"Sin variables, solo **markdown**",
		strings.Repeat("é", MaxSurveyMessage),
	}
	for _, m := range ok {
		if err := ValidateSurveyMessage(m); err != nil {
			t.Errorf("%q: %v", m, err)
		}
	}
	bad := []string{
		"Hola {{user.nmae}}",
		"Hola {{user.guild}}",
		"Hola {{}}",
		"Hola {{user.name}",
		"Hola user.name}}",
		strings.Repeat("a", MaxSurveyMessage+1),
	}
	for _, m := range bad {
		if err := ValidateSurveyMessage(m); !errors.Is(err, ErrInvalid) {
			t.Errorf("%q: want ErrInvalid, got %v", m, err)
		}
	}
}
