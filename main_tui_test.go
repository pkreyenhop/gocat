package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

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

func TestDrawStyledTUICellLine_TabKeepsStyleAlignment(t *testing.T) {
	s := tcell.NewSimulationScreen("UTF-8")
	if err := s.Init(); err != nil {
		t.Fatalf("init simulation screen: %v", err)
	}
	defer s.Fini()

	base := tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorBlack)
	line := "\tif x"
	styles := []tokenStyle{styleDefault, styleKeyword, styleKeyword, styleDefault, styleDefault}
	drawStyledTUICellLine(s, 0, 0, line, styles, base, 0, nil)

	_, _, got, _ := s.GetContent(tabWidth, 0)
	gotFg, _, _ := got.Decompose()
	wantFg, _, _ := tuiStyleForToken(base, styleKeyword).Decompose()
	if gotFg != wantFg {
		t.Fatalf("tab-aligned rune foreground=%v, want %v", gotFg, wantFg)
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
	before := app.ed.String()

	if !handleTUIKey(&app, tcell.NewEventKey(tcell.KeyRune, 'z', 0)) {
		t.Fatal("prefix rune should not quit")
	}
	if got := app.ed.String(); got != before {
		t.Fatalf("prefix rune should not insert text, got %q", got)
	}
	if app.cmdPrefixActive {
		t.Fatal("prefix should be consumed after next key")
	}
}

func TestTUIEscDelayShowsShortcutHelpPopup(t *testing.T) {
	app := appState{}
	app.initBuffers(editor.NewEditor("abc"))
	app.escHelpDelay = time.Second

	if !handleTUIKey(&app, tcell.NewEventKey(tcell.KeyEscape, 0, 0)) {
		t.Fatal("Esc should arm prefix")
	}
	if !app.cmdPrefixActive {
		t.Fatal("expected command prefix active")
	}
	app.escPrefixAt = time.Now().Add(-2 * time.Second)
	handleTUIInterrupt(&app, tcell.NewEventInterrupt(app.escHelpToken))
	if !app.escHelpVisible {
		t.Fatal("interrupt after delay should show Esc help popup")
	}
	if !handleTUIKey(&app, tcell.NewEventKey(tcell.KeyRune, 'b', 0)) {
		t.Fatal("Esc+b should continue")
	}
	if app.escHelpVisible {
		t.Fatal("consuming prefixed key should hide Esc help popup")
	}
}

func TestTUIEscHelpPopupIgnoresStaleInterrupt(t *testing.T) {
	app := appState{}
	app.initBuffers(editor.NewEditor("abc"))
	app.escHelpDelay = time.Millisecond
	if !handleTUIKey(&app, tcell.NewEventKey(tcell.KeyEscape, 0, 0)) {
		t.Fatal("Esc should arm prefix")
	}
	token := app.escHelpToken
	if !handleTUIKey(&app, tcell.NewEventKey(tcell.KeyRune, 'a', 0)) {
		t.Fatal("prefixed key should consume prefix")
	}
	if !handleTUIKey(&app, tcell.NewEventKey(tcell.KeyEscape, 0, 0)) {
		t.Fatal("Esc should arm prefix again")
	}
	// stale token from a previous arm should not open popup
	handleTUIInterrupt(&app, tcell.NewEventInterrupt(token))
	if app.escHelpVisible {
		t.Fatal("stale interrupt should not show popup")
	}
}

func TestEscHelpPopupShowsNextLetterCommandsOnly(t *testing.T) {
	lines := escHelpPopupLines()
	if len(lines) == 0 {
		t.Fatal("expected non-empty Esc help lines")
	}
	all := strings.Join(lines, "\n")
	if strings.Contains(all, "Ctrl+") || strings.Contains(all, "ctrl+") {
		t.Fatalf("Esc help should not list Ctrl sequences, got:\n%s", all)
	}
	if !strings.Contains(all, "  f  save + fmt/fix + reload") {
		t.Fatalf("Esc help should list next-letter commands, got:\n%s", all)
	}
}

func TestTUIEscPrefixCommandDoesNotSwallowNextTypedRune(t *testing.T) {
	app := appState{}
	app.initBuffers(editor.NewEditor(""))

	if !handleTUIKey(&app, tcell.NewEventKey(tcell.KeyEscape, 0, 0)) {
		t.Fatal("Esc should arm prefix")
	}
	if !handleTUIKey(&app, tcell.NewEventKey(tcell.KeyRune, 'b', 0)) {
		t.Fatal("Esc+b should execute command")
	}
	if len(app.buffers) != 2 {
		t.Fatalf("Esc+b should create buffer, got %d", len(app.buffers))
	}
	if !handleTUIKey(&app, tcell.NewEventKey(tcell.KeyRune, 'a', 0)) {
		t.Fatal("typing should continue")
	}
	if got := app.ed.String(); got != "a" {
		t.Fatalf("first typed rune should not be swallowed, got %q", got)
	}
}

func TestTUIShiftArrowsActivateSelection(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		start     int
		ev        *tcell.EventKey
		wantCaret int
		wantA     int
		wantB     int
	}{
		{
			name:      "shift right",
			text:      "abc",
			start:     1,
			ev:        tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModShift),
			wantCaret: 2,
			wantA:     1,
			wantB:     2,
		},
		{
			name:      "shift left",
			text:      "abc",
			start:     2,
			ev:        tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModShift),
			wantCaret: 1,
			wantA:     1,
			wantB:     2,
		},
		{
			name:      "shift down",
			text:      "ab\ncd\n",
			start:     1,
			ev:        tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModShift),
			wantCaret: 3,
			wantA:     0,
			wantB:     6,
		},
		{
			name:      "shift up",
			text:      "ab\ncd\n",
			start:     4,
			ev:        tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModShift),
			wantCaret: 0,
			wantA:     0,
			wantB:     6,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app := appState{}
			app.initBuffers(editor.NewEditor(tc.text))
			app.ed.Caret = tc.start
			if !handleTUIKey(&app, tc.ev) {
				t.Fatal("shift+arrow should not quit")
			}
			if app.ed.Caret != tc.wantCaret {
				t.Fatalf("caret=%d, want %d", app.ed.Caret, tc.wantCaret)
			}
			if !app.ed.Sel.Active {
				t.Fatal("selection should be active")
			}
			gotA, gotB := app.ed.Sel.Normalised()
			if gotA != tc.wantA || gotB != tc.wantB {
				t.Fatalf("selection=(%d,%d), want (%d,%d)", gotA, gotB, tc.wantA, tc.wantB)
			}
		})
	}
}

func TestTUIEscCSIShiftDownExtendsLineSelection(t *testing.T) {
	app := appState{}
	app.initBuffers(editor.NewEditor("l1\nl2\nl3\n"))
	app.ed.Caret = 1

	seq := []*tcell.EventKey{
		tcell.NewEventKey(tcell.KeyEscape, 0, 0),
		tcell.NewEventKey(tcell.KeyRune, '[', 0),
		tcell.NewEventKey(tcell.KeyRune, '1', 0),
		tcell.NewEventKey(tcell.KeyRune, ';', 0),
		tcell.NewEventKey(tcell.KeyRune, '2', 0),
		tcell.NewEventKey(tcell.KeyRune, 'B', 0),
	}
	for _, ev := range seq {
		if !handleTUIKey(&app, ev) {
			t.Fatal("CSI sequence should not quit")
		}
	}

	if !app.ed.Sel.Active {
		t.Fatal("selection should be active after Esc[1;2B")
	}
	a, b := app.ed.Sel.Normalised()
	if a != 0 || b != 6 {
		t.Fatalf("selection=(%d,%d), want (0,6)", a, b)
	}
	if app.ed.Caret != 3 {
		t.Fatalf("caret=%d, want 3", app.ed.Caret)
	}
}

func TestTUIEscCSIPlainDownMovesWithoutSelection(t *testing.T) {
	app := appState{}
	app.initBuffers(editor.NewEditor("l1\nl2\nl3\n"))
	app.ed.Caret = 1

	seq := []*tcell.EventKey{
		tcell.NewEventKey(tcell.KeyEscape, 0, 0),
		tcell.NewEventKey(tcell.KeyRune, '[', 0),
		tcell.NewEventKey(tcell.KeyRune, 'B', 0),
	}
	for _, ev := range seq {
		if !handleTUIKey(&app, ev) {
			t.Fatal("CSI sequence should not quit")
		}
	}

	if app.ed.Sel.Active {
		t.Fatal("plain Esc[B should not activate selection")
	}
	if app.ed.Caret != 4 {
		t.Fatalf("caret=%d, want 4", app.ed.Caret)
	}
}

func TestTUIEscXLineHighlightModeAndExtend(t *testing.T) {
	app := appState{}
	app.initBuffers(editor.NewEditor("l1\nl2\nl3\n"))
	app.ed.Caret = 4 // line 2

	if !handleTUIKey(&app, tcell.NewEventKey(tcell.KeyEscape, 0, 0)) {
		t.Fatal("Esc should arm prefix")
	}
	if !handleTUIKey(&app, tcell.NewEventKey(tcell.KeyRune, 'x', 0)) {
		t.Fatal("Esc+x should start line highlight mode")
	}
	if !app.lineHighlightMode {
		t.Fatal("line highlight mode should be active")
	}
	a, b := app.ed.Sel.Normalised()
	if a != 3 || b != 6 {
		t.Fatalf("selection after Esc+x=(%d,%d), want (3,6)", a, b)
	}

	if !handleTUIKey(&app, tcell.NewEventKey(tcell.KeyRune, 'x', 0)) {
		t.Fatal("x should extend line highlight")
	}
	a, b = app.ed.Sel.Normalised()
	if a != 3 || b != 9 {
		t.Fatalf("selection after x=(%d,%d), want (3,9)", a, b)
	}
	if got := app.ed.String(); got != "l1\nl2\nl3\n" {
		t.Fatalf("x should not insert text, got %q", got)
	}
}

func TestTUIEscSlashSearchModeAndTabNextWrap(t *testing.T) {
	app := appState{}
	app.initBuffers(editor.NewEditor("zero hello one hello two"))
	app.ed.Caret = 0

	if !handleTUIKey(&app, tcell.NewEventKey(tcell.KeyEscape, 0, 0)) {
		t.Fatal("Esc should arm prefix")
	}
	if !handleTUIKey(&app, tcell.NewEventKey(tcell.KeyRune, '/', 0)) {
		t.Fatal("Esc+/ should start search mode")
	}
	if !app.searchActive {
		t.Fatal("search mode should be active")
	}
	if !handleTUIKey(&app, tcell.NewEventKey(tcell.KeyRune, 'h', 0)) {
		t.Fatal("typing search query should continue")
	}
	if !handleTUIKey(&app, tcell.NewEventKey(tcell.KeyRune, 'e', 0)) {
		t.Fatal("typing search query should continue")
	}
	if !handleTUIKey(&app, tcell.NewEventKey(tcell.KeyRune, 'l', 0)) {
		t.Fatal("typing search query should continue")
	}
	if app.ed.Caret != 5 {
		t.Fatalf("caret after incremental search=%d, want 5", app.ed.Caret)
	}
	a, b := app.ed.Sel.Normalised()
	if a != 5 || b != 8 {
		t.Fatalf("selection after incremental search=(%d,%d), want (5,8)", a, b)
	}
	if !handleTUIKey(&app, tcell.NewEventKey(tcell.KeyRune, '/', 0)) {
		t.Fatal("slash should lock search pattern")
	}
	if !app.searchPatternDone {
		t.Fatal("slash should finalize pattern entry")
	}
	if !handleTUIKey(&app, tcell.NewEventKey(tcell.KeyTAB, 0, 0)) {
		t.Fatal("Tab in search mode should continue")
	}
	if app.ed.Caret != 15 {
		t.Fatalf("caret after next match=%d, want 15", app.ed.Caret)
	}
	a, b = app.ed.Sel.Normalised()
	if a != 15 || b != 18 {
		t.Fatalf("selection after next match=(%d,%d), want (15,18)", a, b)
	}
	if !handleTUIKey(&app, tcell.NewEventKey(tcell.KeyTAB, 0, 0)) {
		t.Fatal("Tab wrap in search mode should continue")
	}
	if app.ed.Caret != 5 {
		t.Fatalf("caret after wrapped match=%d, want 5", app.ed.Caret)
	}
	if !handleTUIKey(&app, tcell.NewEventKey(tcell.KeyBacktab, 0, 0)) {
		t.Fatal("Shift+Tab in search mode should continue")
	}
	if app.ed.Caret != 15 {
		t.Fatalf("caret after previous match=%d, want 15", app.ed.Caret)
	}
}

func TestTUISearchFinalizeWithSlashThenAnyOtherRuneExitsSearchAndInserts(t *testing.T) {
	app := appState{}
	app.initBuffers(editor.NewEditor("zero hello one hello two"))
	app.ed.Caret = 0
	_ = handleTUIKey(&app, tcell.NewEventKey(tcell.KeyEscape, 0, 0))
	_ = handleTUIKey(&app, tcell.NewEventKey(tcell.KeyRune, '/', 0))
	_ = handleTUIKey(&app, tcell.NewEventKey(tcell.KeyRune, 'h', 0))
	_ = handleTUIKey(&app, tcell.NewEventKey(tcell.KeyRune, '/', 0))

	if !handleTUIKey(&app, tcell.NewEventKey(tcell.KeyRune, 'e', 0)) {
		t.Fatal("typing should continue")
	}
	if app.searchActive {
		t.Fatal("typing non-tab/non-x should exit search mode")
	}
	if got := app.ed.String(); got != "zero ehello one hello two" {
		t.Fatalf("typed rune should be inserted after exiting search, got %q", got)
	}
}

func TestTUISearchBeforeLockXExtendsPatternNotLineHighlight(t *testing.T) {
	app := appState{}
	app.initBuffers(editor.NewEditor("xylophone xx"))
	app.ed.Caret = 0
	_ = handleTUIKey(&app, tcell.NewEventKey(tcell.KeyEscape, 0, 0))
	_ = handleTUIKey(&app, tcell.NewEventKey(tcell.KeyRune, '/', 0))
	_ = handleTUIKey(&app, tcell.NewEventKey(tcell.KeyRune, 'x', 0))

	if !handleTUIKey(&app, tcell.NewEventKey(tcell.KeyRune, 'x', 0)) {
		t.Fatal("typing x in unlocked search should continue")
	}
	if !app.searchActive {
		t.Fatal("search should stay active before lock")
	}
	if app.searchPatternDone {
		t.Fatal("search should not lock before slash")
	}
	if app.lineHighlightMode {
		t.Fatal("x before lock must not enter line-highlight mode")
	}
	if got := string(app.searchQuery); got != "xx" {
		t.Fatalf("query should keep growing before lock, got %q", got)
	}
}

func TestTUIEmptySearchPatternRedoesLastSearch(t *testing.T) {
	app := appState{}
	app.initBuffers(editor.NewEditor("a hello b hello c"))
	app.ed.Caret = 0

	_ = handleTUIKey(&app, tcell.NewEventKey(tcell.KeyEscape, 0, 0))
	_ = handleTUIKey(&app, tcell.NewEventKey(tcell.KeyRune, '/', 0))
	_ = handleTUIKey(&app, tcell.NewEventKey(tcell.KeyRune, 'h', 0))
	_ = handleTUIKey(&app, tcell.NewEventKey(tcell.KeyRune, 'e', 0))
	_ = handleTUIKey(&app, tcell.NewEventKey(tcell.KeyRune, 'l', 0))
	_ = handleTUIKey(&app, tcell.NewEventKey(tcell.KeyRune, 'l', 0))
	_ = handleTUIKey(&app, tcell.NewEventKey(tcell.KeyRune, 'o', 0))
	_ = handleTUIKey(&app, tcell.NewEventKey(tcell.KeyRune, '/', 0))
	if app.ed.Caret != 2 {
		t.Fatalf("expected first match at 2, got %d", app.ed.Caret)
	}
	_ = handleTUIKey(&app, tcell.NewEventKey(tcell.KeyEscape, 0, 0))

	_ = handleTUIKey(&app, tcell.NewEventKey(tcell.KeyEscape, 0, 0))
	_ = handleTUIKey(&app, tcell.NewEventKey(tcell.KeyRune, '/', 0))
	if !handleTUIKey(&app, tcell.NewEventKey(tcell.KeyRune, '/', 0)) {
		t.Fatal("empty pattern lock should continue")
	}
	if app.ed.Caret != 10 {
		t.Fatalf("expected redo to next match at 10, got %d", app.ed.Caret)
	}
	if got := string(app.searchQuery); got != "hello" {
		t.Fatalf("expected reused query 'hello', got %q", got)
	}
}

func TestTUISearchModeXStartsLineHighlightAndNextXExtends(t *testing.T) {
	app := appState{}
	app.initBuffers(editor.NewEditor("a\nhello\nb\nc\n"))
	app.ed.Caret = 0
	_ = handleTUIKey(&app, tcell.NewEventKey(tcell.KeyEscape, 0, 0))
	_ = handleTUIKey(&app, tcell.NewEventKey(tcell.KeyRune, '/', 0))
	_ = handleTUIKey(&app, tcell.NewEventKey(tcell.KeyRune, 'h', 0))
	_ = handleTUIKey(&app, tcell.NewEventKey(tcell.KeyRune, '/', 0))

	if !handleTUIKey(&app, tcell.NewEventKey(tcell.KeyRune, 'x', 0)) {
		t.Fatal("x should continue")
	}
	if app.searchActive {
		t.Fatal("x should exit search mode")
	}
	if !app.lineHighlightMode {
		t.Fatal("x should enter line highlight mode")
	}
	a, b := app.ed.Sel.Normalised()
	if a != 2 || b != 8 {
		t.Fatalf("first x should mark matched line, got (%d,%d)", a, b)
	}
	if !handleTUIKey(&app, tcell.NewEventKey(tcell.KeyRune, 'x', 0)) {
		t.Fatal("second x should continue")
	}
	a, b = app.ed.Sel.Normalised()
	if a != 2 || b != 10 {
		t.Fatalf("second x should extend by one line, got (%d,%d)", a, b)
	}
}

func TestTUICtrlSlashTogglesCommentAndDoesNotStartSearch(t *testing.T) {
	app := appState{}
	app.initBuffers(editor.NewEditor("line\n"))
	app.ed.Caret = 0

	if !handleTUIKey(&app, tcell.NewEventKey(tcell.KeyRune, '/', tcell.ModCtrl)) {
		t.Fatal("Ctrl+/ should continue")
	}
	if app.searchActive {
		t.Fatal("Ctrl+/ should not start search mode")
	}
	if got := app.ed.String(); got != "//line\n" {
		t.Fatalf("Ctrl+/ should toggle comment, got %q", got)
	}
}

func TestTUICtrlCommaPeriodMoveCaretPage(t *testing.T) {
	var b strings.Builder
	for range 200 {
		b.WriteString("line\n")
	}
	app := appState{}
	app.initBuffers(editor.NewEditor(b.String()))
	app.ed.Caret = app.ed.RuneLen()

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
	app.ed.Caret = app.ed.RuneLen()

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
	app.buffers[1].ed.SetRunes([]rune("clean"))
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

	lines1, _, lang1, _ := renderData(&app)
	if lang1 != "go" {
		t.Fatalf("lang1=%q, want go", lang1)
	}
	rev1 := app.render.textRev
	if rev1 != app.buffers[0].textRev {
		t.Fatalf("cache text rev mismatch: render=%d buffer=%d", rev1, app.buffers[0].textRev)
	}

	lines2, _, lang2, _ := renderData(&app)
	if lang2 != "go" {
		t.Fatalf("lang2=%q, want go", lang2)
	}
	if len(lines1) != len(lines2) {
		t.Fatalf("expected cached lines length, got %d and %d", len(lines1), len(lines2))
	}

	app.ed.InsertText("func main() {}\n")
	app.markDirty()
	lines3, _, _, _ := renderData(&app)
	if app.render.textRev == rev1 {
		t.Fatalf("expected cache rev to update after edit")
	}
	if len(lines3) <= len(lines2) {
		t.Fatalf("expected more lines after edit: before=%d after=%d", len(lines2), len(lines3))
	}
}

func TestRenderDataUsesPerBufferCacheWhenSwitchingBack(t *testing.T) {
	app := appState{syntaxHL: newGoHighlighter()}
	app.initBuffers(editor.NewEditor("package main\n"))
	app.currentPath = "a.go"
	app.buffers[0].path = "a.go"

	app.addBuffer()
	app.buffers[1].ed.SetRunes([]rune("package main\nfunc main() {}\n"))
	app.buffers[1].path = "b.go"
	app.currentPath = "b.go"
	app.markDirty()

	app.switchBuffer(-1)
	app.currentPath = "a.go"
	lines0, _, _, _ := renderData(&app)
	if len(lines0) == 0 {
		t.Fatalf("expected non-empty lines for buffer 0")
	}

	// Poison buffer-0 cache and verify switching away/back reuses it.
	app.buffers[0].cachedLines = []string{"cached-line"}
	app.buffers[0].cachedLineStyles = [][]tokenStyle{{styleComment}}
	app.buffers[0].cachedLangMode = "go"
	app.buffers[0].cachedTextRev = app.buffers[0].textRev
	app.buffers[0].cachedMode = app.buffers[0].mode
	app.buffers[0].cachedPath = app.buffers[0].path

	app.switchBuffer(1)
	app.currentPath = "b.go"
	_, _, _, _ = renderData(&app)

	app.switchBuffer(-1)
	app.currentPath = "a.go"
	linesBack, stylesBack, _, _ := renderData(&app)
	if len(linesBack) != 1 || linesBack[0] != "cached-line" {
		t.Fatalf("expected per-buffer cached lines, got %#v", linesBack)
	}
	if len(stylesBack) != 1 || len(stylesBack[0]) != 1 || stylesBack[0][0] != styleComment {
		t.Fatalf("expected per-buffer cached styles, got %#v", stylesBack)
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
			_, _, _, _ = renderData(&app)
		}
	})

	b.Run("text-miss", func(b *testing.B) {
		app := appState{syntaxHL: newGoHighlighter()}
		app.initBuffers(editor.NewEditor(text))
		app.currentPath = "bench.go"
		app.buffers[0].path = "bench.go"
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			app.touchActiveBufferText()
			_, _, _, _ = renderData(&app)
		}
	})

	b.Run("hit", func(b *testing.B) {
		app := appState{syntaxHL: newGoHighlighter()}
		app.initBuffers(editor.NewEditor(text))
		app.currentPath = "bench.go"
		app.buffers[0].path = "bench.go"
		_, _, _, _ = renderData(&app)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _, _, _ = renderData(&app)
		}
	})

	b.Run("alternate-buffers", func(b *testing.B) {
		app := appState{syntaxHL: newGoHighlighter()}
		app.initBuffers(editor.NewEditor(text))
		app.currentPath = "bench1.go"
		app.buffers[0].path = "bench1.go"
		app.addBuffer()
		app.buffers[1].ed.SetRunes([]rune(text))
		app.buffers[1].path = "bench2.go"
		app.markDirty()
		app.switchBuffer(-1)
		app.currentPath = "bench1.go"
		_, _, _, _ = renderData(&app)
		app.switchBuffer(1)
		app.currentPath = "bench2.go"
		_, _, _, _ = renderData(&app)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			app.switchBuffer(1)
			app.currentPath = app.buffers[app.bufIdx].path
			_, _, _, _ = renderData(&app)
		}
	})
}

func TestRenderDataStartupFastSkipsHighlightOnce(t *testing.T) {
	app := appState{syntaxHL: newGoHighlighter(), startupFast: true}
	app.initBuffers(editor.NewEditor("package main\nfunc main() {}\n"))
	app.currentPath = "p.go"
	app.buffers[0].path = "p.go"

	_, styles1, lang1, _ := renderData(&app)
	if lang1 != "go" {
		t.Fatalf("lang1=%q, want go", lang1)
	}
	if styles1 != nil {
		t.Fatalf("expected startup fast render to skip highlighting")
	}
	if app.startupFast {
		t.Fatalf("startupFast should be consumed after first render")
	}

	_, styles2, _, _ := renderData(&app)
	if styles2 == nil {
		t.Fatalf("expected second render to include highlighting")
	}
}
