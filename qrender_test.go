package main

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func baseQ() map[string]any {
	return map[string]any{
		"slug": "consumo-medio-agua-tabela", "category": "Mathematics",
		"tags":   []string{"Statistics", "Arithmetic Mean"},
		"source": "Question 7 - Yellow Notebook - 1st Day - Example Exam 2025",
		"effort": "Minimal", "knowledge": "~10", "answer": 2,
		"content":      []string{"Qual foi o consumo médio mensal?"},
		"alternatives": []string{"12", "13,5", "15", "16,5", "18"},
		"images":       []any{},
	}
}

func mod(f func(m map[string]any)) map[string]any {
	m := baseQ()
	f(m)
	return m
}

func writeInput(t *testing.T, qs map[string]map[string]any, imgs map[string][]byte) string {
	t.Helper()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "questions"), 0o755)
	os.MkdirAll(filepath.Join(dir, "images"), 0o755)
	for name, q := range qs {
		b, _ := json.Marshal(q)
		os.WriteFile(filepath.Join(dir, "questions", name), b, 0o644)
	}
	for name, b := range imgs {
		os.WriteFile(filepath.Join(dir, "images", name), b, 0o644)
	}
	return dir
}

func pngBytes() []byte {
	im := image.NewRGBA(image.Rect(0, 0, 2, 2))
	im.Set(0, 0, color.RGBA{255, 0, 0, 255})
	var b bytes.Buffer
	png.Encode(&b, im)
	return b.Bytes()
}

func runTool(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var o, e bytes.Buffer
	code := run(args, &o, &e)
	return code, o.String(), e.String()
}

func TestGoldenRender(t *testing.T) {
	q := &Question{
		Slug: "x-y", Category: "Mathematics", Tags: []string{"Statistics", "Arithmetic Mean", "Tables"},
		Source: "Question 7 - Yellow Notebook - 1st Day - Example Exam 2025", Effort: "Minimal", Knowledge: "~10", Answer: 2,
		Content: []string{
			"O quadro mostra o consumo de água de uma residência em três meses consecutivos.",
			"| Mês | Consumo (m³) |\n|---|---|\n| Janeiro | 12 |\n| Fevereiro | 15 |\n| Março | 18 |",
			"Qual foi o consumo médio mensal, em m³, nesse período?",
		},
		Alternatives: []string{"12", "13,5", "15", "16,5", "18"},
	}
	want := "---\ncategory: Mathematics\ntags: Statistics; Arithmetic Mean; Tables\nsource: Question 7 - Yellow Notebook - 1st Day - Example Exam 2025\neffort: Minimal\nknowledge: ~10\nanswer: 2\n---\n\n# Content\n\nO quadro mostra o consumo de água de uma residência em três meses consecutivos.\n\n| Mês | Consumo (m³) |\n|---|---|\n| Janeiro | 12 |\n| Fevereiro | 15 |\n| Março | 18 |\n\nQual foi o consumo médio mensal, em m³, nesse período?\n\n# Alternatives\n\n12\n\n13,5\n\n15\n\n16,5\n\n18\n"
	if got := renderMarkdown(q, nil); got != want {
		t.Fatalf("golden mismatch:\n%s", got)
	}
}

func TestYAMLScalar(t *testing.T) {
	cases := map[string]string{
		"~10":                         "~10",
		"None":                        "None",
		"Question 3 - Green Notebook": "Question 3 - Green Notebook",
		"Prova 1: Linguagens":         `"Prova 1: Linguagens"`,
		"A # b":                       `"A # b"`,
		"[x]":                         `"[x]"`,
		"2026":                        `"2026"`,
		"yes":                         `"yes"`,
		"":                            `""`,
		`say "hi"`:                    `say "hi"`,
		`"quoted" start`:              `"\"quoted\" start"`,
		"ends:":                       `"ends:"`,
	}
	for in, want := range cases {
		if got := yamlScalar(in); got != want {
			t.Errorf("yamlScalar(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestValidationErrors(t *testing.T) {
	cases := []struct {
		name string
		m    map[string]any
		want string
	}{
		{"answer range", mod(func(m map[string]any) { m["answer"] = 5 }), "answer 5 is outside"},
		{"knowledge", mod(func(m map[string]any) { m["knowledge"] = "~12" }), "knowledge"},
		{"slug", mod(func(m map[string]any) { m["slug"] = "one" }), "slug"},
		{"one alternative", mod(func(m map[string]any) { m["alternatives"] = []string{"a"}; m["answer"] = 0 }), "at least 2"},
		{"currency", mod(func(m map[string]any) { m["content"] = []string{"Custa R$ 12,00."} }), "currency"},
		{"odd dollar", mod(func(m map[string]any) { m["content"] = []string{"Seja $x a variável."} }), "odd number"},
		{"blank line", mod(func(m map[string]any) { m["content"] = []string{"a\n\nb"} }), "blank line"},
		{"html comment", mod(func(m map[string]any) { m["content"] = []string{"a <!-- x -->"} }), "HTML comment"},
		{"blockquote", mod(func(m map[string]any) { m["content"] = []string{"> a\nb"} }), "blockquote"},
		{"reserved heading", mod(func(m map[string]any) { m["content"] = []string{"# Alternatives"} }), "reserved heading"},
		{"unclosed fence", mod(func(m map[string]any) { m["content"] = []string{"```go\nx := 1"} }), "unclosed"},
		{"image not declared", mod(func(m map[string]any) { m["content"] = []string{"![g][image1]"} }), "images lists 0"},
		{"image order", mod(func(m map[string]any) {
			m["content"] = []string{"![g][image2]"}
			m["images"] = []any{map[string]any{"id": "image2", "file": nil}}
		}), "in order of appearance"},
		{"image path", mod(func(m map[string]any) {
			m["content"] = []string{"![g][image1]"}
			m["images"] = []any{map[string]any{"id": "image1", "file": "../x.png"}}
		}), "relative path under images/"},
		{"malformed image", mod(func(m map[string]any) { m["content"] = []string{"![g] [image1]"} }), "malformed image"},
		{"unknown key", mod(func(m map[string]any) { m["extra"] = 1 }), "unknown key extra"},
		{"missing key", mod(func(m map[string]any) { delete(m, "effort") }), "missing key effort"},
		{"wrong type", mod(func(m map[string]any) { m["answer"] = 1.5 }), "wrong field type"},
		{"empty block", mod(func(m map[string]any) { m["content"] = []string{"  "} }), "empty"},
		{"tag semicolon", mod(func(m map[string]any) { m["tags"] = []string{"a;b"} }), "';'"},
	}
	for _, c := range cases {
		dir := writeInput(t, map[string]map[string]any{"001-a.json": c.m}, nil)
		code, _, stderr := runTool(t, "-o", filepath.Join(t.TempDir(), "o"), dir)
		if code != 1 || !strings.Contains(stderr, c.want) {
			t.Errorf("%s: code %d, stderr %q, want substring %q", c.name, code, stderr, c.want)
		}
	}
}

func TestValidEscapesAccepted(t *testing.T) {
	m := mod(func(m map[string]any) {
		m["content"] = []string{`Custa R\$ 12,00 e $x = 2$ e ` + "`a$b`" + ".", "```go\nx := \"$\"\n\ny := 2\n```"}
	})
	dir := writeInput(t, map[string]map[string]any{"001-a.json": m}, nil)
	if code, _, stderr := runTool(t, "-o", filepath.Join(t.TempDir(), "o"), dir); code != 0 {
		t.Fatalf("code %d: %s", code, stderr)
	}
}

func TestImageEmbeddedAndPlaceholder(t *testing.T) {
	img := pngBytes()
	m := mod(func(m map[string]any) {
		m["content"] = []string{"![Um gráfico com \\[0, 1)][image1]", "texto"}
		m["alternatives"] = []string{"![a][image2]", "b", "![c][image3]"}
		m["answer"] = 0
		m["images"] = []any{
			map[string]any{"id": "image1", "file": "images/p1-1.png"},
			map[string]any{"id": "image2", "file": "images/p1-1.png"},
			map[string]any{"id": "image3", "file": nil},
		}
	})
	dir := writeInput(t, map[string]map[string]any{"001-a.json": m}, map[string][]byte{"p1-1.png": img})
	out := filepath.Join(t.TempDir(), "o")
	code, stdout, stderr := runTool(t, "-o", out, dir)
	if code != 0 || !strings.Contains(stderr, "image3 has no file") || !strings.Contains(stdout, "1 PLACEHOLDER") {
		t.Fatalf("code %d\n%s\n%s", code, stdout, stderr)
	}
	files, _ := filepath.Glob(filepath.Join(out, "*.md"))
	b, _ := os.ReadFile(files[0])
	uri := "data:image/png;base64," + base64.StdEncoding.EncodeToString(img)
	for _, want := range []string{"[image1]: " + uri + "\n", "[image2]: " + uri + "\n", "[image3]: PLACEHOLDER\n"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("missing %.40s…", want)
		}
	}
	// strict mode turns the placeholder into an error
	if code, _, _ := runTool(t, "-strict", "-o", filepath.Join(t.TempDir(), "o2"), dir); code != 1 {
		t.Errorf("strict: code %d, want 1", code)
	}
}

func TestImageFileProblems(t *testing.T) {
	m := mod(func(m map[string]any) {
		m["content"] = []string{"![g][image1]"}
		m["images"] = []any{map[string]any{"id": "image1", "file": "images/p1-1.png"}}
	})
	dir := writeInput(t, map[string]map[string]any{"001-a.json": m}, nil)
	if code, _, e := runTool(t, "-o", filepath.Join(t.TempDir(), "o"), dir); code != 1 || !strings.Contains(e, "cannot read") {
		t.Errorf("missing file: %d %s", code, e)
	}
	dir = writeInput(t, map[string]map[string]any{"001-a.json": m}, map[string][]byte{"p1-1.png": []byte("not an image")})
	if code, _, e := runTool(t, "-o", filepath.Join(t.TempDir(), "o"), dir); code != 1 || !strings.Contains(e, "not a PNG") {
		t.Errorf("bad image: %d %s", code, e)
	}
}

func TestDeterministicNamesAndRandom(t *testing.T) {
	qs := map[string]map[string]any{"001-a.json": baseQ()}
	dir := writeInput(t, qs, nil)
	re := regexp.MustCompile(`consumo-medio-agua-tabela-[0-9a-f]{6}\.md`)
	_, o1, _ := runTool(t, "-o", filepath.Join(t.TempDir(), "a"), dir)
	_, o2, _ := runTool(t, "-o", filepath.Join(t.TempDir(), "b"), dir)
	n1, n2 := re.FindString(o1), re.FindString(o2)
	if n1 == "" || n1 != n2 {
		t.Fatalf("names differ or malformed: %q %q", n1, n2)
	}
	_, o3, _ := runTool(t, "-random", "-o", filepath.Join(t.TempDir(), "c"), dir)
	if !re.MatchString(o3) {
		t.Errorf("random name malformed: %s", o3)
	}
	_, o4, _ := runTool(t, "-hash-len", "10", "-o", filepath.Join(t.TempDir(), "d"), dir)
	if !regexp.MustCompile(`-[0-9a-f]{10}\.md`).MatchString(o4) {
		t.Errorf("hash-len ignored: %s", o4)
	}
}

func TestDuplicateSourceAndSlug(t *testing.T) {
	dir := writeInput(t, map[string]map[string]any{"001-a.json": baseQ(), "002-b.json": baseQ()}, nil)
	if code, _, e := runTool(t, "-o", filepath.Join(t.TempDir(), "o"), dir); code != 1 || !strings.Contains(e, "same source and slug") {
		t.Errorf("%d %s", code, e)
	}
}

func TestAllOrNothingAndPartial(t *testing.T) {
	good := baseQ()
	bad := mod(func(m map[string]any) {
		m["source"] = "Question 8 - Yellow Notebook - 1st Day - Example Exam 2025"
		m["answer"] = 9
	})
	dir := writeInput(t, map[string]map[string]any{"001-a.json": good, "002-b.json": bad}, nil)
	out := filepath.Join(t.TempDir(), "o")
	if code, _, _ := runTool(t, "-o", out, dir); code != 1 {
		t.Fatalf("code %d", code)
	}
	if _, err := os.Stat(out); err == nil {
		t.Errorf("output directory created despite errors")
	}
	if code, _, _ := runTool(t, "-partial", "-o", out, dir); code != 1 {
		t.Fatalf("partial: code %d", code)
	}
	if files, _ := filepath.Glob(filepath.Join(out, "*.md")); len(files) != 1 {
		t.Errorf("partial wrote %d files, want 1", len(files))
	}
}

func TestZipInputWithTopFolderAndMap(t *testing.T) {
	dir := t.TempDir()
	zpath := filepath.Join(dir, "questions.zip")
	f, _ := os.Create(zpath)
	zw := zip.NewWriter(f)
	b, _ := json.Marshal(baseQ())
	w, _ := zw.Create("exam/questions/001-a.json")
	w.Write(b)
	w, _ = zw.Create("__MACOSX/junk")
	w.Write([]byte("x"))
	zw.Close()
	f.Close()
	mp := filepath.Join(dir, "map.tsv")
	code, _, e := runTool(t, "-map", mp, "-o", filepath.Join(dir, "o"), zpath)
	if code != 0 {
		t.Fatalf("%d %s", code, e)
	}
	tsv, _ := os.ReadFile(mp)
	if !strings.HasPrefix(string(tsv), "001-a.json\tconsumo-medio-agua-tabela-") {
		t.Errorf("map: %q", tsv)
	}
}

func TestUsageErrors(t *testing.T) {
	if code, _, _ := runTool(t); code != 2 {
		t.Errorf("no args: %d", code)
	}
	if code, _, _ := runTool(t, "/nonexistent"); code != 2 {
		t.Errorf("missing input: %d", code)
	}
	if code, _, _ := runTool(t, "-hash-len", "2", t.TempDir()); code != 2 {
		t.Errorf("bad hash-len: %d", code)
	}
}
