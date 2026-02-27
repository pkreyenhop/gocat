package main

import (
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
