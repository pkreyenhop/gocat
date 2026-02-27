package main

import (
	"encoding/json"
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
	})
	if !ok {
		t.Fatalf("expected high-confidence completion")
	}
	if item.Insert != "Println" {
		t.Fatalf("insert=%q, want %q", item.Insert, "Println")
	}

	if _, ok := extremelySureCompletion("Pr", []completionItem{{Label: "Println", Insert: "Println"}}); ok {
		t.Fatalf("expected low confidence for short prefix")
	}
	if _, ok := extremelySureCompletion("Prin", []completionItem{{Label: "Println", Insert: "Println"}, {Label: "Printf", Insert: "Printf"}}); ok {
		t.Fatalf("expected low confidence for multiple candidates")
	}
	if _, ok := extremelySureCompletion("Prin", []completionItem{{Label: "Println", Insert: "Println()"}}); ok {
		t.Fatalf("expected low confidence for punctuation insert text")
	}
}
