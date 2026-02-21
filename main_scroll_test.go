package main

import "testing"

func TestEnsureCaretVisibleScrollsDown(t *testing.T) {
	var app appState

	ensureCaretVisible(&app, 0, 50, 5)
	if app.scrollLine != 0 {
		t.Fatalf("initial scroll should stay at 0, got %d", app.scrollLine)
	}

	ensureCaretVisible(&app, 6, 50, 5)
	if want := 2; app.scrollLine != want {
		t.Fatalf("scroll after moving past view: want %d, got %d", want, app.scrollLine)
	}
}

func TestEnsureCaretVisibleScrollsUp(t *testing.T) {
	app := appState{scrollLine: 10}

	ensureCaretVisible(&app, 4, 50, 5)
	if want := 4; app.scrollLine != want {
		t.Fatalf("scroll when caret above view: want %d, got %d", want, app.scrollLine)
	}
}

func TestEnsureCaretVisibleClampsEnd(t *testing.T) {
	var app appState

	ensureCaretVisible(&app, 49, 50, 10)
	if want := 40; app.scrollLine != want {
		t.Fatalf("scroll near end: want %d, got %d", want, app.scrollLine)
	}
}

func TestEnsureCaretVisibleHandlesShortBuffers(t *testing.T) {
	app := appState{scrollLine: 3}

	ensureCaretVisible(&app, 0, 3, 10)
	if app.scrollLine != 0 {
		t.Fatalf("short buffer should reset scroll, got %d", app.scrollLine)
	}
}

func TestEnsureCaretVisibleKeepsCaretWithinWindow(t *testing.T) {
	app := appState{scrollLine: 5}

	ensureCaretVisible(&app, 7, 50, 5)
	if want := 5; app.scrollLine != want {
		t.Fatalf("caret already visible should not scroll, want %d got %d", want, app.scrollLine)
	}
}

func TestEnsureCaretVisibleClampsNegativeCaret(t *testing.T) {
	var app appState

	ensureCaretVisible(&app, -3, 20, 5)
	if app.scrollLine != 0 {
		t.Fatalf("negative caret should clamp to 0 scroll, got %d", app.scrollLine)
	}
}

func TestEnsureCaretVisibleZeroVisibleLines(t *testing.T) {
	var app appState

	ensureCaretVisible(&app, 3, 10, 0)
	if want := 3; app.scrollLine != want {
		t.Fatalf("zero visible lines should treat as 1 and follow caret, want %d got %d", want, app.scrollLine)
	}
}

func TestEnsureCaretVisibleCaretBeyondEnd(t *testing.T) {
	var app appState

	ensureCaretVisible(&app, 200, 50, 5)
	if want := 45; app.scrollLine != want {
		t.Fatalf("caret beyond end should clamp to max start, want %d got %d", want, app.scrollLine)
	}
}
