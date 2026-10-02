package main

import (
	"regexp"
	"strings"
	"unicode"
)

var (
	reYAMLNumber = regexp.MustCompile(`^[-+]?([0-9][0-9_]*(\.[0-9_]*)?([eE][-+]?[0-9]+)?|\.[0-9_]+([eE][-+]?[0-9]+)?|0x[0-9a-fA-F_]+|\.inf|\.nan)$`)
	reYAMLDate   = regexp.MustCompile(`^[0-9]{4}-[0-9]{1,2}-[0-9]{1,2}`)
)

// yamlScalar returns s as a YAML scalar: plain when that is safe, double-quoted
// otherwise. Plain is kept whenever possible so that the files match the
// format document (for example `knowledge: ~10`).
func yamlScalar(s string) string {
	if needsQuote(s) {
		return yamlQuote(s)
	}
	return s
}

func needsQuote(s string) bool {
	if s == "" || s != strings.TrimSpace(s) {
		return true
	}
	if strings.ContainsAny(s[:1], "-?:,[]{}#&*!|>'\"%@`") {
		return true
	}
	if strings.Contains(s, ": ") || strings.Contains(s, " #") || strings.HasSuffix(s, ":") {
		return true
	}
	switch strings.ToLower(s) {
	case "~", "null", "true", "false", "yes", "no", "on", "off", "y", "n":
		return true
	}
	if reYAMLNumber.MatchString(s) || reYAMLDate.MatchString(s) {
		return true
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

func yamlQuote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
