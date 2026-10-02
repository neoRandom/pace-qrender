package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// Question is the JSON record produced by the conversion agent.
type Question struct {
	Slug         string   `json:"slug"`
	Category     string   `json:"category"`
	Tags         []string `json:"tags"`
	Source       string   `json:"source"`
	Effort       string   `json:"effort"`
	Knowledge    string   `json:"knowledge"`
	Answer       int      `json:"answer"`
	Content      []string `json:"content"`
	Alternatives []string `json:"alternatives"`
	Images       []Image  `json:"images"`
}

type Image struct {
	ID   string  `json:"id"`
	File *string `json:"file"`
}

var requiredKeys = []string{"slug", "category", "tags", "source", "effort", "knowledge", "answer", "content", "alternatives", "images"}

var (
	reSlug       = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+){1,5}$`)
	reKnowledge  = regexp.MustCompile(`^(None|~10+)$`)
	reImageRef   = regexp.MustCompile(`!\[((?:\\.|[^\]\\\n])*)\]\[(image[0-9]+)\]`)
	reCurrency   = regexp.MustCompile(`\b(?:R|US)\$\s?[0-9]`)
	reTableSep   = regexp.MustCompile(`^\|?\s*:?-+:?\s*(\|\s*:?-+:?\s*)*\|?\s*$`)
	reAltLabel   = regexp.MustCompile(`^(\(?[A-Ea-e]\)|[A-E][.)]\s|[0-9]+[.)]\s)`)
	reInlineCode = regexp.MustCompile("`[^`\n]*`")
	reListLine   = regexp.MustCompile(`(?m)^\s*([0-9]+[.)]|[-*+])\s`)
	reHeading    = regexp.MustCompile(`(?m)^#{1,6}\s`)
)

// decodeQuestion decodes strictly: exactly the ten keys, right types.
func decodeQuestion(data []byte) (*Question, []string) {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, []string{"invalid JSON: " + err.Error()}
	}
	var errs []string
	for _, k := range requiredKeys {
		if _, ok := probe[k]; !ok {
			errs = append(errs, "missing key "+k)
		}
	}
	var unknown []string
	for k := range probe {
		known := false
		for _, r := range requiredKeys {
			if k == r {
				known = true
			}
		}
		if !known {
			unknown = append(unknown, k)
		}
	}
	sort.Strings(unknown)
	for _, k := range unknown {
		errs = append(errs, "unknown key "+k)
	}
	if len(errs) > 0 {
		return nil, errs
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var q Question
	if err := dec.Decode(&q); err != nil {
		return nil, []string{"wrong field type: " + err.Error()}
	}
	return &q, nil
}

func normalizeBlocks(blocks []string) []string {
	out := make([]string, len(blocks))
	for i, b := range blocks {
		out[i] = strings.Trim(strings.ReplaceAll(b, "\r\n", "\n"), "\n")
	}
	return out
}

// validate returns blocking errors and non-blocking warnings.
func validate(q *Question) (errs, warns []string) {
	q.Content = normalizeBlocks(q.Content)
	q.Alternatives = normalizeBlocks(q.Alternatives)

	if !reSlug.MatchString(q.Slug) {
		errs = append(errs, fmt.Sprintf("slug %q must be 2 to 6 lowercase ASCII words joined by hyphens", q.Slug))
	}
	errs = append(errs, checkMeta("category", q.Category)...)
	errs = append(errs, checkMeta("source", q.Source)...)
	errs = append(errs, checkMeta("effort", q.Effort)...)
	if !reKnowledge.MatchString(q.Knowledge) {
		errs = append(errs, fmt.Sprintf("knowledge %q must be None or ~10, ~100, ~1000, …", q.Knowledge))
	}
	for i, t := range q.Tags {
		errs = append(errs, checkMeta(fmt.Sprintf("tags[%d]", i), t)...)
		if strings.Contains(t, ";") {
			errs = append(errs, fmt.Sprintf("tags[%d] contains ';' (the separator)", i))
		}
	}
	if len(q.Tags) == 0 {
		warns = append(warns, "tags is empty")
	}
	if len(q.Content) == 0 {
		errs = append(errs, "content has no blocks")
	}
	if len(q.Alternatives) < 2 {
		errs = append(errs, "alternatives needs at least 2 entries")
	}
	if q.Answer < 0 || q.Answer >= len(q.Alternatives) {
		errs = append(errs, fmt.Sprintf("answer %d is outside 0..%d", q.Answer, len(q.Alternatives)-1))
	}

	var refs []string
	collect := func(where string, blocks []string, isAlt bool) {
		for i, b := range blocks {
			name := fmt.Sprintf("%s[%d]", where, i)
			if strings.TrimSpace(b) == "" {
				errs = append(errs, name+" is empty")
				continue
			}
			e, w, ids := checkBlock(b)
			for _, m := range e {
				errs = append(errs, name+": "+m)
			}
			for _, m := range w {
				warns = append(warns, name+": "+m)
			}
			refs = append(refs, ids...)
			if isAlt && reAltLabel.MatchString(b) {
				warns = append(warns, name+": starts with a letter or number label; alternatives must be unnumbered")
			}
		}
	}
	collect("content", q.Content, false)
	collect("alternatives", q.Alternatives, true)

	for i, id := range refs {
		if want := fmt.Sprintf("image%d", i+1); id != want {
			errs = append(errs, fmt.Sprintf("image references must run image1, image2, … in order of appearance; found %s at position %d", id, i+1))
			break
		}
	}
	if len(q.Images) != len(refs) {
		errs = append(errs, fmt.Sprintf("images lists %d entries but the text references %d", len(q.Images), len(refs)))
	}
	for i, im := range q.Images {
		if i < len(refs) && im.ID != refs[i] {
			errs = append(errs, fmt.Sprintf("images[%d].id is %q, expected %q", i, im.ID, refs[i]))
		}
		if im.File != nil {
			f := *im.File
			if !fs.ValidPath(f) || !strings.HasPrefix(f, "images/") || strings.HasSuffix(f, "/") {
				errs = append(errs, fmt.Sprintf("images[%d].file %q must be a relative path under images/", i, f))
			}
		}
	}
	return errs, warns
}

func checkMeta(name, v string) []string {
	var errs []string
	if strings.TrimSpace(v) == "" {
		return []string{name + " is empty"}
	}
	if v != strings.TrimSpace(v) {
		errs = append(errs, name+" has leading or trailing whitespace")
	}
	for _, r := range v {
		if unicode.IsControl(r) {
			errs = append(errs, name+" contains a control character or line break")
			break
		}
	}
	return errs
}

// stripCode removes fenced code blocks and inline code spans.
func stripCode(s string) string {
	var kept []string
	inFence, mark := false, ""
	for _, line := range strings.Split(s, "\n") {
		t := strings.TrimSpace(line)
		if !inFence && (strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~")) {
			inFence, mark = true, t[:3]
			continue
		}
		if inFence {
			if strings.HasPrefix(t, mark) {
				inFence = false
			}
			continue
		}
		kept = append(kept, line)
	}
	return reInlineCode.ReplaceAllString(strings.Join(kept, "\n"), "")
}

func unescapedDollars(s string) int {
	n := 0
	r := []rune(s)
	for i := 0; i < len(r); i++ {
		if r[i] == '\\' {
			i++
			continue
		}
		if r[i] == '$' {
			n++
		}
	}
	return n
}

// checkBlock validates one Markdown block and returns the image ids it references.
func checkBlock(b string) (errs, warns, ids []string) {
	lines := strings.Split(b, "\n")
	inFence, mark := false, ""
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if !inFence && (strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~")) {
			inFence, mark = true, t[:3]
			continue
		}
		if inFence {
			if strings.HasPrefix(t, mark) {
				inFence = false
			}
			continue
		}
		if t == "" {
			errs = append(errs, fmt.Sprintf("blank line at line %d outside a code fence (a block is one paragraph, quote, table, or code block)", i+1))
		}
		if t == "# Content" || t == "# Alternatives" {
			errs = append(errs, fmt.Sprintf("line %d is a reserved heading", i+1))
		}
	}
	if inFence {
		errs = append(errs, "unclosed code fence")
	}
	if strings.Contains(b, "<!--") {
		errs = append(errs, "contains an HTML comment")
	}

	plain := stripCode(b)
	if strings.HasPrefix(strings.TrimSpace(b), ">") {
		for i, line := range lines {
			if !strings.HasPrefix(strings.TrimLeft(line, " "), ">") {
				errs = append(errs, fmt.Sprintf("blockquote line %d does not start with '>'", i+1))
				break
			}
		}
	}
	if reCurrency.MatchString(plain) {
		errs = append(errs, `unescaped currency sign; write R\$ 12,00`)
	}
	if unescapedDollars(plain)%2 == 1 {
		errs = append(errs, "odd number of unescaped $ (unterminated math, or a currency sign that is not escaped)")
	}

	matches := reImageRef.FindAllStringSubmatch(plain, -1)
	for _, m := range matches {
		ids = append(ids, m[2])
	}
	if strings.Count(plain, "![") != len(matches) {
		errs = append(errs, "malformed image reference; expected ![label][imageN] on one line")
	}

	if strings.HasPrefix(strings.TrimSpace(b), "|") {
		if len(lines) < 2 || !reTableSep.MatchString(strings.TrimSpace(lines[1])) {
			warns = append(warns, "table without a separator row on its second line")
		}
	}
	if reHeading.MatchString(plain) {
		warns = append(warns, "a line starts like a Markdown heading (escape '#' if literal)")
	}
	if reListLine.MatchString(plain) {
		warns = append(warns, "a line starts like a list item (escape '1.' or '-' if literal)")
	}
	return errs, warns, ids
}
