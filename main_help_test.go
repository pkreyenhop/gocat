package main

import (
	"os"
	"strings"
	"testing"
)

// Ensure help entries stay in sync with README so docs and help text do not drift.
func TestHelpEntriesPresentInREADME(t *testing.T) {
	data, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatalf("read README: %v", err)
	}
	content := string(data)
	for _, h := range helpEntries {
		if !containsAll(content, h.action, h.keys) {
			t.Fatalf("README missing help entry: %q / %q", h.action, h.keys)
		}
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !strings.Contains(s, p) {
			return false
		}
	}
	return true
}

func TestCoreBehaviorsDocumentedInAllMarkdownGuides(t *testing.T) {
	readme := readMarkdown(t, "README.md")
	manual := readMarkdown(t, "MANUAL.md")
	rules := readMarkdown(t, "RULES.md")

	required := []string{
		"Esc+Space",
		"Esc+M",
		"Space",
		"Esc",
		"Esc+i",
		"Shift+Tab",
		"Ctrl+,",
		"Ctrl+.",
		"lang=",
		"Tab",
		"go fmt",
		"go fix",
		"go run .",
	}
	for _, token := range required {
		if !strings.Contains(readme, token) {
			t.Fatalf("README.md missing %q", token)
		}
		if !strings.Contains(manual, token) {
			t.Fatalf("MANUAL.md missing %q", token)
		}
		if !strings.Contains(rules, token) {
			t.Fatalf("RULES.md missing %q", token)
		}
	}
}

func readMarkdown(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(data)
}
