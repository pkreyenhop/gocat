package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gc/editor"
)

func TestDetectSyntaxByPath(t *testing.T) {
	tests := []struct {
		path string
		src  string
		want syntaxKind
	}{
		{path: "a.go", want: syntaxGo},
		{path: "a.md", want: syntaxMarkdown},
		{path: "a.markdown", want: syntaxMarkdown},
		{path: "a.c", want: syntaxC},
		{path: "a.h", want: syntaxC},
		{path: "a.m", want: syntaxMiranda},
		{path: "a.txt", want: syntaxNone},
	}
	for _, tc := range tests {
		if got := detectSyntax(tc.path, tc.src); got != tc.want {
			t.Fatalf("detectSyntax(%q)=%v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestDetectSyntaxByContent(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want syntaxKind
	}{
		{name: "go package", src: "\n  package main\nfunc main(){}", want: syntaxGo},
		{name: "markdown heading", src: "## title\ntext", want: syntaxMarkdown},
		{name: "unknown", src: "plain text\nsecond line", want: syntaxNone},
	}
	for _, tc := range tests {
		if got := detectSyntax("", tc.src); got != tc.want {
			t.Fatalf("%s: detectSyntax=%v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestBufferLanguageMode(t *testing.T) {
	tests := []struct {
		path string
		src  string
		want string
	}{
		{path: "a.go", want: "go"},
		{path: "a.md", want: "markdown"},
		{path: "a.c", want: "c"},
		{path: "a.m", want: "miranda"},
		{path: "a.txt", want: "text"},
	}
	for _, tc := range tests {
		if got := syntaxKindLabel(detectSyntax(tc.path, tc.src)); got != tc.want {
			t.Fatalf("syntax kind label(%q)=%q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestSyntaxKindLabel(t *testing.T) {
	tests := []struct {
		kind syntaxKind
		want string
	}{
		{kind: syntaxNone, want: "text"},
		{kind: syntaxGo, want: "go"},
		{kind: syntaxMarkdown, want: "markdown"},
		{kind: syntaxC, want: "c"},
		{kind: syntaxMiranda, want: "miranda"},
	}
	for _, tc := range tests {
		if got := syntaxKindLabel(tc.kind); got != tc.want {
			t.Fatalf("syntaxKindLabel(%v)=%q, want %q", tc.kind, got, tc.want)
		}
	}
}

func TestBufferSyntaxKindUsesForcedMode(t *testing.T) {
	app := appState{}
	app.initBuffers(editor.NewEditor("plain text"))
	app.currentPath = "note.txt"
	app.buffers[0].path = "note.txt"

	if got := bufferSyntaxKind(&app, app.currentPath, app.ed.Runes()); got != syntaxNone {
		t.Fatalf("default syntax kind=%v, want text/none", got)
	}
	app.buffers[0].mode = syntaxGo
	if got := bufferSyntaxKind(&app, app.currentPath, app.ed.Runes()); got != syntaxGo {
		t.Fatalf("forced syntax kind=%v, want go", got)
	}
}

func TestSyntaxHighlighterLineStyleForLanguages(t *testing.T) {
	tests := []struct {
		name string
		path string
		src  string
	}{
		{name: "go", path: "main.go", src: "package main\nfunc main() { return }\n"},
		{name: "markdown", path: "notes.md", src: "# Header\n- item\n"},
		{name: "c", path: "main.c", src: "int main(void) { return 0; }\n"},
		{name: "miranda", path: "demo.m", src: "module Demo where\nx = 1\n"},
	}
	h := newGoHighlighter()
	for _, tc := range tests {
		lines := editor.SplitLines([]rune(tc.src))
		got := h.lineStyleForKind(tc.path, tc.src, lines, detectSyntax(tc.path, tc.src))
		if len(got) == 0 {
			t.Fatalf("%s: expected highlighted tokens, got none", tc.name)
		}
	}
}

func TestGoKeywordStyleIncludesFirstRune(t *testing.T) {
	src := "package main\nfunc main() {\n\tif true {\n\t\tfor i := 0; i < 1; i++ {}\n\t}\n}\n"
	lines := editor.SplitLines([]rune(src))
	h := newGoHighlighter()
	styles := h.lineStyleForKind("main.go", src, lines, syntaxGo)
	if len(styles) == 0 {
		t.Fatalf("expected non-empty styles")
	}

	lineIf := 2
	if strings.TrimSpace(lines[lineIf])[:2] != "if" {
		t.Fatalf("test setup: line %d does not start with if: %q", lineIf+1, lines[lineIf])
	}
	colIf := strings.Index(lines[lineIf], "if")
	if colIf < 0 || colIf >= len(styles[lineIf]) {
		t.Fatalf("if column out of range: col=%d len=%d", colIf, len(styles[lineIf]))
	}
	if styles[lineIf][colIf] != styleKeyword {
		t.Fatalf("if first rune style=%v, want %v", styles[lineIf][colIf], styleKeyword)
	}

	lineFor := 3
	colFor := strings.Index(lines[lineFor], "for")
	if colFor < 0 || colFor >= len(styles[lineFor]) {
		t.Fatalf("for column out of range: col=%d len=%d", colFor, len(styles[lineFor]))
	}
	if styles[lineFor][colFor] != styleKeyword {
		t.Fatalf("for first rune style=%v, want %v", styles[lineFor][colFor], styleKeyword)
	}
}

func TestIdentPrefixStart(t *testing.T) {
	buf := []rune("fmt.Prin")
	if got := identPrefixStart(buf, len(buf)); got != 4 {
		t.Fatalf("identPrefixStart=%d, want 4", got)
	}
	if got := identPrefixStart([]rune("abc"), 0); got != 0 {
		t.Fatalf("identPrefixStart at 0=%d, want 0", got)
	}
}

func TestStripSnippet(t *testing.T) {
	in := "Printf(${1:format}, $0)"
	got := stripSnippet(in)
	want := "Printf(format, )"
	if got != want {
		t.Fatalf("stripSnippet=%q, want %q", got, want)
	}
}

func TestParseCompletionItems(t *testing.T) {
	raw := json.RawMessage(`[{"label":"Printf","insertText":"Printf(${1:format})","insertTextFormat":2}]`)
	items := parseCompletionItems(raw)
	if len(items) != 1 {
		t.Fatalf("len(items)=%d, want 1", len(items))
	}
	if items[0].Insert != "Printf(format)" {
		t.Fatalf("insert=%q, want %q", items[0].Insert, "Printf(format)")
	}
}

func TestExtremelySureCompletion(t *testing.T) {
	item, ok := extremelySureCompletion("Prin", []completionItem{
		{Label: "Println", Insert: "Println"},
	}, 3)
	if !ok {
		t.Fatalf("expected high-confidence completion")
	}
	if item.Insert != "Println" {
		t.Fatalf("insert=%q, want %q", item.Insert, "Println")
	}

	if _, ok := extremelySureCompletion("Pr", []completionItem{{Label: "Println", Insert: "Println"}}, 3); ok {
		t.Fatalf("expected low confidence for short prefix")
	}
	if _, ok := extremelySureCompletion("Prin", []completionItem{{Label: "Println", Insert: "Println"}, {Label: "Printf", Insert: "Printf"}}, 3); ok {
		t.Fatalf("expected low confidence for multiple candidates")
	}
	item3, ok := extremelySureCompletion("Prin", []completionItem{{Label: "Println", Insert: "Println()"}}, 3)
	if !ok || item3.Insert != "Println" {
		t.Fatalf("expected label fallback for punctuation insert text, got ok=%v insert=%q", ok, item3.Insert)
	}
	item2, ok := extremelySureCompletion("pack", []completionItem{{Label: "package", Insert: "package ${1:name}"}}, 1)
	if !ok || item2.Insert != "package" {
		t.Fatalf("expected label fallback for snippet completion, got ok=%v insert=%q", ok, item2.Insert)
	}
}

func TestGoKeywordFallback(t *testing.T) {
	if got, ok := goKeywordFallback("pack"); !ok || got != "package" {
		t.Fatalf("goKeywordFallback(pack)=%q ok=%v, want package true", got, ok)
	}
	if _, ok := goKeywordFallback("r"); ok {
		t.Fatalf("goKeywordFallback(r) should be ambiguous")
	}
}

func TestCycleBufferModeAndForcedGoCompletion(t *testing.T) {
	app := appState{noGopls: true}
	app.initBuffers(editor.NewEditor("packa"))
	app.currentPath = "untitled"
	app.buffers[0].path = "untitled"
	app.ed.Caret = app.ed.RuneLen()

	if tryManualCompletion(&app) {
		t.Fatalf("completion should be off in text mode")
	}
	if mode := cycleBufferMode(&app); mode != "go" {
		t.Fatalf("mode=%q, want go", mode)
	}
	if !tryManualCompletion(&app) {
		t.Fatalf("completion should work in forced go mode")
	}
	if got := app.ed.String(); got != "package" {
		t.Fatalf("buf=%q, want package", got)
	}
}

func TestCycleBufferModeWrapsToText(t *testing.T) {
	app := appState{}
	app.initBuffers(editor.NewEditor("x"))
	order := []string{"go", "markdown", "c", "miranda", "text"}
	for _, want := range order {
		if got := cycleBufferMode(&app); got != want {
			t.Fatalf("cycle mode=%q, want %q", got, want)
		}
	}
}

func TestForcedGoCompletionKeywordFastPathWithoutGopls(t *testing.T) {
	app := appState{}
	app.initBuffers(editor.NewEditor("pack"))
	app.currentPath = "untitled"
	app.buffers[0].path = "untitled"
	app.buffers[0].mode = syntaxGo
	app.ed.Caret = app.ed.RuneLen()
	app.noGopls = false
	app.gopls = nil // would panic if tryManualCompletion reached gopls

	if !tryManualCompletion(&app) {
		t.Fatalf("expected keyword completion success")
	}
	if got := app.ed.String(); got != "package" {
		t.Fatalf("buf=%q, want package", got)
	}
}

func TestImportedPackageNameExpansion(t *testing.T) {
	src := "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfm\n}\n"
	app := appState{noGopls: true}
	app.initBuffers(editor.NewEditor(src))
	app.currentPath = "a.go"
	app.ed.Caret = strings.Index(app.ed.String(), "\tfm") + 3

	if !tryManualCompletion(&app) {
		t.Fatalf("expected import-name completion")
	}
	if !strings.Contains(app.ed.String(), "\tfmt\n") {
		t.Fatalf("expected fm -> fmt expansion, got %q", app.ed.String())
	}
}

func TestSelectorCompletionPopupAndApply(t *testing.T) {
	src := "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.\n}\n"
	app := appState{}
	app.initBuffers(editor.NewEditor(src))
	app.currentPath = "a.go"
	app.ed.Caret = strings.Index(app.ed.String(), "fmt.") + len("fmt.")

	oldComplete := completeGoCompletions
	defer func() { completeGoCompletions = oldComplete }()
	completeGoCompletions = func(_ *appState, _ string, _ string, _ int, _ int) ([]completionItem, error) {
		return []completionItem{
			{Label: "Print", Insert: "Print", Detail: "func Print(a ...any) (n int, err error)"},
			{Label: "Println", Insert: "Println", Detail: "func Println(a ...any) (n int, err error)"},
		}, nil
	}

	if !tryManualCompletion(&app) {
		t.Fatalf("expected selector completion popup")
	}
	if !app.completionPopup.active || len(app.completionPopup.items) != 2 {
		t.Fatalf("expected active popup with items, got active=%v len=%d", app.completionPopup.active, len(app.completionPopup.items))
	}
	completionPopupMove(&app, 1)
	if !completionPopupApplySelection(&app) {
		t.Fatalf("expected popup selection apply")
	}
	if !strings.Contains(app.ed.String(), "fmt.Println") {
		t.Fatalf("expected selected completion to apply, got %q", app.ed.String())
	}
}

func TestGoSyntaxCheckerLineErrors(t *testing.T) {
	c := newGoSyntaxChecker()

	noErr := c.lineErrorsFor("ok.go", []rune("package main\nfunc main() {}\n"))
	if len(noErr) != 0 {
		t.Fatalf("expected no syntax errors, got %v", noErr)
	}

	src := "package main\nfunc main() {\n"
	withErr := c.lineErrorsFor("bad.go", []rune(src))
	if len(withErr) == 0 {
		t.Fatalf("expected syntax error lines for incomplete Go source")
	}
	if _, ok := withErr[1]; !ok {
		t.Fatalf("expected line 2 marker, got %v", withErr)
	}

	nonGo := c.lineErrorsFor("notes.md", []rune("# h1\n"))
	if len(nonGo) != 0 {
		t.Fatalf("expected no syntax checking for non-Go buffers")
	}
}

func TestSymbolUnderCaret(t *testing.T) {
	buf := []rune("package main\nfmt.Println(x)\n")
	if got := symbolUnderCaret(buf, 2); got != "package" {
		t.Fatalf("symbolUnderCaret keyword=%q, want package", got)
	}
	pos := strings.Index(string(buf), "Println") + 2
	if got := symbolUnderCaret(buf, pos); got != "Println" {
		t.Fatalf("symbolUnderCaret function=%q, want Println", got)
	}
}

func TestShowSymbolInfoKeywordAndBuiltin(t *testing.T) {
	app := appState{noGopls: true}
	app.initBuffers(editor.NewEditor("package main\n"))
	app.currentPath = "a.go"
	app.ed.Caret = 2
	if got := showSymbolInfo(&app); !strings.Contains(got, "Go keyword: package") {
		t.Fatalf("keyword info mismatch: %q", got)
	} else if !strings.Contains(got, "Usage:\npackage main") {
		t.Fatalf("expected keyword usage example, got %q", got)
	}

	app2 := appState{noGopls: true}
	app2.initBuffers(editor.NewEditor("x := len(y)\n"))
	app2.currentPath = "b.go"
	app2.ed.Caret = strings.Index(app2.ed.String(), "len") + 1
	if got := showSymbolInfo(&app2); !strings.Contains(got, "Go builtin: len") {
		t.Fatalf("builtin info mismatch: %q", got)
	} else if !strings.Contains(got, "Usage:\nn := len(v)") {
		t.Fatalf("expected builtin usage example, got %q", got)
	}
}

func TestShowSymbolInfoFindsLocalDefinition(t *testing.T) {
	src := "package main\n\nfunc helper(a int) int { return a + 1 }\n\nfunc main() { _ = helper(1) }\n"
	app := appState{noGopls: true}
	app.initBuffers(editor.NewEditor(src))
	app.currentPath = "a.go"
	app.ed.Caret = strings.Index(src, "helper(1)") + 2
	got := showSymbolInfo(&app)
	if !strings.Contains(got, "Local definition") || !strings.Contains(got, "func helper(a int) int") {
		t.Fatalf("expected local definition info, got %q", got)
	}
}

func TestShowSymbolInfoPackageNameAndImportSelector(t *testing.T) {
	src := "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hi\")\n}\n"
	app := appState{noGopls: true}
	app.initBuffers(editor.NewEditor(src))
	app.currentPath = "a.go"

	app.ed.Caret = strings.Index(src, "main")
	gotPkg := showSymbolInfo(&app)
	if !strings.Contains(gotPkg, "Package declaration: main") {
		t.Fatalf("expected package declaration info, got %q", gotPkg)
	}

	app.ed.Caret = strings.Index(src, "Println") + 2
	gotSel := showSymbolInfo(&app)
	if !strings.Contains(gotSel, "Package member: fmt.Println") {
		t.Fatalf("expected imported member info, got %q", gotSel)
	}
	if !strings.Contains(gotSel, "Imported package: fmt (\"fmt\")") {
		t.Fatalf("expected import package info, got %q", gotSel)
	}
}

func TestShowSymbolInfoFindsShortVarDefinition(t *testing.T) {
	src := "package main\n\nfunc main() {\n\tvalue := 42\n\t_ = value\n}\n"
	app := appState{noGopls: true}
	app.initBuffers(editor.NewEditor(src))
	app.currentPath = "a.go"
	app.ed.Caret = strings.LastIndex(src, "value")
	got := showSymbolInfo(&app)
	if !strings.Contains(got, "Local definition") || !strings.Contains(got, "value := 42") {
		t.Fatalf("expected short var definition info, got %q", got)
	}
}

func TestShowSymbolInfoNonGoAndNoSymbol(t *testing.T) {
	app := appState{noGopls: true}
	app.initBuffers(editor.NewEditor("plain text"))
	app.currentPath = "note.txt"
	app.ed.Caret = 2
	if got := showSymbolInfo(&app); got != "Symbol info: Go mode only" {
		t.Fatalf("expected non-go message, got %q", got)
	}

	app2 := appState{noGopls: true}
	app2.initBuffers(editor.NewEditor("package main\n"))
	app2.currentPath = "a.go"
	app2.ed.Caret = app2.ed.RuneLen()
	if got := showSymbolInfo(&app2); got == "" {
		t.Fatalf("expected non-empty message")
	}
}

func TestWrapPopupTextAndSingleLine(t *testing.T) {
	lines := wrapPopupText("one two three four five six seven", 11)
	if len(lines) < 2 {
		t.Fatalf("expected wrapped lines, got %v", lines)
	}
	lines2 := wrapPopupText("Header line\n\nsecond paragraph content", 12)
	if len(lines2) < 3 || lines2[0] != "Header line" || lines2[1] != "" {
		t.Fatalf("expected newline-preserving wrap, got %v", lines2)
	}
	if got := singleLine("hello\nworld"); got != "hello world" {
		t.Fatalf("singleLine newline flatten failed: %q", got)
	}
}

func TestFormatHoverMarkdown(t *testing.T) {
	in := "# Signature\n\n- `Println` writes output\nSee [fmt](https://pkg.go.dev/fmt).\n\n```go\nfmt.Println(x)\n```"
	got := formatHoverMarkdown(in)
	if !strings.Contains(got, "SIGNATURE") {
		t.Fatalf("expected heading, got %q", got)
	}
	if !strings.Contains(got, "• \"Println\" writes output") {
		t.Fatalf("expected bullet + inline code formatting, got %q", got)
	}
	if !strings.Contains(got, "fmt (https://pkg.go.dev/fmt)") {
		t.Fatalf("expected link formatting, got %q", got)
	}
	if !strings.Contains(got, "Code (go):") || !strings.Contains(got, "    fmt.Println(x)") {
		t.Fatalf("expected fenced code formatting, got %q", got)
	}
}

func TestParseHoverText(t *testing.T) {
	raw1 := json.RawMessage(`{"contents":"abc"}`)
	if got := parseHoverText(raw1); got != "abc" {
		t.Fatalf("parseHoverText string=%q", got)
	}
	raw2 := json.RawMessage(`{"contents":{"kind":"markdown","value":"**x**"}}`)
	if got := parseHoverText(raw2); got != "**x**" {
		t.Fatalf("parseHoverText markup=%q", got)
	}
	raw3 := json.RawMessage(`{"contents":[{"kind":"markdown","value":"a"},"b"]}`)
	if got := parseHoverText(raw3); got != "a\nb" {
		t.Fatalf("parseHoverText array=%q", got)
	}
}

func TestParseLineFromErr(t *testing.T) {
	if ln, ok := parseLineFromErr("bad.go:4:2: expected ';'"); !ok || ln != 3 {
		t.Fatalf("parseLineFromErr line parse mismatch: ln=%d ok=%v", ln, ok)
	}
	if _, ok := parseLineFromErr("nonsense"); ok {
		t.Fatalf("parseLineFromErr should reject malformed messages")
	}
	if _, ok := parseLineFromErr("bad.go:x:2: expected"); ok {
		t.Fatalf("parseLineFromErr should reject non-numeric line numbers")
	}
}

func TestAppendRunOutput(t *testing.T) {
	ed := editor.NewEditor("abc")
	ed.Caret = 0
	appendRunOutput(ed, "xyz")
	if got := ed.String(); got != "abcxyz" {
		t.Fatalf("appendRunOutput buf=%q, want %q", got, "abcxyz")
	}
	if ed.Caret != ed.RuneLen() {
		t.Fatalf("appendRunOutput caret=%d, want %d", ed.Caret, ed.RuneLen())
	}
	appendRunOutput(nil, "noop")
	appendRunOutput(ed, "")
}

func TestRunCurrentPackageNilApp(t *testing.T) {
	if err := runCurrentPackage(nil); err == nil {
		t.Fatalf("runCurrentPackage(nil) should fail")
	}
}

func TestRunCurrentPackageOpensBufferAndStreamsOutput(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "p.go")
	app := appState{}
	app.initBuffers(editor.NewEditor("package main\n"))
	app.currentPath = path
	app.buffers[0].path = path

	oldRun := startGoRun
	defer func() { startGoRun = oldRun }()
	startGoRun = func(runDir string, onOut func(string), onDone func(error)) error {
		if runDir != dir {
			t.Fatalf("runDir=%q, want %q", runDir, dir)
		}
		onOut("hello\n")
		onDone(errors.New("boom"))
		return nil
	}

	if err := runCurrentPackage(&app); err != nil {
		t.Fatalf("runCurrentPackage err: %v", err)
	}
	if len(app.buffers) != 2 {
		t.Fatalf("expected run buffer to be added, got %d buffers", len(app.buffers))
	}
	got := app.ed.String()
	if !strings.Contains(got, "$ (cd "+dir+" && go run .)") {
		t.Fatalf("run buffer missing command header: %q", got)
	}
	if !strings.Contains(got, "hello\n") {
		t.Fatalf("run buffer missing streamed output: %q", got)
	}
	if !strings.Contains(got, "[exit] boom") {
		t.Fatalf("run buffer missing error exit footer: %q", got)
	}
}

func TestRunCurrentPackageUsesCwdFallback(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	app := appState{}
	app.initBuffers(editor.NewEditor("package main\n"))
	app.currentPath = ""
	app.openRoot = ""

	oldRun := startGoRun
	defer func() { startGoRun = oldRun }()
	startGoRun = func(runDir string, onOut func(string), onDone func(error)) error {
		if runDir != cwd {
			t.Fatalf("runDir=%q, want cwd %q", runDir, cwd)
		}
		if onDone != nil {
			onDone(nil)
		}
		return nil
	}

	if err := runCurrentPackage(&app); err != nil {
		t.Fatalf("runCurrentPackage err: %v", err)
	}
	if !strings.Contains(app.ed.String(), "[exit] ok") {
		t.Fatalf("run buffer should include ok footer, got %q", app.ed.String())
	}
}
