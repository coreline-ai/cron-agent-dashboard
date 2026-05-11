package scheduler

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

var templateVarRE = regexp.MustCompile(`{{\s*([A-Za-z_][A-Za-z0-9_]*)\s*}}`)

var allowedTemplateVars = map[string]struct{}{
	"date":     {},
	"datetime": {},
	"time":     {},
}

func ValidateTemplate(tmpl string) error {
	for _, match := range templateVarRE.FindAllStringSubmatch(tmpl, -1) {
		if len(match) < 2 {
			continue
		}
		if _, ok := allowedTemplateVars[match[1]]; !ok {
			return fmt.Errorf("unknown template variable %q", match[1])
		}
	}
	return nil
}

func RenderTemplate(tmpl string, now time.Time) (string, error) {
	if err := ValidateTemplate(tmpl); err != nil {
		return "", err
	}
	values := map[string]string{
		"date":     now.Format("2006-01-02"),
		"datetime": now.Format("2006-01-02 15:04"),
		"time":     now.Format("15:04"),
	}
	out := templateVarRE.ReplaceAllStringFunc(tmpl, func(token string) string {
		match := templateVarRE.FindStringSubmatch(token)
		if len(match) < 2 {
			return token
		}
		return values[match[1]]
	})
	return strings.TrimSpace(out), nil
}
