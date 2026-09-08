// Package urlnorm normalizes user-typed URL strings so the WASM client can
// validate before submitting to the server's strict http/https check.
package urlnorm

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// urlInText runs to the first whitespace or character that cannot sit inside a
// URL; FindURLs trims trailing sentence punctuation.
var urlInText = regexp.MustCompile(`https?://[^\s"'<>` + "`" + `]+`)

// URLSpan's Col0 and Col1 are 1-based inclusive columns, the xterm link-range
// convention. Byte offsets equal columns for ASCII URLs.
type URLSpan struct {
	Col0, Col1 int
	URL        string
}

// FindURLs locates every http(s) URL in one line, for the xterm link provider.
// Trailing prose punctuation (.,;:!?) and an unbalanced ")" are excluded.
func FindURLs(text string) []URLSpan {
	var out []URLSpan
	for _, loc := range urlInText.FindAllStringIndex(text, -1) {
		trimmed := strings.TrimRight(text[loc[0]:loc[1]], ".,;:!?")
		// Drop a ")" closing a paren the URL never opened, as in
		// "(see https://x)", while keeping "/Foo_(bar)".
		for strings.HasSuffix(trimmed, ")") && strings.Count(trimmed, ")") > strings.Count(trimmed, "(") {
			trimmed = strings.TrimRight(trimmed[:len(trimmed)-1], ".,;:!?")
		}
		if trimmed == "" {
			continue
		}
		out = append(out, URLSpan{Col0: loc[0] + 1, Col1: loc[0] + len(trimmed), URL: trimmed})
	}
	return out
}

// Normalize prepends "https://" when the input carries no scheme. The error
// text is user-facing.
func Normalize(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", errors.New("please enter a URL")
	}
	if i := strings.Index(s, "://"); i >= 0 {
		scheme := strings.ToLower(s[:i])
		if scheme != "http" && scheme != "https" {
			return "", fmt.Errorf("only http and https URLs are allowed (got %q)", scheme)
		}
		rest := s[i+3:]
		if !looksLikeHost(hostPart(rest)) {
			return "", errors.New("that does not look like a URL")
		}
		return s, nil
	}
	host := hostPart(s)
	if !looksLikeHost(host) {
		return "", errors.New("please enter a valid URL (e.g. example.com)")
	}
	return "https://" + s, nil
}

// Candidate's Title is the url tile's alt_text, "" when never frozen.
type Candidate struct {
	URL, Title string
}

// Suggest ranks candidates for the new-url modal's autocomplete. The query
// matches the comparable address or a substring of the title; a prefix match
// ranks ahead of any other, and within a rank the caller's order is kept.
// Dedupe is by comparable address, so scheme and www variants collapse. Empty
// input returns the first limit distinct candidates; limit <= 0 returns nil.
func Suggest(input string, candidates []Candidate, limit int) []Candidate {
	if limit <= 0 {
		return nil
	}
	q := comparableURL(input)
	qTitle := strings.ToLower(strings.TrimSpace(input))
	seen := make(map[string]bool, len(candidates))
	var prefix, other []Candidate
	for _, c := range candidates {
		c.URL = strings.TrimSpace(c.URL)
		cmp := comparableURL(c.URL)
		if c.URL == "" || seen[cmp] {
			continue
		}
		seen[cmp] = true
		if q == "" {
			prefix = append(prefix, c)
			continue
		}
		switch idx := strings.Index(cmp, q); {
		case idx == 0:
			prefix = append(prefix, c)
		case idx > 0:
			other = append(other, c)
		case qTitle != "" && strings.Contains(strings.ToLower(c.Title), qTitle):
			other = append(other, c)
		}
	}
	out := append(prefix, other...)
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// comparableURL strips what a user does not type: case, scheme, "www.".
func comparableURL(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	for _, p := range []string{"https://", "http://"} {
		if strings.HasPrefix(s, p) {
			s = s[len(p):]
			break
		}
	}
	return strings.TrimPrefix(s, "www.")
}

// hostPart stops at the first `/`, `?`, or `#`, so path and query characters
// do not confuse looksLikeHost's dot check.
func hostPart(s string) string {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '/', '?', '#':
			return s[:i]
		}
	}
	return s
}

// looksLikeHost is deliberately lenient: a dot or "localhost", optionally with
// a :port. Internationalized domains and IPv6 literals are out of scope.
func looksLikeHost(s string) bool {
	if s == "" {
		return false
	}
	// Drop userinfo before the port check, or the password's colon reads as
	// the port separator and the server would accept what this rejects.
	if at := strings.LastIndex(s, "@"); at >= 0 {
		s = s[at+1:]
	}
	if i := strings.LastIndex(s, ":"); i > 0 {
		s = s[:i]
	}
	if s == "localhost" {
		return true
	}
	return strings.Contains(s, ".")
}
