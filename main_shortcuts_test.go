package main

import (
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"sdl-alt-test/editor"

	"github.com/veandco/go-sdl2/sdl"
)

var initSDLOnce sync.Once

func ensureSDL(t *testing.T) {
	t.Helper()
	initSDLOnce.Do(func() {
		_ = os.Setenv("SDL_VIDEODRIVER", "dummy")
		if err := sdl.Init(sdl.INIT_VIDEO); err != nil {
			t.Fatalf("SDL init: %v", err)
		}
	})
}

func TestShortcutCtrlBCreatesBufferAndTabCycles(t *testing.T) {
	ensureSDL(t)
	app := appState{}
	app.initBuffers(editor.NewEditor("one"))
	app.buffers[0].path = "one.txt"

	sdl.SetModState(sdl.KMOD_CTRL)
	if !handleEvent(&app, &sdl.KeyboardEvent{
		Type:   sdl.KEYDOWN,
		Repeat: 0,
		Keysym: sdl.Keysym{Sym: sdl.K_b},
	}) {
		t.Fatal("unexpected quit on Ctrl+B")
	}
	if len(app.buffers) != 2 {
		t.Fatalf("Ctrl+B should add buffer, got %d", len(app.buffers))
	}

	// Tab cycles forward
	sdl.SetModState(0)
	if !handleEvent(&app, &sdl.KeyboardEvent{
		Type:   sdl.KEYDOWN,
		Repeat: 0,
		Keysym: sdl.Keysym{Sym: sdl.K_TAB},
	}) {
		t.Fatal("unexpected quit on Tab")
	}
	if app.bufIdx != 0 {
		t.Fatalf("Tab should cycle to next buffer modulo N; got idx=%d", app.bufIdx)
	}
}

func TestShortcutCtrlQQuitsImmediateWithShift(t *testing.T) {
	ensureSDL(t)
	app := appState{}
	app.initBuffers(editor.NewEditor("first"))
	app.addBuffer()

	sdl.SetModState(sdl.KMOD_CTRL | sdl.KMOD_SHIFT)
	// Shift+Ctrl+Q quits
	if handleEvent(&app, &sdl.KeyboardEvent{
		Type:   sdl.KEYDOWN,
		Repeat: 0,
		Keysym: sdl.Keysym{Sym: sdl.K_q, Mod: sdl.KMOD_LSHIFT},
	}) {
		t.Fatal("expected quit on Ctrl+Shift+Q")
	}
}

func TestShortcutCtrlOOpensPickerAndCtrlLLoads(t *testing.T) {
	ensureSDL(t)
	root := t.TempDir()
	alpha := filepath.Join(root, "alpha.txt")
	bravo := filepath.Join(root, "bravo.txt")
	if err := os.WriteFile(alpha, []byte("AAA"), 0644); err != nil {
		t.Fatalf("write alpha: %v", err)
	}
	if err := os.WriteFile(bravo, []byte("BBB"), 0644); err != nil {
		t.Fatalf("write bravo: %v", err)
	}

	app := appState{openRoot: root}
	app.initBuffers(editor.NewEditor(""))

	// Ctrl+O opens picker
	sdl.SetModState(sdl.KMOD_CTRL)
	if !handleEvent(&app, &sdl.KeyboardEvent{
		Type:   sdl.KEYDOWN,
		Repeat: 0,
		Keysym: sdl.Keysym{Sym: sdl.K_o},
	}) {
		t.Fatal("unexpected quit on Ctrl+O")
	}
	if len(app.buffers) != 2 || !app.buffers[app.bufIdx].picker {
		t.Fatalf("Ctrl+O should create picker buffer; buffers=%d picker=%v", len(app.buffers), app.buffers[app.bufIdx].picker)
	}

	// Move caret to second line (bravo.txt)
	app.ed.Caret = len([]rune("alpha.txt")) + 1

	// Ctrl+L loads file under caret
	if !handleEvent(&app, &sdl.KeyboardEvent{
		Type:   sdl.KEYDOWN,
		Repeat: 0,
		Keysym: sdl.Keysym{Sym: sdl.K_l},
	}) {
		t.Fatal("unexpected quit on Ctrl+L")
	}
	if app.buffers[app.bufIdx].picker {
		t.Fatalf("picker flag should be cleared after Ctrl+L")
	}
	if got := string(app.ed.Buf); got != "BBB" {
		t.Fatalf("loaded buffer contents mismatch: %q", got)
	}
	if app.currentPath != bravo {
		t.Fatalf("currentPath: want %s, got %s", bravo, app.currentPath)
	}
}

func TestCursorKeyRepeatMovesCaret(t *testing.T) {
	app := appState{}
	app.initBuffers(editor.NewEditor("abcd"))
	app.ed.Caret = 2

	// first press
	if !handleEvent(&app, &sdl.KeyboardEvent{
		Type:   sdl.KEYDOWN,
		Repeat: 0,
		Keysym: sdl.Keysym{Sym: sdl.K_LEFT},
	}) {
		t.Fatal("unexpected quit on left")
	}
	if app.ed.Caret != 1 {
		t.Fatalf("caret after first left: want 1, got %d", app.ed.Caret)
	}

	// repeat event should move again
	if !handleEvent(&app, &sdl.KeyboardEvent{
		Type:   sdl.KEYDOWN,
		Repeat: 1,
		Keysym: sdl.Keysym{Sym: sdl.K_LEFT},
	}) {
		t.Fatal("unexpected quit on left repeat")
	}
	if app.ed.Caret != 0 {
		t.Fatalf("caret after repeat left: want 0, got %d", app.ed.Caret)
	}

	// PageDown should jump multiple lines
	app.ed.Buf = []rune("a\nb\nc\nd\ne\nf\n")
	app.ed.Caret = 0
	if !handleEvent(&app, &sdl.KeyboardEvent{
		Type:   sdl.KEYDOWN,
		Repeat: 0,
		Keysym: sdl.Keysym{Sym: sdl.K_PAGEDOWN},
	}) {
		t.Fatal("unexpected quit on PageDown")
	}
	if app.ed.Caret <= 0 {
		t.Fatalf("expected caret to move on PageDown, got %d", app.ed.Caret)
	}
}

func TestCtrlAETCKill(t *testing.T) {
	app := appState{}
	app.initBuffers(editor.NewEditor("hello\nworld"))
	app.ed.Caret = 3

	sdl.SetModState(sdl.KMOD_CTRL)
	// Ctrl+A to start of line
	if !handleEvent(&app, &sdl.KeyboardEvent{
		Type:   sdl.KEYDOWN,
		Repeat: 0,
		Keysym: sdl.Keysym{Sym: sdl.K_a},
	}) {
		t.Fatal("unexpected quit on Ctrl+A")
	}
	if app.ed.Caret != 0 {
		t.Fatalf("caret after Ctrl+A: want 0, got %d", app.ed.Caret)
	}

	// Ctrl+E to end of line
	sdl.SetModState(sdl.KMOD_CTRL)
	if !handleEvent(&app, &sdl.KeyboardEvent{
		Type:   sdl.KEYDOWN,
		Repeat: 0,
		Keysym: sdl.Keysym{Sym: sdl.K_e},
	}) {
		t.Fatal("unexpected quit on Ctrl+E")
	}
	if app.ed.Caret != 5 {
		t.Fatalf("Ctrl+E should move to end; caret=%d", app.ed.Caret)
	}

	// Ctrl+Shift+A to start of doc with selection
	sdl.SetModState(sdl.KMOD_CTRL | sdl.KMOD_SHIFT)
	if !handleEvent(&app, &sdl.KeyboardEvent{
		Type:   sdl.KEYDOWN,
		Repeat: 0,
		Keysym: sdl.Keysym{Sym: sdl.K_a, Mod: sdl.KMOD_LSHIFT},
	}) {
		t.Fatal("unexpected quit on Ctrl+Shift+A")
	}
	if app.ed.Caret != 0 || !app.ed.Sel.Active {
		t.Fatalf("Ctrl+Shift+A should select to top; caret=%d sel=%v", app.ed.Caret, app.ed.Sel.Active)
	}

	// Ctrl+Shift+E to end of doc with selection
	if !handleEvent(&app, &sdl.KeyboardEvent{
		Type:   sdl.KEYDOWN,
		Repeat: 0,
		Keysym: sdl.Keysym{Sym: sdl.K_e, Mod: sdl.KMOD_LSHIFT},
	}) {
		t.Fatal("unexpected quit on Ctrl+Shift+E to end")
	}
	if app.ed.Caret != len(app.ed.Buf) || !app.ed.Sel.Active {
		t.Fatalf("Ctrl+Shift+E should select to end; caret=%d sel=%v", app.ed.Caret, app.ed.Sel.Active)
	}

	// Ctrl+K kills to end of line (including newline)
	sdl.SetModState(sdl.KMOD_CTRL)
	if !handleEvent(&app, &sdl.KeyboardEvent{
		Type:   sdl.KEYDOWN,
		Repeat: 0,
		Keysym: sdl.Keysym{Sym: sdl.K_k},
	}) {
		t.Fatal("unexpected quit on Ctrl+K")
	}
	if string(app.ed.Buf) != "hello\nworld" {
		t.Fatalf("Ctrl+K should kill to end of line; buf=%q", string(app.ed.Buf))
	}

	// Type something then undo with Ctrl+U
	ti := &sdl.TextInputEvent{Type: sdl.TEXTINPUT}
	copy(ti.Text[:], []byte("!\x00"))
	if !handleEvent(&app, ti) {
		t.Fatal("unexpected quit on text input")
	}
	if !handleEvent(&app, &sdl.KeyboardEvent{
		Type:   sdl.KEYDOWN,
		Repeat: 0,
		Keysym: sdl.Keysym{Sym: sdl.K_u},
	}) {
		t.Fatal("unexpected quit on Ctrl+U")
	}
	if string(app.ed.Buf) != "hello\nworld" {
		t.Fatalf("Ctrl+U should undo the last edit; got %q", string(app.ed.Buf))
	}

	// Ctrl+U again should pop another change (original delete)
	if !handleEvent(&app, &sdl.KeyboardEvent{
		Type:   sdl.KEYDOWN,
		Repeat: 0,
		Keysym: sdl.Keysym{Sym: sdl.K_u},
	}) {
		t.Fatal("unexpected quit on second Ctrl+U")
	}
	if string(app.ed.Buf) != "hello\nworld" {
		t.Fatalf("second Ctrl+U should have no further effect; got %q", string(app.ed.Buf))
	}
}

func TestEscapeClearsSelectionAndQuitsOtherwise(t *testing.T) {
	app := appState{}
	app.initBuffers(editor.NewEditor("abc"))
	app.ed.Sel.Active = true
	app.ed.Sel.A, app.ed.Sel.B = 0, 3

	// Esc should clear selection and keep running
	if !handleEvent(&app, &sdl.KeyboardEvent{
		Type:   sdl.KEYDOWN,
		Repeat: 0,
		Keysym: sdl.Keysym{Sym: sdl.K_ESCAPE},
	}) {
		t.Fatal("escape should not quit when clearing selection")
	}
	if app.ed.Sel.Active {
		t.Fatal("selection should be cleared by escape")
	}

	// Second escape with no selection should be a no-op (not quit)
	if !handleEvent(&app, &sdl.KeyboardEvent{
		Type:   sdl.KEYDOWN,
		Repeat: 0,
		Keysym: sdl.Keysym{Sym: sdl.K_ESCAPE},
	}) {
		t.Fatal("escape should not quit when idle")
	}
}

func TestEscapeClosesPickerBuffer(t *testing.T) {
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "f.txt"), []byte("x"), 0644)
	app := appState{openRoot: root}
	app.initBuffers(editor.NewEditor(""))
	app.addPickerBuffer([]string{"f.txt"})
	if len(app.buffers) != 2 || !app.buffers[app.bufIdx].picker {
		t.Fatalf("expected picker buffer")
	}

	if !handleEvent(&app, &sdl.KeyboardEvent{
		Type:   sdl.KEYDOWN,
		Repeat: 0,
		Keysym: sdl.Keysym{Sym: sdl.K_ESCAPE},
	}) {
		t.Fatal("escape should not quit while closing picker")
	}
	if len(app.buffers) != 1 {
		t.Fatalf("picker buffer should be closed; remaining=%d", len(app.buffers))
	}
}

func TestLeapAgainShortcutsWithCtrlCmd(t *testing.T) {
	app := appState{}
	app.initBuffers(editor.NewEditor("aa aa"))
	app.ed.Leap.LastCommit = []rune("aa")

	// Ctrl+Cmd (RGUI) should leap forward
	sdl.SetModState(sdl.KMOD_CTRL)
	if !handleEvent(&app, &sdl.KeyboardEvent{
		Type:     sdl.KEYDOWN,
		Repeat:   0,
		Keysym:   sdl.Keysym{Scancode: sdl.SCANCODE_RGUI, Sym: sdl.K_RGUI},
		WindowID: 1,
	}) {
		t.Fatal("unexpected quit on Ctrl+Cmd (RGUI)")
	}
	if app.ed.Caret != 3 {
		t.Fatalf("Ctrl+Cmd forward should move to next match; caret=%d", app.ed.Caret)
	}

	// Move back with Ctrl+Cmd (LGUI)
	sdl.SetModState(sdl.KMOD_CTRL)
	if !handleEvent(&app, &sdl.KeyboardEvent{
		Type:     sdl.KEYDOWN,
		Repeat:   0,
		Keysym:   sdl.Keysym{Scancode: sdl.SCANCODE_LGUI, Sym: sdl.K_LGUI},
		WindowID: 1,
	}) {
		t.Fatal("unexpected quit on Ctrl+Cmd (LGUI)")
	}
	if app.ed.Caret != 0 {
		t.Fatalf("Ctrl+Cmd backward should wrap to previous; caret=%d", app.ed.Caret)
	}
}

type stubClip struct {
	text string
}

func (s *stubClip) GetText() (string, error) { return s.text, nil }
func (s *stubClip) SetText(t string) error   { s.text = t; return nil }

func TestClipboardShortcuts(t *testing.T) {
	app := appState{}
	app.initBuffers(editor.NewEditor("copy"))
	sc := &stubClip{}
	app.ed.SetClipboard(sc)

	// Select "copy"
	app.ed.Sel.Active = true
	app.ed.Sel.A, app.ed.Sel.B = 0, 4

	sdl.SetModState(sdl.KMOD_CTRL)
	if !handleEvent(&app, &sdl.KeyboardEvent{
		Type:   sdl.KEYDOWN,
		Repeat: 0,
		Keysym: sdl.Keysym{Sym: sdl.K_c},
	}) {
		t.Fatal("unexpected quit on Ctrl+C")
	}
	if sc.text != "copy" {
		t.Fatalf("clipboard should have copy; got %q", sc.text)
	}

	// Cut
	app.ed.Sel.Active = true
	app.ed.Sel.A, app.ed.Sel.B = 0, 4
	if !handleEvent(&app, &sdl.KeyboardEvent{
		Type:   sdl.KEYDOWN,
		Repeat: 0,
		Keysym: sdl.Keysym{Sym: sdl.K_x},
	}) {
		t.Fatal("unexpected quit on Ctrl+X")
	}
	if string(app.ed.Buf) != "" {
		t.Fatalf("buffer after cut should be empty, got %q", string(app.ed.Buf))
	}

	// Paste
	if !handleEvent(&app, &sdl.KeyboardEvent{
		Type:   sdl.KEYDOWN,
		Repeat: 0,
		Keysym: sdl.Keysym{Sym: sdl.K_v},
	}) {
		t.Fatal("unexpected quit on Ctrl+V")
	}
	if string(app.ed.Buf) != "copy" {
		t.Fatalf("buffer after paste: got %q", string(app.ed.Buf))
	}
}

func TestCtrlShiftSlashOpensHelpBuffer(t *testing.T) {
	app := appState{}
	app.initBuffers(editor.NewEditor("start"))
	sdl.SetModState(sdl.KMOD_CTRL | sdl.KMOD_SHIFT)
	if !handleEvent(&app, &sdl.KeyboardEvent{
		Type:   sdl.KEYDOWN,
		Repeat: 0,
		Keysym: sdl.Keysym{Sym: sdl.K_SLASH, Mod: sdl.KMOD_LSHIFT},
	}) {
		t.Fatal("unexpected quit on Ctrl+Shift+/")
	}
	if len(app.buffers) != 2 {
		t.Fatalf("expected new buffer for help, got %d", len(app.buffers))
	}
	if !strings.Contains(string(app.ed.Buf), "Shortcuts") {
		t.Fatalf("help buffer missing header, got %q", string(app.ed.Buf))
	}
}

func TestCtrlSlashCommentsSelectionAndLine(t *testing.T) {
	app := appState{}
	app.initBuffers(editor.NewEditor("one\ntwo"))

	// Selection over both lines -> comment both
	app.ed.Sel.Active = true
	app.ed.Sel.A, app.ed.Sel.B = 0, len(app.ed.Buf)
	sdl.SetModState(sdl.KMOD_CTRL)
	if !handleEvent(&app, &sdl.KeyboardEvent{
		Type:   sdl.KEYDOWN,
		Repeat: 0,
		Keysym: sdl.Keysym{Sym: sdl.K_SLASH},
	}) {
		t.Fatal("unexpected quit on Ctrl+/")
	}
	if string(app.ed.Buf) != "//one\n//two" {
		t.Fatalf("comment toggle failed: %q", string(app.ed.Buf))
	}
	if !app.ed.Sel.Active {
		t.Fatalf("selection should remain after toggle")
	}

	// With no selection, uncomment current line only (line 2)
	app.ed.Sel.Active = false
	app.ed.Caret = len(app.ed.Buf) // end of buffer (line 2)
	if !handleEvent(&app, &sdl.KeyboardEvent{
		Type:   sdl.KEYDOWN,
		Repeat: 0,
		Keysym: sdl.Keysym{Sym: sdl.K_SLASH},
	}) {
		t.Fatal("unexpected quit on Ctrl+/ second time")
	}
	if string(app.ed.Buf) != "//one\ntwo" {
		t.Fatalf("unexpected buffer after second toggle: %q", string(app.ed.Buf))
	}
}

func TestToggleCommentPreservesSelectionOffsets(t *testing.T) {
	app := appState{}
	app.initBuffers(editor.NewEditor("aa\nbb\ncc"))
	app.ed.Sel.Active = true
	app.ed.Sel.A, app.ed.Sel.B = 3, 5 // select "bb" only

	sdl.SetModState(sdl.KMOD_CTRL)
	if !handleEvent(&app, &sdl.KeyboardEvent{
		Type:   sdl.KEYDOWN,
		Repeat: 0,
		Keysym: sdl.Keysym{Sym: sdl.K_SLASH},
	}) {
		t.Fatal("unexpected quit on Ctrl+/")
	}
	if string(app.ed.Buf) != "aa\n//bb\ncc" {
		t.Fatalf("comment toggle mismatch: %q", string(app.ed.Buf))
	}
	if !app.ed.Sel.Active {
		t.Fatalf("selection should persist")
	}
	a, b := app.ed.Sel.Normalised()
	if a >= b || b <= a {
		t.Fatalf("selection malformed after toggle: (%d,%d)", a, b)
	}
}

// Chaos monkey: randomised but deterministic event stream should never quit or panic.
func TestChaosMonkeyInputDoesNotQuitOrPanic(t *testing.T) {
	ensureSDL(t)
	app := appState{}
	app.initBuffers(editor.NewEditor(""))
	rng := rand.New(rand.NewSource(42))

	for i := range 500 {
		choice := rng.Intn(8)
		switch choice {
		case 0: // text input
			ti := &sdl.TextInputEvent{Type: sdl.TEXTINPUT}
			ti.Text[0] = byte('a' + rng.Intn(26))
			if !handleEvent(&app, ti) {
				t.Fatalf("unexpected quit on text input at iter %d", i)
			}
		case 1: // backspace/delete
			key := sdl.Keycode(sdl.K_BACKSPACE)
			if rng.Intn(2) == 0 {
				key = sdl.Keycode(sdl.K_DELETE)
			}
			if !handleEvent(&app, &sdl.KeyboardEvent{
				Type:   sdl.KEYDOWN,
				Repeat: uint8(rng.Intn(2)),
				Keysym: sdl.Keysym{Sym: key},
			}) {
				t.Fatalf("unexpected quit on backspace/delete at iter %d", i)
			}
		case 2: // arrows
			arrows := []sdl.Keycode{sdl.K_LEFT, sdl.K_RIGHT, sdl.K_UP, sdl.K_DOWN}
			key := arrows[rng.Intn(len(arrows))]
			if !handleEvent(&app, &sdl.KeyboardEvent{
				Type:   sdl.KEYDOWN,
				Repeat: uint8(rng.Intn(2)),
				Keysym: sdl.Keysym{Sym: key},
			}) {
				t.Fatalf("unexpected quit on arrow at iter %d", i)
			}
		case 3: // page up/down
			key := sdl.Keycode(sdl.K_PAGEDOWN)
			if rng.Intn(2) == 0 {
				key = sdl.Keycode(sdl.K_PAGEUP)
			}
			if !handleEvent(&app, &sdl.KeyboardEvent{
				Type:   sdl.KEYDOWN,
				Repeat: 0,
				Keysym: sdl.Keysym{Sym: key},
			}) {
				t.Fatalf("unexpected quit on page key at iter %d", i)
			}
		case 4: // ctrl+u undo
			sdl.SetModState(sdl.KMOD_CTRL)
			if !handleEvent(&app, &sdl.KeyboardEvent{
				Type:   sdl.KEYDOWN,
				Repeat: 0,
				Keysym: sdl.Keysym{Sym: sdl.K_u},
			}) {
				t.Fatalf("unexpected quit on Ctrl+U at iter %d", i)
			}
			sdl.SetModState(0)
		case 5: // enter newline
			if !handleEvent(&app, &sdl.KeyboardEvent{
				Type:   sdl.KEYDOWN,
				Repeat: 0,
				Keysym: sdl.Keysym{Sym: sdl.K_RETURN},
			}) {
				t.Fatalf("unexpected quit on enter at iter %d", i)
			}
		case 6: // leap start + keydown append
			// Start leap forward (Cmd simulation)
			sdl.SetModState(sdl.KMOD_RGUI)
			if !handleEvent(&app, &sdl.KeyboardEvent{
				Type:   sdl.KEYDOWN,
				Repeat: 0,
				Keysym: sdl.Keysym{Scancode: sdl.SCANCODE_RGUI, Sym: sdl.K_RGUI},
			}) {
				t.Fatalf("unexpected quit starting leap at iter %d", i)
			}
			letter := sdl.Keycode(sdl.K_a + rng.Intn(3))
			handleEvent(&app, &sdl.KeyboardEvent{
				Type:   sdl.KEYDOWN,
				Repeat: 0,
				Keysym: sdl.Keysym{Sym: letter},
			})
			// End leap
			sdl.SetModState(0)
			handleEvent(&app, &sdl.KeyboardEvent{
				Type:   sdl.KEYUP,
				Repeat: 0,
				Keysym: sdl.Keysym{Scancode: sdl.SCANCODE_RGUI, Sym: sdl.K_RGUI},
			})
		case 7: // ctrl+s save current (no-op without path)
			sdl.SetModState(sdl.KMOD_CTRL)
			handleEvent(&app, &sdl.KeyboardEvent{
				Type:   sdl.KEYDOWN,
				Repeat: 0,
				Keysym: sdl.Keysym{Sym: sdl.K_s},
			})
			sdl.SetModState(0)
		}
		if app.ed == nil {
			t.Fatalf("editor lost at iter %d", i)
		}
	}
}

func TestShortcutCtrlSSavesAllBuffers(t *testing.T) {
	ensureSDL(t)
	root := t.TempDir()
	a := filepath.Join(root, "a.txt")
	b := filepath.Join(root, "b.txt")
	app := appState{openRoot: root}
	app.initBuffers(editor.NewEditor("A"))
	app.buffers[0].path = a
	app.addBuffer()
	app.buffers[1].path = b
	app.currentPath = b
	app.ed.InsertText("B")

	sdl.SetModState(sdl.KMOD_CTRL | sdl.KMOD_SHIFT)
	if !handleEvent(&app, &sdl.KeyboardEvent{
		Type:   sdl.KEYDOWN,
		Repeat: 0,
		Keysym: sdl.Keysym{Sym: sdl.K_s},
	}) {
		t.Fatal("unexpected quit on Ctrl+Shift+S")
	}

	da, _ := os.ReadFile(a)
	db, _ := os.ReadFile(b)
	if string(da) != "A" || string(db) != "B" {
		t.Fatalf("saved contents mismatch: a=%q b=%q", string(da), string(db))
	}
	// Ensure active buffer restored to last index (second buffer).
	if app.bufIdx != 1 {
		t.Fatalf("expected to stay on last buffer, got idx=%d", app.bufIdx)
	}
}

func TestShortcutCtrlSavesCurrentBufferOnly(t *testing.T) {
	ensureSDL(t)
	root := t.TempDir()
	a := filepath.Join(root, "a.txt")
	b := filepath.Join(root, "b.txt")
	app := appState{openRoot: root}
	app.initBuffers(editor.NewEditor("A"))
	app.buffers[0].path = a
	app.addBuffer()
	app.buffers[1].path = b
	app.currentPath = b
	app.ed.InsertText("B")

	sdl.SetModState(sdl.KMOD_CTRL)
	if !handleEvent(&app, &sdl.KeyboardEvent{
		Type:   sdl.KEYDOWN,
		Repeat: 0,
		Keysym: sdl.Keysym{Sym: sdl.K_s},
	}) {
		t.Fatal("unexpected quit on Ctrl+s")
	}

	// Only active buffer saved
	if data, _ := os.ReadFile(b); string(data) != "B" {
		t.Fatalf("expected active buffer saved; got %q", string(data))
	}
	if _, err := os.Stat(a); err == nil {
		if data, _ := os.ReadFile(a); string(data) == "A" {
			t.Fatalf("first buffer should not be saved by Ctrl+s")
		}
	}
}

func TestShortcutCtrlQBehavior(t *testing.T) {
	ensureSDL(t)
	app := appState{}
	app.initBuffers(editor.NewEditor("first"))
	app.addBuffer()

	sdl.SetModState(sdl.KMOD_CTRL)
	// lower-case: close one buffer but keep running
	if !handleEvent(&app, &sdl.KeyboardEvent{
		Type:   sdl.KEYDOWN,
		Repeat: 0,
		Keysym: sdl.Keysym{Sym: sdl.K_q},
	}) {
		t.Fatal("unexpected quit on Ctrl+q for single buffer close")
	}
	if len(app.buffers) != 1 {
		t.Fatalf("expected one buffer remaining, got %d", len(app.buffers))
	}

	// upper-case: quit immediately
	sdl.SetModState(sdl.KMOD_CTRL | sdl.KMOD_SHIFT)
	if handleEvent(&app, &sdl.KeyboardEvent{
		Type:   sdl.KEYDOWN,
		Repeat: 0,
		Keysym: sdl.Keysym{Sym: sdl.K_q},
	}) {
		t.Fatal("expected quit on Ctrl+Shift+Q")
	}
}

func TestListFilesSkipsHiddenVendor(t *testing.T) {
	root := t.TempDir()
	hidden := filepath.Join(root, ".git")
	vendor := filepath.Join(root, "vendor")
	_ = os.MkdirAll(hidden, 0755)
	_ = os.MkdirAll(vendor, 0755)
	keep := filepath.Join(root, "keep.txt")
	_ = os.WriteFile(keep, []byte("ok"), 0644)
	_ = os.WriteFile(filepath.Join(hidden, "ignore.txt"), []byte("no"), 0644)
	_ = os.WriteFile(filepath.Join(vendor, "ignore.txt"), []byte("no"), 0644)

	files, err := listFiles(root, 10)
	if err != nil {
		t.Fatalf("listFiles: %v", err)
	}
	if len(files) != 1 || files[0] != "keep.txt" {
		t.Fatalf("listFiles should skip hidden/vendor, got %v", files)
	}
}

func TestDefaultPathForBuffers(t *testing.T) {
	app := appState{}
	app.initBuffers(editor.NewEditor(""))
	if got := defaultPath(&app); got != "leap.txt" {
		t.Fatalf("defaultPath first buffer: %q", got)
	}
	app.addBuffer()
	if got := defaultPath(&app); got != "leap-2.txt" {
		t.Fatalf("defaultPath second buffer: %q", got)
	}
}

func TestEndToEndCreateEditSaveFile(t *testing.T) {
	ensureSDL(t)
	root := t.TempDir()
	target := filepath.Join(root, "note.txt")

	app := appState{openRoot: root}
	app.initBuffers(editor.NewEditor(""))
	app.currentPath = target
	app.buffers[app.bufIdx].path = target

	// Type text via TEXTINPUT event
	ti := &sdl.TextInputEvent{Type: sdl.TEXTINPUT}
	copy(ti.Text[:], []byte("hello world\x00"))
	if !handleEvent(&app, ti) {
		t.Fatal("unexpected quit during text input")
	}

	sdl.SetModState(sdl.KMOD_CTRL)
	if !handleEvent(&app, &sdl.KeyboardEvent{
		Type:   sdl.KEYDOWN,
		Repeat: 0,
		Keysym: sdl.Keysym{Sym: sdl.K_w},
	}) {
		t.Fatal("unexpected quit on Ctrl+W")
	}
	sdl.SetModState(0)

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("expected saved file: %v", err)
	}
	if string(data) != "hello world" {
		t.Fatalf("saved contents mismatch: %q", string(data))
	}
	if app.buffers[app.bufIdx].path != target {
		t.Fatalf("buffer path not updated: %q", app.buffers[app.bufIdx].path)
	}
}

// Basic latency guard: ensure per-event processing stays under a small budget so
// interactive typing remains responsive. This is a coarse wall-clock check and
// should remain generous to avoid flakes.
func TestInputLatencyUnderBudget(t *testing.T) {
	ensureSDL(t)
	const budget = 5 * time.Millisecond
	app := appState{}
	app.initBuffers(editor.NewEditor(""))

	for i := range 200 {
		ti := &sdl.TextInputEvent{Type: sdl.TEXTINPUT}
		copy(ti.Text[:], []byte("x\x00"))
		start := time.Now()
		if !handleEvent(&app, ti) {
			t.Fatal("unexpected quit while handling text input")
		}
		if !handleEvent(&app, &sdl.KeyboardEvent{
			Type:   sdl.KEYDOWN,
			Repeat: 0,
			Keysym: sdl.Keysym{Sym: sdl.K_RIGHT},
		}) {
			t.Fatal("unexpected quit while handling keydown")
		}
		if elapsed := time.Since(start); elapsed > budget {
			t.Fatalf("input handling exceeded budget at iter %d: %v > %v", i, elapsed, budget)
		}
	}
}

func TestStartupLoadsFileFromArg(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "start.txt")
	if err := os.WriteFile(path, []byte("hi"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	app := appState{openRoot: root}
	app.initBuffers(editor.NewEditor(""))
	loadStartupFiles(&app, []string{path})
	if string(app.ed.Buf) != "hi" {
		t.Fatalf("buffer: want %q, got %q", "hi", string(app.ed.Buf))
	}
	if app.currentPath != path {
		t.Fatalf("currentPath: want %s, got %s", path, app.currentPath)
	}
	if app.openRoot != filepath.Dir(path) {
		t.Fatalf("openRoot should track file dir: %s", app.openRoot)
	}
}

func TestStartupCreatesMissingFileFromArg(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "missing.txt")

	app := appState{openRoot: root}
	app.initBuffers(editor.NewEditor(""))
	loadStartupFiles(&app, []string{path})

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file to exist: %v", err)
	}
	if string(app.ed.Buf) != "" {
		t.Fatalf("buffer should start empty, got %q", string(app.ed.Buf))
	}
	if app.currentPath != path {
		t.Fatalf("currentPath: want %s, got %s", path, app.currentPath)
	}
}

func TestStartupCreatesParentDir(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "nested", "note.txt")

	app := appState{openRoot: root}
	app.initBuffers(editor.NewEditor(""))
	loadStartupFiles(&app, []string{sub})

	if _, err := os.Stat(sub); err != nil {
		t.Fatalf("expected nested file to exist: %v", err)
	}
	if app.currentPath != sub {
		t.Fatalf("currentPath: want %s, got %s", sub, app.currentPath)
	}
	if app.openRoot != filepath.Dir(sub) {
		t.Fatalf("openRoot should be set to nested dir; got %s", app.openRoot)
	}
}

func TestStartupLoadsMultipleFiles(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "a.txt")
	b := filepath.Join(root, "dir", "b.txt")
	if err := os.MkdirAll(filepath.Dir(b), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(a, []byte("A"), 0644); err != nil {
		t.Fatalf("write a: %v", err)
	}
	if err := os.WriteFile(b, []byte("B"), 0644); err != nil {
		t.Fatalf("write b: %v", err)
	}

	app := appState{openRoot: root}
	app.initBuffers(editor.NewEditor(""))
	loadStartupFiles(&app, []string{a, b})

	if len(app.buffers) != 2 {
		t.Fatalf("expected two buffers, got %d", len(app.buffers))
	}
	if app.currentPath != b {
		t.Fatalf("active buffer should be last file; got %s", app.currentPath)
	}
	// Verify first buffer content/path
	if string(app.buffers[0].ed.Buf) != "A" || app.buffers[0].path != a {
		t.Fatalf("first buffer mismatch: buf=%q path=%s", string(app.buffers[0].ed.Buf), app.buffers[0].path)
	}
	if app.openRoot != filepath.Dir(b) {
		t.Fatalf("openRoot should follow last file dir; got %s", app.openRoot)
	}
}
