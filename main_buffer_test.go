package main

import (
	"testing"

	"sdl-alt-test/editor"
)

func TestAddAndSwitchBuffers(t *testing.T) {
	first := editor.NewEditor("one")
	app := appState{}
	app.initBuffers(first)
	app.currentPath = "one.txt"
	app.buffers[0].path = "one.txt"

	app.addBuffer()
	if len(app.buffers) != 2 {
		t.Fatalf("buffers len: want 2, got %d", len(app.buffers))
	}
	if app.bufIdx != 1 {
		t.Fatalf("bufIdx after add: want 1, got %d", app.bufIdx)
	}
	if app.ed == nil || app.ed == first {
		t.Fatalf("expected a new active editor after add")
	}
	if app.currentPath != "" {
		t.Fatalf("new buffer should start untitled, got %q", app.currentPath)
	}

	app.currentPath = "two.txt"
	app.buffers[app.bufIdx].path = "two.txt"

	app.switchBuffer(1) // wrap to first
	if app.bufIdx != 0 || app.ed != first {
		t.Fatalf("switchBuffer should wrap to first buffer; idx=%d", app.bufIdx)
	}
	if app.currentPath != "one.txt" {
		t.Fatalf("currentPath after switching back: want one.txt, got %q", app.currentPath)
	}

	app.switchBuffer(1)
	if app.bufIdx != 1 {
		t.Fatalf("switchBuffer should move to second buffer; idx=%d", app.bufIdx)
	}
	if app.currentPath != "two.txt" {
		t.Fatalf("currentPath after switching to second: want two.txt, got %q", app.currentPath)
	}
}

func TestCloseBufferCountsAndSwitches(t *testing.T) {
	first := editor.NewEditor("first")
	second := editor.NewEditor("second")
	app := appState{}
	app.initBuffers(first)
	app.buffers[0].path = "one.txt"
	app.addBuffer()
	app.buffers[1].ed = second
	app.buffers[1].path = "two.txt"

	if remaining := app.closeBuffer(); remaining != 1 {
		t.Fatalf("after closing second buffer, remaining=%d want 1", remaining)
	}
	if app.ed != first || app.currentPath != "one.txt" {
		t.Fatalf("should switch back to first buffer; path=%q", app.currentPath)
	}

	if remaining := app.closeBuffer(); remaining != 0 {
		t.Fatalf("closing last buffer should leave 0 remaining, got %d", remaining)
	}
	if app.ed != nil {
		t.Fatalf("expected no active editor after closing all buffers")
	}
}
