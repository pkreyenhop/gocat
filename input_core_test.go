package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gc/editor"
)

func TestHandleKeyEventCtrlBAddsBufferWithoutFrontendDispatch(t *testing.T) {
	app := appState{}
	app.initBuffers(editor.NewEditor("abc"))
	if !handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keyB, mods: modCtrl}) {
		t.Fatalf("handleKeyEvent should continue running")
	}
	if len(app.buffers) != 2 {
		t.Fatalf("expected buffer count 2, got %d", len(app.buffers))
	}
}

func TestHandleTextEventInsertsTextWithoutFrontendDispatch(t *testing.T) {
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

func TestCtrlShortcutsReplacedByEscPrefix(t *testing.T) {
	app := appState{}
	app.initBuffers(editor.NewEditor("package main\nfunc main(){}\n"))
	app.currentPath = "p.go"
	app.buffers[0].path = "p.go"

	if !handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keyI, mods: modCtrl}) {
		t.Fatalf("ctrl+i should continue")
	}
	if app.symbolInfoPopup != "" {
		t.Fatalf("ctrl+i should no longer open symbol info")
	}

	app.addBuffer()
	if !handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keyQ, mods: modCtrl | modShift}) {
		t.Fatalf("ctrl+shift+q should not quit now")
	}

	if !handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keyEscape, mods: 0}) {
		t.Fatalf("esc should arm prefix")
	}
	if handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keyQ, mods: modShift}) {
		t.Fatalf("esc+shift+q should still quit")
	}
}

func TestEscShiftDeleteClearsWholeBuffer(t *testing.T) {
	app := appState{}
	app.initBuffers(editor.NewEditor("one\ntwo\nthree\n"))
	app.buffers[0].dirty = false

	if !handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keyEscape}) {
		t.Fatalf("esc should arm prefix")
	}
	if !handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keyDelete, mods: modShift}) {
		t.Fatalf("esc+shift+delete should continue")
	}

	if got := string(app.ed.Buf); got != "" {
		t.Fatalf("buffer should be cleared, got %q", got)
	}
	if app.ed.Caret != 0 {
		t.Fatalf("caret should reset to 0, got %d", app.ed.Caret)
	}
	if !app.buffers[0].dirty {
		t.Fatalf("clear should mark buffer dirty")
	}
}

func TestEscPrefixCtrlCommandDoesNotDropNextText(t *testing.T) {
	app := appState{}
	app.initBuffers(editor.NewEditor(""))

	if !handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keyEscape}) {
		t.Fatalf("esc should arm prefix")
	}
	if !handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keyB, mods: modCtrl}) {
		t.Fatalf("prefixed ctrl+b should continue")
	}
	if len(app.buffers) != 2 {
		t.Fatalf("expected new buffer from prefixed ctrl+b, got %d", len(app.buffers))
	}
	if !handleTextEvent(&app, "a", 0) {
		t.Fatalf("text event should continue")
	}
	if got := string(app.ed.Buf); got != "a" {
		t.Fatalf("first typed character should be preserved, got %q", got)
	}
}

func TestShiftUpDownExtendsLineSelectionForDelete(t *testing.T) {
	app := appState{}
	app.initBuffers(editor.NewEditor("l1\nl2\nl3\nl4\n"))
	app.ed.Caret = 1 // inside line 1

	if !handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keyDown, mods: modShift}) {
		t.Fatalf("shift+down should continue")
	}
	if !app.ed.Sel.Active {
		t.Fatalf("selection should be active after shift+down")
	}
	a, b := app.ed.Sel.Normalised()
	if a != 0 || b != 6 {
		t.Fatalf("selection after first shift+down = (%d,%d), want (0,6)", a, b)
	}

	if !handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keyDown, mods: modShift}) {
		t.Fatalf("second shift+down should continue")
	}
	a, b = app.ed.Sel.Normalised()
	if a != 0 || b != 9 {
		t.Fatalf("selection after second shift+down = (%d,%d), want (0,9)", a, b)
	}

	if !handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keyBackspace, mods: 0}) {
		t.Fatalf("backspace should continue")
	}
	if got := string(app.ed.Buf); got != "l4\n" {
		t.Fatalf("deleting line selection failed, got %q", got)
	}
}

func TestShiftUpContractsLineSelectionByLine(t *testing.T) {
	app := appState{}
	app.initBuffers(editor.NewEditor("l1\nl2\nl3\nl4\n"))
	app.ed.Caret = 1

	_ = handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keyDown, mods: modShift})
	_ = handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keyDown, mods: modShift})
	if !handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keyUp, mods: modShift}) {
		t.Fatalf("shift+up should continue")
	}
	a, b := app.ed.Sel.Normalised()
	if a != 0 || b != 6 {
		t.Fatalf("selection after contraction = (%d,%d), want (0,6)", a, b)
	}
}

func TestEscXStartsLineHighlightModeAndXExtends(t *testing.T) {
	app := appState{}
	app.initBuffers(editor.NewEditor("l1\nl2\nl3\n"))
	app.ed.Caret = 4 // on line 2

	if !handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keyEscape}) {
		t.Fatalf("esc should arm prefix")
	}
	if !handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keyX}) {
		t.Fatalf("esc+x should start line highlight mode")
	}
	if !app.lineHighlightMode {
		t.Fatalf("line highlight mode should be active")
	}
	a, b := app.ed.Sel.Normalised()
	if a != 3 || b != 6 {
		t.Fatalf("selection after esc+x = (%d,%d), want (3,6)", a, b)
	}

	if !handleTextEvent(&app, "x", 0) {
		t.Fatalf("x in line highlight mode should extend selection")
	}
	a, b = app.ed.Sel.Normalised()
	if a != 3 || b != 9 {
		t.Fatalf("selection after x = (%d,%d), want (3,9)", a, b)
	}

	if !handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keyBackspace}) {
		t.Fatalf("backspace should delete highlighted lines")
	}
	if got := string(app.ed.Buf); got != "l1\n" {
		t.Fatalf("buffer after delete = %q, want %q", got, "l1\n")
	}
}

func TestEscSlashSearchModeLiveAndTabWrap(t *testing.T) {
	app := appState{}
	app.initBuffers(editor.NewEditor("zero hello one hello two"))
	app.ed.Caret = 0

	if !handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keyEscape}) {
		t.Fatalf("esc should arm prefix")
	}
	if !handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keySlash}) {
		t.Fatalf("esc+/ should start search mode")
	}
	if !app.searchActive {
		t.Fatalf("search mode should be active")
	}

	if !handleTextEvent(&app, "h", 0) || !handleTextEvent(&app, "e", 0) || !handleTextEvent(&app, "l", 0) {
		t.Fatalf("typing search query should continue")
	}
	if !app.searchActive {
		t.Fatalf("search should stay active while entering pattern")
	}
	if app.ed.Caret != 5 {
		t.Fatalf("caret after incremental search = %d, want 5", app.ed.Caret)
	}
	a, b := app.ed.Sel.Normalised()
	if a != 5 || b != 8 {
		t.Fatalf("selection after incremental search = (%d,%d), want (5,8)", a, b)
	}
	if !handleTextEvent(&app, "/", 0) {
		t.Fatalf("slash should lock search pattern")
	}
	if !app.searchPatternDone {
		t.Fatalf("slash should finalize pattern entry")
	}
	if got := string(app.ed.Buf); got != "zero hello one hello two" {
		t.Fatalf("search typing should not edit buffer, got %q", got)
	}

	if !handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keyTab}) {
		t.Fatalf("tab in search mode should continue")
	}
	if app.ed.Caret != 15 {
		t.Fatalf("caret after tab next = %d, want 15", app.ed.Caret)
	}
	a, b = app.ed.Sel.Normalised()
	if a != 15 || b != 18 {
		t.Fatalf("selection after tab next = (%d,%d), want (15,18)", a, b)
	}
	if !handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keyTab}) {
		t.Fatalf("tab wrap in search mode should continue")
	}
	if app.ed.Caret != 5 {
		t.Fatalf("caret after tab wrap = %d, want 5", app.ed.Caret)
	}
	if !handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keyTab, mods: modShift}) {
		t.Fatalf("shift+tab in search mode should continue")
	}
	if app.ed.Caret != 15 {
		t.Fatalf("caret after shift+tab previous = %d, want 15", app.ed.Caret)
	}
}

func TestSearchFinalizeWithSlashThenAnyOtherRuneExitsSearchAndInserts(t *testing.T) {
	app := appState{}
	app.initBuffers(editor.NewEditor("zero hello one hello two"))
	app.ed.Caret = 0
	_ = handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keyEscape})
	_ = handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keySlash})
	_ = handleTextEvent(&app, "h", 0)
	_ = handleTextEvent(&app, "/", 0)

	if !app.searchActive {
		t.Fatalf("search should still be active after slash lock")
	}
	if !handleTextEvent(&app, "e", 0) {
		t.Fatalf("typing should continue")
	}
	if app.searchActive {
		t.Fatalf("typing non-tab/non-x should exit search mode")
	}
	if got := string(app.ed.Buf); got != "zero ehello one hello two" {
		t.Fatalf("typed rune should be inserted after exiting search, got %q", got)
	}
}

func TestSearchBeforeLockXExtendsPatternNotLineHighlight(t *testing.T) {
	app := appState{}
	app.initBuffers(editor.NewEditor("xylophone xx"))
	app.ed.Caret = 0
	_ = handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keyEscape})
	_ = handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keySlash})
	_ = handleTextEvent(&app, "x", 0)

	if !handleTextEvent(&app, "x", 0) {
		t.Fatalf("typing x in unlocked search should continue")
	}
	if !app.searchActive {
		t.Fatalf("search should stay active before lock")
	}
	if app.searchPatternDone {
		t.Fatalf("search should not lock before slash")
	}
	if app.lineHighlightMode {
		t.Fatalf("x before lock must not enter line-highlight mode")
	}
	if got := string(app.searchQuery); got != "xx" {
		t.Fatalf("query should keep growing before lock, got %q", got)
	}
}

func TestEmptySearchPatternRedoesLastSearch(t *testing.T) {
	app := appState{}
	app.initBuffers(editor.NewEditor("a hello b hello c"))
	app.ed.Caret = 0

	_ = handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keyEscape})
	_ = handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keySlash})
	_ = handleTextEvent(&app, "h", 0)
	_ = handleTextEvent(&app, "e", 0)
	_ = handleTextEvent(&app, "l", 0)
	_ = handleTextEvent(&app, "l", 0)
	_ = handleTextEvent(&app, "o", 0)
	_ = handleTextEvent(&app, "/", 0) // lock search
	if app.ed.Caret != 2 {
		t.Fatalf("expected first match at 2, got %d", app.ed.Caret)
	}
	_ = handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keyEscape}) // exit search

	_ = handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keyEscape})
	_ = handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keySlash})
	if !handleTextEvent(&app, "/", 0) { // empty pattern: redo last search
		t.Fatalf("empty pattern lock should continue")
	}
	if app.ed.Caret != 10 {
		t.Fatalf("expected redo to next match at 10, got %d", app.ed.Caret)
	}
	if got := string(app.searchQuery); got != "hello" {
		t.Fatalf("expected reused query 'hello', got %q", got)
	}
}

func TestCtrlSlashTogglesCommentAndDoesNotStartSearch(t *testing.T) {
	app := appState{}
	app.initBuffers(editor.NewEditor("line\n"))
	app.ed.Caret = 0

	if !handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keySlash, mods: modCtrl}) {
		t.Fatalf("ctrl+/ should continue")
	}
	if app.searchActive {
		t.Fatalf("ctrl+/ should not start search mode")
	}
	if got := string(app.ed.Buf); got != "//line\n" {
		t.Fatalf("ctrl+/ should toggle comment, got %q", got)
	}
}

func TestSearchModeBackspaceCancelsAndDeletesSelection(t *testing.T) {
	app := appState{}
	app.initBuffers(editor.NewEditor("zero hello one hello two"))
	app.ed.Caret = 0
	_ = handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keyEscape})
	_ = handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keySlash})
	_ = handleTextEvent(&app, "h", 0)
	_ = handleTextEvent(&app, "/", 0)

	if !app.searchActive || !app.ed.Sel.Active {
		t.Fatalf("search should have active match selection")
	}
	if !handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keyBackspace}) {
		t.Fatalf("backspace should continue")
	}
	if app.searchActive {
		t.Fatalf("backspace should cancel search mode")
	}
	if got := string(app.ed.Buf); got != "zerohello one hello two" {
		t.Fatalf("backspace should apply normal behavior, got %q", got)
	}
}

func TestSearchModeDeleteCancelsAndDeletesWordAtCaret(t *testing.T) {
	app := appState{}
	app.initBuffers(editor.NewEditor("zero hello one hello two"))
	app.ed.Caret = 0
	_ = handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keyEscape})
	_ = handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keySlash})
	_ = handleTextEvent(&app, "h", 0)
	_ = handleTextEvent(&app, "/", 0)

	if !handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keyDelete}) {
		t.Fatalf("delete should continue")
	}
	if app.searchActive {
		t.Fatalf("delete should cancel search mode")
	}
	if got := string(app.ed.Buf); got != "zero  one hello two" {
		t.Fatalf("delete should apply normal behavior, got %q", got)
	}
}

func TestSearchModeXStartsLineHighlightAndNextXExtends(t *testing.T) {
	app := appState{}
	app.initBuffers(editor.NewEditor("a\nhello\nb\nc\n"))
	app.ed.Caret = 0
	_ = handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keyEscape})
	_ = handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keySlash})
	_ = handleTextEvent(&app, "h", 0)
	_ = handleTextEvent(&app, "/", 0)

	if !handleTextEvent(&app, "x", 0) {
		t.Fatalf("x should continue")
	}
	if app.searchActive {
		t.Fatalf("x should exit search mode")
	}
	if !app.lineHighlightMode {
		t.Fatalf("x should enter line highlight mode")
	}
	a, b := app.ed.Sel.Normalised()
	if a != 2 || b != 8 {
		t.Fatalf("first x should mark matched line, got (%d,%d)", a, b)
	}
	if !handleTextEvent(&app, "x", 0) {
		t.Fatalf("second x should continue")
	}
	a, b = app.ed.Sel.Normalised()
	if a != 2 || b != 10 {
		t.Fatalf("second x should extend by one line, got (%d,%d)", a, b)
	}
}

func TestDeleteIgnoresSelectionAndDeletesWordAtCaret(t *testing.T) {
	app := appState{}
	app.initBuffers(editor.NewEditor("alpha beta gamma"))
	app.ed.Caret = 6 // on "beta"
	app.ed.Sel.Active = true
	app.ed.Sel.A = 0
	app.ed.Sel.B = 5 // "alpha"

	if !handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keyDelete}) {
		t.Fatalf("delete should continue")
	}
	if got := string(app.ed.Buf); got != "alpha  gamma" {
		t.Fatalf("delete should remove word at caret, got %q", got)
	}
}

func TestSearchModeShiftDeleteCancelsAndDeletesLine(t *testing.T) {
	app := appState{}
	app.initBuffers(editor.NewEditor("a\nhello\nb\n"))
	app.ed.Caret = 0
	_ = handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keyEscape})
	_ = handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keySlash})
	_ = handleTextEvent(&app, "h", 0)
	_ = handleTextEvent(&app, "/", 0)

	if !handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keyDelete, mods: modShift}) {
		t.Fatalf("shift+delete should continue")
	}
	if app.searchActive {
		t.Fatalf("shift+delete should cancel search mode")
	}
	if got := string(app.ed.Buf); got != "a\nb\n" {
		t.Fatalf("shift+delete should apply normal behavior, got %q", got)
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
