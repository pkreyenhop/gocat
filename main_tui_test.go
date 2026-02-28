package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"gc/editor"

	"github.com/gdamore/tcell/v2"
)

func TestCtrlRuneToKey(t *testing.T) {
	if got, ok := ctrlRuneToKey('q'); !ok || got != keyQ {
		t.Fatalf("ctrlRuneToKey('q') = %v %v, want keyQ true", got, ok)
	}
	if got, ok := ctrlRuneToKey('i'); !ok || got != keyTab {
		t.Fatalf("ctrlRuneToKey('i') = %v %v, want keyTab true", got, ok)
	}
	if got, ok := ctrlRuneToKey(','); !ok || got != keyComma {
		t.Fatalf("ctrlRuneToKey(',') = %v %v, want keyComma true", got, ok)
	}
	if got, ok := ctrlRuneToKey('.'); !ok || got != keyPeriod {
		t.Fatalf("ctrlRuneToKey('.') = %v %v, want keyPeriod true", got, ok)
	}
	if got, ok := ctrlRuneToKey('<'); !ok || got != keyComma {
		t.Fatalf("ctrlRuneToKey('<') = %v %v, want keyComma true", got, ok)
	}
	if got, ok := ctrlRuneToKey('>'); !ok || got != keyPeriod {
		t.Fatalf("ctrlRuneToKey('>') = %v %v, want keyPeriod true", got, ok)
	}
}

func TestTcellCtrlIMapsToTab(t *testing.T) {
	ev := tcell.NewEventKey(tcell.KeyCtrlI, 0, tcell.ModCtrl)
	got, ok := tcellKeyToKeyCode(ev)
	if !ok || got != keyTab {
		t.Fatalf("tcellKeyToKeyCode(CtrlI) = %v %v, want keyTab true", got, ok)
	}
}

func TestTUIEscPrefixThenFInvokesFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "p.go")
	if err := os.WriteFile(path, []byte("package main\nfunc main(){\n}\n"), 0644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	app := appState{}
	app.initBuffers(editor.NewEditor("package main\nfunc main(){\n}\n"))
	app.currentPath = path
	app.buffers[0].path = path
	app.buffers[0].dirty = true
	app.ed.Caret = 1

	called := false
	oldRun := runFmtFix
	defer func() { runFmtFix = oldRun }()
	runFmtFix = func(p string) error {
		called = true
		return nil
	}

	if !handleTUIKey(&app, tcell.NewEventKey(tcell.KeyEscape, 0, 0)) {
		t.Fatal("Esc should not quit in prefix case")
	}
	if !app.cmdPrefixActive {
		t.Fatal("Esc away from 1,1 should arm prefix mode")
	}
	if !handleTUIKey(&app, tcell.NewEventKey(tcell.KeyRune, 'f', 0)) {
		t.Fatal("prefixed f should not quit")
	}
	if !called {
		t.Fatal("Esc then f should invoke format command")
	}
}

func TestTUIEscPrefixConsumesRuneWithoutInsertion(t *testing.T) {
	app := appState{}
	app.initBuffers(editor.NewEditor("abc"))
	app.ed.Caret = 1
	app.cmdPrefixActive = true
	before := string(app.ed.Buf)

	if !handleTUIKey(&app, tcell.NewEventKey(tcell.KeyRune, 'z', 0)) {
		t.Fatal("prefix rune should not quit")
	}
	if got := string(app.ed.Buf); got != before {
		t.Fatalf("prefix rune should not insert text, got %q", got)
	}
	if app.cmdPrefixActive {
		t.Fatal("prefix should be consumed after next key")
	}
}

func TestTUICtrlCommaPeriodMoveCaretPage(t *testing.T) {
	var b strings.Builder
	for range 200 {
		b.WriteString("line\n")
	}
	app := appState{}
	app.initBuffers(editor.NewEditor(b.String()))
	app.ed.Caret = len(app.ed.Buf)

	before := app.ed.Caret
	if !handleTUIKey(&app, tcell.NewEventKey(tcell.KeyRune, ',', tcell.ModCtrl)) {
		t.Fatal("Ctrl+, should not quit")
	}
	if app.ed.Caret >= before {
		t.Fatalf("Ctrl+, should move caret backward by page: before=%d after=%d", before, app.ed.Caret)
	}

	before = app.ed.Caret
	if !handleTUIKey(&app, tcell.NewEventKey(tcell.KeyRune, '.', tcell.ModCtrl)) {
		t.Fatal("Ctrl+. should not quit")
	}
	if app.ed.Caret <= before {
		t.Fatalf("Ctrl+. should move caret forward by page: before=%d after=%d", before, app.ed.Caret)
	}
}

func TestTUIEscPrefixCommaPeriodMoveCaretPage(t *testing.T) {
	var b strings.Builder
	for range 200 {
		b.WriteString("line\n")
	}
	app := appState{}
	app.initBuffers(editor.NewEditor(b.String()))
	app.ed.Caret = len(app.ed.Buf)

	before := app.ed.Caret
	if !handleTUIKey(&app, tcell.NewEventKey(tcell.KeyEscape, 0, 0)) {
		t.Fatal("Esc should not quit")
	}
	if !handleTUIKey(&app, tcell.NewEventKey(tcell.KeyRune, ',', 0)) {
		t.Fatal("Esc then , should not quit")
	}
	if app.ed.Caret >= before {
		t.Fatalf("Esc+, should move caret backward by page: before=%d after=%d", before, app.ed.Caret)
	}

	before = app.ed.Caret
	if !handleTUIKey(&app, tcell.NewEventKey(tcell.KeyEscape, 0, 0)) {
		t.Fatal("Esc should not quit")
	}
	if !handleTUIKey(&app, tcell.NewEventKey(tcell.KeyRune, '.', 0)) {
		t.Fatal("Esc then . should not quit")
	}
	if app.ed.Caret <= before {
		t.Fatalf("Esc+. should move caret forward by page: before=%d after=%d", before, app.ed.Caret)
	}
}

func TestTUIEscSpaceLessModeSequence(t *testing.T) {
	var b strings.Builder
	for range 200 {
		b.WriteString("line\n")
	}
	app := appState{}
	app.initBuffers(editor.NewEditor(b.String()))
	app.ed.Caret = 0

	if !handleTUIKey(&app, tcell.NewEventKey(tcell.KeyEscape, 0, 0)) {
		t.Fatal("Esc should arm prefix")
	}
	if !handleTUIKey(&app, tcell.NewEventKey(tcell.KeyRune, ' ', 0)) {
		t.Fatal("Esc+Space should enter less mode")
	}
	if !app.lessMode {
		t.Fatal("less mode should be on")
	}
	before := app.ed.Caret
	if !handleTUIKey(&app, tcell.NewEventKey(tcell.KeyRune, ' ', 0)) {
		t.Fatal("space should page in less mode")
	}
	if app.ed.Caret <= before {
		t.Fatalf("space in less mode should page down: before=%d after=%d", before, app.ed.Caret)
	}
	if !handleTUIKey(&app, tcell.NewEventKey(tcell.KeyRune, ' ', 0)) {
		t.Fatal("second space should page again")
	}
	if !app.lessMode {
		t.Fatal("less mode should stay active across spaces")
	}
	if !handleTUIKey(&app, tcell.NewEventKey(tcell.KeyEscape, 0, 0)) {
		t.Fatal("Esc should exit less mode")
	}
	if app.lessMode {
		t.Fatal("less mode should be off after Esc")
	}
}

func TestTUIEscShiftQClosesAllBuffers(t *testing.T) {
	app := appState{}
	app.initBuffers(editor.NewEditor("one"))
	app.addBuffer()
	if len(app.buffers) != 2 {
		t.Fatalf("expected 2 buffers, got %d", len(app.buffers))
	}
	if !handleTUIKey(&app, tcell.NewEventKey(tcell.KeyEscape, 0, 0)) {
		t.Fatal("Esc should arm prefix")
	}
	if handleTUIKey(&app, tcell.NewEventKey(tcell.KeyRune, 'Q', tcell.ModShift)) {
		t.Fatal("Esc+Shift+Q should close all buffers and quit")
	}
}

func TestTUIEscShiftSSavesDirtyBuffers(t *testing.T) {
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

	if !handleTUIKey(&app, tcell.NewEventKey(tcell.KeyEscape, 0, 0)) {
		t.Fatal("Esc should arm prefix")
	}
	// Uppercase rune without relying on modifier flags; shift is inferred.
	if !handleTUIKey(&app, tcell.NewEventKey(tcell.KeyRune, 'S', 0)) {
		t.Fatal("Esc+Shift+S should not quit")
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

func TestTUIEscITogglesSymbolInfoPopup(t *testing.T) {
	app := appState{noGopls: true}
	app.initBuffers(editor.NewEditor("package main\n"))
	app.currentPath = "p1.go"
	app.buffers[0].path = "p1.go"
	app.ed.Caret = 2

	if !handleTUIKey(&app, tcell.NewEventKey(tcell.KeyEscape, 0, 0)) {
		t.Fatal("Esc should arm prefix")
	}
	if !handleTUIKey(&app, tcell.NewEventKey(tcell.KeyRune, 'i', 0)) {
		t.Fatal("Esc+i should open info popup")
	}
	if app.symbolInfoPopup == "" {
		t.Fatal("expected symbol info popup to be open")
	}
	if !handleTUIKey(&app, tcell.NewEventKey(tcell.KeyEscape, 0, 0)) {
		t.Fatal("Esc should arm prefix")
	}
	if !handleTUIKey(&app, tcell.NewEventKey(tcell.KeyRune, 'i', 0)) {
		t.Fatal("Esc+i should close info popup when already open")
	}
	if app.symbolInfoPopup != "" {
		t.Fatal("expected symbol info popup to be closed")
	}
}

func TestTUIEscClosesSymbolInfoPopup(t *testing.T) {
	app := appState{noGopls: true}
	app.initBuffers(editor.NewEditor("package main\n"))
	app.currentPath = "p1.go"
	app.buffers[0].path = "p1.go"
	app.ed.Caret = 2
	app.symbolInfoPopup = "info text"

	if !handleTUIKey(&app, tcell.NewEventKey(tcell.KeyEscape, 0, 0)) {
		t.Fatal("Esc should not quit")
	}
	if app.symbolInfoPopup != "" {
		t.Fatal("Esc should close symbol info popup")
	}
	if app.cmdPrefixActive {
		t.Fatal("Esc should close popup, not arm prefix")
	}
}

func TestPopupVisibleLinesRespectsScroll(t *testing.T) {
	lines := []string{"l1", "l2", "l3", "l4", "l5"}
	got := popupVisibleLines(lines, 2, 2)
	if len(got) != 2 || got[0] != "l3" || got[1] != "l4" {
		t.Fatalf("unexpected popup slice: %#v", got)
	}

	got = popupVisibleLines(lines, 99, 3)
	if len(got) != 1 || got[0] != "l5" {
		t.Fatalf("scroll should clamp to end, got %#v", got)
	}

	if got = popupVisibleLines(lines, 0, 0); got != nil {
		t.Fatalf("maxLines=0 should return nil, got %#v", got)
	}
}

func TestRenderDataCachesByBufferRevision(t *testing.T) {
	app := appState{syntaxHL: newGoHighlighter()}
	app.initBuffers(editor.NewEditor("package main\n"))
	app.currentPath = "p.go"
	app.buffers[0].path = "p.go"

	lines1, _, lang1 := renderData(&app)
	if lang1 != "go" {
		t.Fatalf("lang1=%q, want go", lang1)
	}
	rev1 := app.render.rev
	if rev1 != app.buffers[0].rev {
		t.Fatalf("cache rev mismatch: render=%d buffer=%d", rev1, app.buffers[0].rev)
	}

	lines2, _, lang2 := renderData(&app)
	if lang2 != "go" {
		t.Fatalf("lang2=%q, want go", lang2)
	}
	if len(lines1) != len(lines2) {
		t.Fatalf("expected cached lines length, got %d and %d", len(lines1), len(lines2))
	}

	app.ed.InsertText("func main() {}\n")
	app.markDirty()
	lines3, _, _ := renderData(&app)
	if app.render.rev == rev1 {
		t.Fatalf("expected cache rev to update after edit")
	}
	if len(lines3) <= len(lines2) {
		t.Fatalf("expected more lines after edit: before=%d after=%d", len(lines2), len(lines3))
	}
}

func BenchmarkRenderDataCache(b *testing.B) {
	var src strings.Builder
	src.WriteString("package main\n\n")
	src.WriteString("import \"fmt\"\n\n")
	for i := range 400 {
		src.WriteString("func f")
		src.WriteString(strconv.Itoa(i))
		src.WriteString("() { fmt.Println(\"hello\", ")
		src.WriteString(strconv.Itoa(i))
		src.WriteString(") }\n")
	}
	text := src.String()

	b.Run("miss", func(b *testing.B) {
		app := appState{syntaxHL: newGoHighlighter()}
		app.initBuffers(editor.NewEditor(text))
		app.currentPath = "bench.go"
		app.buffers[0].path = "bench.go"
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			app.touchActiveBuffer()
			_, _, _ = renderData(&app)
		}
	})

	b.Run("hit", func(b *testing.B) {
		app := appState{syntaxHL: newGoHighlighter()}
		app.initBuffers(editor.NewEditor(text))
		app.currentPath = "bench.go"
		app.buffers[0].path = "bench.go"
		_, _, _ = renderData(&app)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _, _ = renderData(&app)
		}
	})
}

func TestRenderDataStartupFastSkipsHighlightOnce(t *testing.T) {
	app := appState{syntaxHL: newGoHighlighter(), startupFast: true}
	app.initBuffers(editor.NewEditor("package main\nfunc main() {}\n"))
	app.currentPath = "p.go"
	app.buffers[0].path = "p.go"

	_, styles1, lang1 := renderData(&app)
	if lang1 != "go" {
		t.Fatalf("lang1=%q, want go", lang1)
	}
	if styles1 != nil {
		t.Fatalf("expected startup fast render to skip highlighting")
	}
	if app.startupFast {
		t.Fatalf("startupFast should be consumed after first render")
	}

	_, styles2, _ := renderData(&app)
	if styles2 == nil {
		t.Fatalf("expected second render to include highlighting")
	}
}
