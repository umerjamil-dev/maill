package mailer

import (
	"html"
	"regexp"
	"strings"
)

// mergeTagPattern matches {{ .Name }} / {{name}} style placeholders.
var mergeTagPattern = regexp.MustCompile(`\{\{\s*\.?([A-Za-z0-9_]+)\s*\}\}`)

// Render applies merge tags against subscriber fields.
// Available tags: subscriber custom fields, plus built-ins
// name, email, unsubscribe_url.
func Render(tpl string, vars map[string]string) string {
	return mergeTagPattern.ReplaceAllStringFunc(tpl, func(match string) string {
		key := mergeTagPattern.FindStringSubmatch(match)[1]
		if v, ok := vars[key]; ok {
			return v
		}
		return ""
	})
}

// RenderSubject applies merge tags to a subject line (escaped-safe).
func RenderSubject(subject string, vars map[string]string) string {
	return html.UnescapeString(Render(subject, vars))
}

// EscapeHTMLValue prepares a subscriber-supplied value for HTML context.
func EscapeHTMLValue(s string) string {
	return strings.TrimSpace(html.EscapeString(s))
}
