package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type options struct {
	out     string
	random  bool
	hashLen int
	strict  bool
	partial bool
	mapFile string
}

type diag struct {
	file string
	msg  string
	warn bool
}

type item struct {
	jsonName     string
	q            *Question
	body         string
	mdName       string
	placeholders int
}

type imageData struct {
	uri  string
	size int
}

type imageCache map[string]imageData

const maxImageBytes = 1536 * 1024

func (c imageCache) load(fsys fs.FS, path string) (imageData, error) {
	if d, ok := c[path]; ok {
		return d, nil
	}
	b, err := fs.ReadFile(fsys, path)
	if err != nil {
		return imageData{}, fmt.Errorf("cannot read %s: %w", path, err)
	}
	mime := http.DetectContentType(b)
	switch mime {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
	default:
		return imageData{}, fmt.Errorf("%s is not a PNG, JPEG, GIF or WebP image (detected %s)", path, mime)
	}
	d := imageData{uri: "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(b), size: len(b)}
	c[path] = d
	return d, nil
}

func joinBlocks(blocks []string) string { return strings.Join(blocks, "\n\n") }

// renderMarkdown builds the question file. defs are the image definition lines.
func renderMarkdown(q *Question, defs []string) string {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "category: %s\n", yamlScalar(q.Category))
	fmt.Fprintf(&b, "tags: %s\n", yamlScalar(strings.Join(q.Tags, "; ")))
	fmt.Fprintf(&b, "source: %s\n", yamlScalar(q.Source))
	fmt.Fprintf(&b, "effort: %s\n", yamlScalar(q.Effort))
	fmt.Fprintf(&b, "knowledge: %s\n", yamlScalar(q.Knowledge))
	fmt.Fprintf(&b, "answer: %d\n", q.Answer)
	b.WriteString("---\n\n# Content\n\n")
	b.WriteString(joinBlocks(q.Content))
	b.WriteString("\n\n# Alternatives\n\n")
	b.WriteString(joinBlocks(q.Alternatives))
	b.WriteString("\n")
	if len(defs) > 0 {
		b.WriteString("\n")
		b.WriteString(strings.Join(defs, "\n"))
		b.WriteString("\n")
	}
	return b.String()
}

// build reads, validates and renders every question. Names are assigned later.
func build(fsys fs.FS, o options) ([]*item, []diag) {
	var diags []diag
	files, _ := fs.Glob(fsys, "questions/*.json")
	sort.Strings(files)
	if len(files) == 0 {
		return nil, []diag{{file: "questions/", msg: "no .json files found"}}
	}
	cache := imageCache{}
	seen := map[string]string{}
	var items []*item
	for _, path := range files {
		name := filepath.Base(path)
		add := func(msgs []string, warn bool) {
			for _, m := range msgs {
				diags = append(diags, diag{file: name, msg: m, warn: warn})
			}
		}
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			add([]string{err.Error()}, false)
			continue
		}
		q, errs := decodeQuestion(data)
		if q != nil {
			var warns []string
			errs, warns = validate(q)
			add(warns, true)
		}
		if q != nil {
			key := q.Source + "\x00" + q.Slug
			if prev, dup := seen[key]; dup {
				errs = append(errs, fmt.Sprintf("same source and slug as %s", prev))
			}
			seen[key] = name
		}
		if len(errs) > 0 {
			add(errs, false)
			continue
		}
		var defs []string
		it := &item{jsonName: name, q: q}
		var imgErrs []string
		for _, im := range q.Images {
			if im.File == nil {
				defs = append(defs, fmt.Sprintf("[%s]: PLACEHOLDER", im.ID))
				it.placeholders++
				add([]string{im.ID + " has no file; wrote PLACEHOLDER"}, true)
				continue
			}
			d, err := cache.load(fsys, *im.File)
			if err != nil {
				imgErrs = append(imgErrs, im.ID+": "+err.Error())
				continue
			}
			if d.size > maxImageBytes {
				add([]string{fmt.Sprintf("%s: %s is %d KiB", im.ID, *im.File, d.size/1024)}, true)
			}
			defs = append(defs, fmt.Sprintf("[%s]: %s", im.ID, d.uri))
		}
		if len(imgErrs) > 0 {
			add(imgErrs, false)
			continue
		}
		it.body = renderMarkdown(q, defs)
		items = append(items, it)
	}
	return items, diags
}

func identifier(q *Question, n int, random bool) string {
	if random {
		b := make([]byte, (n+1)/2)
		if _, err := rand.Read(b); err != nil {
			panic(err)
		}
		return hex.EncodeToString(b)[:n]
	}
	sum := sha256.Sum256([]byte(q.Source + "\x00" + q.Slug))
	return hex.EncodeToString(sum[:])[:n]
}

// assignNames sets <slug>-<id>.md. The default id is the first hashLen hex
// characters of SHA-256(source NUL slug), so re-running gives the same names.
func assignNames(items []*item, o options) []diag {
	var diags []diag
	used := map[string]string{}
	for _, it := range items {
		n := o.hashLen
		for {
			name := it.q.Slug + "-" + identifier(it.q, n, o.random) + ".md"
			_, taken := used[name]
			if o.random && !taken {
				if _, err := os.Stat(filepath.Join(o.out, name)); err == nil {
					taken = true
				}
			}
			if !taken {
				it.mdName = name
				used[name] = it.jsonName
				break
			}
			if !o.random {
				n += 2
				if n > 64 {
					diags = append(diags, diag{file: it.jsonName, msg: "cannot find a unique identifier"})
					break
				}
			}
		}
	}
	return diags
}
