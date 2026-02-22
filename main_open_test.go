package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gc/editor"
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

	app := &appState{openRoot: root}
	app.initBuffers(editor.NewEditor(""))
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

func TestFilePickerListsAndLoads(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "a.txt")
	b := filepath.Join(root, "b.txt")
	if err := os.WriteFile(a, []byte("aaa"), 0644); err != nil {
		t.Fatalf("write a: %v", err)
	}
	if err := os.WriteFile(b, []byte("bbb"), 0644); err != nil {
		t.Fatalf("write b: %v", err)
	}

	files, err := listFiles(root, 10)
	if err != nil {
		t.Fatalf("listFiles: %v", err)
	}
	if len(files) != 2 || files[0] != "a.txt" || files[1] != "b.txt" {
		t.Fatalf("files: got %v", files)
	}

	app := &appState{openRoot: root}
	app.initBuffers(editor.NewEditor(""))
	app.addPickerBuffer(files)

	// Move caret to second line (b.txt)
	app.ed.Caret = len([]rune(files[0])) + 1

	if err := loadFileAtCaret(app); err != nil {
		t.Fatalf("loadFileAtCaret: %v", err)
	}
	if string(app.ed.Buf) != "bbb" {
		t.Fatalf("buffer after load: want %q, got %q", "bbb", string(app.ed.Buf))
	}
	if app.currentPath != b {
		t.Fatalf("currentPath: want %s, got %s", b, app.currentPath)
	}
	if app.buffers[app.bufIdx].picker {
		t.Fatalf("picker flag should be cleared after load")
	}
}

func TestOpenPathRejectsOutsideRoot(t *testing.T) {
	root := t.TempDir()
	app := &appState{openRoot: root}
	app.initBuffers(editor.NewEditor(""))

	err := openPath(app, "/tmp/forbidden.txt")
	if err == nil {
		t.Fatalf("expected openPath to reject path outside root")
	}
}

func TestSaveCurrentDefaultsToLeapTxt(t *testing.T) {
	root := t.TempDir()
	app := &appState{openRoot: root}
	app.initBuffers(editor.NewEditor("hi"))

	app.currentPath = filepath.Join(root, defaultPath(app))
	app.buffers[0].path = app.currentPath
	app.buffers[0].dirty = true
	if err := saveCurrent(app); err != nil {
		t.Fatalf("saveCurrent: %v", err)
	}
	if app.currentPath == "" {
		t.Fatalf("currentPath should be set after save")
	}
	data, err := os.ReadFile(app.currentPath)
	if err != nil {
		t.Fatalf("read saved file: %v", err)
	}
	if string(data) != "hi" {
		t.Fatalf("saved contents mismatch: %q", string(data))
	}
}

func TestFilterArgsToFilesSkipsDirs(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "a.txt")
	dir := filepath.Join(root, "dir")
	if err := os.WriteFile(file, []byte("x"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	got := filterArgsToFiles([]string{file, dir})
	if len(got) != 1 || got[0] != file {
		t.Fatalf("filterArgsToFiles got %v", got)
	}
}

func TestLoadStartupFilesCreatesMissing(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "newfile.txt")

	app := &appState{openRoot: root}
	app.initBuffers(editor.NewEditor(""))

	loadStartupFiles(app, []string{target})

	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected file to not exist until saved, err=%v", err)
	}
	if string(app.ed.Buf) != "" {
		t.Fatalf("new buffer should be empty, got %q", string(app.ed.Buf))
	}
	if app.currentPath != target {
		t.Fatalf("currentPath mismatch: %s", app.currentPath)
	}

	// Saving should create the file.
	if err := saveCurrent(app); err != nil {
		t.Fatalf("saveCurrent: %v", err)
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != "" {
		t.Fatalf("expected empty file after save, got %q err=%v", string(data), err)
	}
}

func TestOpenLongFileAndLeapAround(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "long.txt")
	var sb strings.Builder
	for i := range 35000 {
		sb.WriteString("line ")
		sb.WriteString(fmt.Sprintf("%d", i))
		sb.WriteString("\n")
	}
	content := sb.String()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write long file: %v", err)
	}

	app := &appState{openRoot: root}
	app.initBuffers(editor.NewEditor(""))
	if err := openPath(app, path); err != nil {
		t.Fatalf("openPath: %v", err)
	}

	// Leap to end using the last line marker
	app.ed.Leap.LastCommit = []rune("line 34999")
	app.ed.LeapAgain(editor.DirFwd)
	if app.ed.Caret <= len(app.ed.Buf)/2 {
		t.Fatalf("expected caret near end after LeapAgain forward: %d", app.ed.Caret)
	}

	// Then leap back to the start via a new query
	app.ed.Leap.LastCommit = []rune("line 0")
	app.ed.Caret = len(app.ed.Buf)
	app.ed.LeapAgain(editor.DirFwd) // wrap to first occurrence
	if app.ed.Caret != 0 {
		t.Fatalf("expected caret at start after wrap; got %d", app.ed.Caret)
	}
}
