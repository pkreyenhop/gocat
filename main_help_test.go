package main

import (
	"os"
	"strings"
	"testing"
)

// Ensure help entries stay in sync with README so the in-app overlay and docs don't drift.
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
