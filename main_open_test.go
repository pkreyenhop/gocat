package main

import (
	"os"
	"path/filepath"
	"testing"

	"sdl-alt-test/editor"
)

func TestFindMatchesAndOpenPath(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "alpha.txt")
	if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, ".git"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git", "ignored.txt"), []byte("ignored"), 0644); err != nil {
		t.Fatalf("write ignored: %v", err)
	}

	matches := findMatches(root, "alp", 10)
	if len(matches) != 1 || matches[0] != path {
		t.Fatalf("matches = %v, want [%s]", matches, path)
	}

	app := &appState{ed: editor.NewEditor(""), openRoot: root}
	if err := openPath(app, matches[0]); err != nil {
		t.Fatalf("openPath: %v", err)
	}
	if string(app.ed.Buf) != "hello" {
		t.Fatalf("buffer: want %q, got %q", "hello", string(app.ed.Buf))
	}
	if app.currentPath != path {
		t.Fatalf("currentPath: want %s, got %s", path, app.currentPath)
	}
}
