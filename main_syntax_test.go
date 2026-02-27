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
		if got := bufferLanguageMode(tc.path, []rune(tc.src)); got != tc.want {
			t.Fatalf("bufferLanguageMode(%q)=%q, want %q", tc.path, got, tc.want)
		}
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
		got := h.lineStyleFor(tc.path, []rune(tc.src), lines)
		if len(got) == 0 {
			t.Fatalf("%s: expected highlighted tokens, got none", tc.name)
		}
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
	if got := showSymbolInfo(&app); !strings.Contains(got, "Go keyword package") {
		t.Fatalf("keyword info mismatch: %q", got)
	}

	app2 := appState{noGopls: true}
	app2.initBuffers(editor.NewEditor("x := len(y)\n"))
	app2.currentPath = "b.go"
	app2.ed.Caret = strings.Index(string(app2.ed.Buf), "len") + 1
	if got := showSymbolInfo(&app2); !strings.Contains(got, "Go builtin len") {
		t.Fatalf("builtin info mismatch: %q", got)
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
	app2.ed.Caret = len(app2.ed.Buf)
	if got := showSymbolInfo(&app2); got == "" {
		t.Fatalf("expected non-empty message")
	}
}

func TestWrapPopupTextAndSingleLine(t *testing.T) {
	lines := wrapPopupText("one two three four five six seven", 11)
	if len(lines) < 2 {
		t.Fatalf("expected wrapped lines, got %v", lines)
	}
	if got := singleLine("hello\nworld"); got != "hello world" {
		t.Fatalf("singleLine newline flatten failed: %q", got)
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
	if got := string(ed.Buf); got != "abcxyz" {
		t.Fatalf("appendRunOutput buf=%q, want %q", got, "abcxyz")
	}
	if ed.Caret != len(ed.Buf) {
		t.Fatalf("appendRunOutput caret=%d, want %d", ed.Caret, len(ed.Buf))
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
	got := string(app.ed.Buf)
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
	if !strings.Contains(string(app.ed.Buf), "[exit] ok") {
		t.Fatalf("run buffer should include ok footer, got %q", string(app.ed.Buf))
	}
}
