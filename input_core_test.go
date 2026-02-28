package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gc/editor"
)

func TestHandleKeyEventCtrlBAddsBufferWithoutSDLDispatch(t *testing.T) {
	app := appState{}
	app.initBuffers(editor.NewEditor("abc"))
	if !handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keyB, mods: modCtrl}) {
		t.Fatalf("handleKeyEvent should continue running")
	}
	if len(app.buffers) != 2 {
		t.Fatalf("expected buffer count 2, got %d", len(app.buffers))
	}
}

func TestHandleTextEventInsertsTextWithoutSDLDispatch(t *testing.T) {
	app := appState{}
	app.initBuffers(editor.NewEditor("ab"))
	app.ed.Caret = len(app.ed.Buf)
	if !handleTextEvent(&app, "c", 0) {
		t.Fatalf("handleTextEvent should continue running")
	}
	if got := string(app.ed.Buf); got != "abc" {
		t.Fatalf("text insert mismatch: got %q", got)
	}
}

func TestEscPrefixInvokesCommandAndSuppressesTextInput(t *testing.T) {
	app := appState{}
	app.initBuffers(editor.NewEditor("abc"))
	if !handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keyEscape, mods: 0}) {
		t.Fatalf("esc prefix should continue running")
	}
	if !app.cmdPrefixActive {
		t.Fatalf("esc prefix should arm command mode")
	}
	if !handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keyB, mods: 0}) {
		t.Fatalf("prefixed command should continue running")
	}
	if got := len(app.buffers); got != 2 {
		t.Fatalf("ctrl-prefix b should create buffer, got %d", got)
	}
	// The text event that may follow the key event should be ignored once.
	if !handleTextEvent(&app, "b", 0) {
		t.Fatalf("suppressed text should continue running")
	}
	if got := string(app.ed.Buf); got != "" {
		t.Fatalf("suppressed text should not be inserted, got %q", got)
	}
}

func TestEscEscClosesLastBuffer(t *testing.T) {
	app := appState{}
	app.initBuffers(editor.NewEditor("abc"))
	if !handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keyEscape, mods: 0}) {
		t.Fatalf("first esc should arm prefix")
	}
	if handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keyEscape, mods: 0}) {
		t.Fatalf("esc+esc should close last buffer and quit")
	}
}

func TestEscShiftQClosesAllBuffers(t *testing.T) {
	app := appState{}
	app.initBuffers(editor.NewEditor("one"))
	app.addBuffer()
	if len(app.buffers) != 2 {
		t.Fatalf("expected 2 buffers, got %d", len(app.buffers))
	}
	if !handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keyEscape, mods: 0}) {
		t.Fatalf("first esc should arm prefix")
	}
	if handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keyQ, mods: modShift}) {
		t.Fatalf("esc+shift+q should quit/close all buffers")
	}
}

func TestEscSpaceLessModePagesAndEscExits(t *testing.T) {
	var txt strings.Builder
	for range 200 {
		txt.WriteString("line\n")
	}
	app := appState{}
	app.initBuffers(editor.NewEditor(txt.String()))
	app.ed.Caret = 0

	if !handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keyEscape, mods: 0}) {
		t.Fatalf("first esc should arm prefix")
	}
	if !handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keySpace, mods: 0}) {
		t.Fatalf("esc+space should enter less mode")
	}
	if !app.lessMode {
		t.Fatalf("less mode should be active")
	}
	before := app.ed.Caret
	if !handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keySpace, mods: 0}) {
		t.Fatalf("space should page in less mode")
	}
	if app.ed.Caret <= before {
		t.Fatalf("less mode paging should advance caret: before=%d after=%d", before, app.ed.Caret)
	}
	if !handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keySpace, mods: 0}) {
		t.Fatalf("second space should page again")
	}
	if !app.lessMode {
		t.Fatalf("less mode should stay active across spaces")
	}
	if !handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keyEscape, mods: 0}) {
		t.Fatalf("esc should exit less mode")
	}
	if app.lessMode {
		t.Fatalf("less mode should be off after esc")
	}
}

func TestEscShiftSSavesDirtyBuffers(t *testing.T) {
	root := t.TempDir()
	one := filepath.Join(root, "one.txt")
	two := filepath.Join(root, "two.txt")
	if err := os.WriteFile(one, []byte("ONE"), 0644); err != nil {
		t.Fatalf("write one: %v", err)
	}
	if err := os.WriteFile(two, []byte("TWO"), 0644); err != nil {
		t.Fatalf("write two: %v", err)
	}

	app := appState{openRoot: root}
	app.initBuffers(editor.NewEditor("dirty"))
	app.currentPath = one
	app.buffers[0].path = one
	app.buffers[0].dirty = true
	app.addBuffer()
	app.buffers[1].path = two
	app.buffers[1].ed.Buf = []rune("clean")
	app.buffers[1].dirty = false
	app.bufIdx = 0
	app.syncActiveBuffer()

	if !handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keyEscape, mods: 0}) {
		t.Fatalf("first esc should arm prefix")
	}
	if !handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keyS, mods: modShift}) {
		t.Fatalf("esc+shift+s should continue running")
	}

	data1, _ := os.ReadFile(one)
	if string(data1) != "dirty" {
		t.Fatalf("dirty buffer should be saved, got %q", string(data1))
	}
	data2, _ := os.ReadFile(two)
	if string(data2) != "TWO" {
		t.Fatalf("clean buffer should be untouched, got %q", string(data2))
	}
}

func TestEscMCyclesBufferMode(t *testing.T) {
	app := appState{}
	app.initBuffers(editor.NewEditor("x := 1\n"))
	if app.buffers[0].mode != syntaxNone {
		t.Fatalf("initial mode should be text/auto")
	}

	if !handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keyEscape}) {
		t.Fatalf("esc should arm prefix")
	}
	if !handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keyM}) {
		t.Fatalf("esc+m should continue")
	}
	if app.buffers[0].mode != syntaxGo {
		t.Fatalf("mode after first cycle=%v, want %v", app.buffers[0].mode, syntaxGo)
	}

	if !handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keyEscape}) {
		t.Fatalf("esc should arm prefix")
	}
	if !handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keyM}) {
		t.Fatalf("esc+m should continue")
	}
	if app.buffers[0].mode != syntaxMarkdown {
		t.Fatalf("mode after second cycle=%v, want %v", app.buffers[0].mode, syntaxMarkdown)
	}
}

func BenchmarkHandleKeyEventMoveRight(b *testing.B) {
	app := appState{}
	app.initBuffers(editor.NewEditor("package main\nfunc main() {}\n"))
	ev := keyEvent{down: true, key: keyRight, mods: 0}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = handleKeyEvent(&app, ev)
	}
}
