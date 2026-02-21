//go:build gui

package main

import (
	"os"
	"testing"

	"sdl-alt-test/editor"

	"github.com/veandco/go-sdl2/sdl"
	"github.com/veandco/go-sdl2/ttf"
)

// GUI smoke test: renders a small buffer using the real SDL renderer. This runs
// only with the "gui" build tag so it doesn't block headless CI; it will skip
// if the SDL dummy driver or fonts are unavailable.
func TestRenderSmoke(t *testing.T) {
	_ = os.Setenv("SDL_VIDEODRIVER", "dummy") // avoid opening a real window when possible

	if err := sdl.Init(sdl.INIT_VIDEO); err != nil {
		t.Skipf("skip: SDL init failed (%v)", err)
	}
	defer sdl.Quit()

	if err := ttf.Init(); err != nil {
		t.Skipf("skip: TTF init failed (%v)", err)
	}
	defer ttf.Quit()

	win, ren, err := sdl.CreateWindowAndRenderer(320, 240, sdl.WINDOW_HIDDEN)
	if err != nil {
		t.Skipf("skip: create window/renderer failed (%v)", err)
	}
	defer win.Destroy()
	defer ren.Destroy()

	fontPath := pickFont()
	font, err := ttf.OpenFont(fontPath, 14)
	if err != nil {
		t.Skipf("skip: open font %q failed (%v)", fontPath, err)
	}
	defer font.Close()

	app := appState{
		ed:        editor.NewEditor("gui test\nsecond line"),
		lastEvent: "test",
	}
	render(ren, win, font, &app)
}

// GUI input: holding Cmd and pressing "h" should feed the leap query via KEYDOWN
// fallback (macOS suppresses TEXTINPUT when Cmd is held). This ensures the UI
// driver matches headless leap behaviour.
func TestLeapKeydownCapture(t *testing.T) {
	_ = os.Setenv("SDL_VIDEODRIVER", "dummy")

	if err := sdl.Init(sdl.INIT_VIDEO); err != nil {
		t.Skipf("skip: SDL init failed (%v)", err)
	}
	defer sdl.Quit()

	app := appState{
		ed: editor.NewEditor("xxhxx"),
	}

	// Press Cmd (RGUI) to start leap
	sdl.SetModState(sdl.KMOD_RGUI)
	if !handleEvent(&app, &sdl.KeyboardEvent{
		Type:     sdl.KEYDOWN,
		Repeat:   0,
		Keysym:   sdl.Keysym{Scancode: sdl.SCANCODE_RGUI, Sym: sdl.K_RGUI},
		WindowID: 1,
	}) {
		t.Fatal("unexpected quit on Cmd down")
	}
	if !app.ed.Leap.Active {
		t.Fatalf("leap should be active after Cmd down")
	}

	// Press "h" while Cmd held: should append via keyToRune fallback and move caret to match.
	if !handleEvent(&app, &sdl.KeyboardEvent{
		Type:     sdl.KEYDOWN,
		Repeat:   0,
		Keysym:   sdl.Keysym{Scancode: sdl.SCANCODE_H, Sym: sdl.K_h},
		WindowID: 1,
	}) {
		t.Fatal("unexpected quit on h down")
	}

	if got := string(app.ed.Leap.Query); got != "h" {
		t.Fatalf("leap query: want %q, got %q", "h", got)
	}
	if app.ed.Leap.LastSrc != "keydown" {
		t.Fatalf("leap lastSrc: want %q, got %q", "keydown", app.ed.Leap.LastSrc)
	}
	if app.ed.Caret != 2 {
		t.Fatalf("caret: want 2 (first h), got %d", app.ed.Caret)
	}

	// Release Cmd to commit
	sdl.SetModState(0)
	if !handleEvent(&app, &sdl.KeyboardEvent{
		Type:     sdl.KEYUP,
		Repeat:   0,
		Keysym:   sdl.Keysym{Scancode: sdl.SCANCODE_RGUI, Sym: sdl.K_RGUI},
		WindowID: 1,
	}) {
		t.Fatal("unexpected quit on Cmd up")
	}
	if app.ed.Leap.Active {
		t.Fatalf("leap should end after Cmd up")
	}
	if got := string(app.ed.Leap.LastCommit); got != "h" {
		t.Fatalf("last commit: want %q, got %q", "h", got)
	}
}

// GUI input: Cmd+Q should behave like any other letter in Leap and must not
// close the app. We simulate a QuitEvent arriving mid-leap and ensure it is
// ignored so Leap can commit normally.
func TestLeapCmdQDoesNotQuit(t *testing.T) {
	_ = os.Setenv("SDL_VIDEODRIVER", "dummy")
	if err := sdl.Init(sdl.INIT_VIDEO); err != nil {
		t.Skipf("skip: SDL init failed (%v)", err)
	}
	defer sdl.Quit()

	app := appState{ed: editor.NewEditor("qq hello")}

	// Start leap with Cmd
	sdl.SetModState(sdl.KMOD_RGUI)
	if !handleEvent(&app, &sdl.KeyboardEvent{
		Type:     sdl.KEYDOWN,
		Repeat:   0,
		Keysym:   sdl.Keysym{Scancode: sdl.SCANCODE_RGUI, Sym: sdl.K_RGUI},
		WindowID: 1,
	}) {
		t.Fatal("unexpected quit on Cmd down")
	}

	// Send Cmd+Q (keydown) to append query
	if !handleEvent(&app, &sdl.KeyboardEvent{
		Type:     sdl.KEYDOWN,
		Repeat:   0,
		Keysym:   sdl.Keysym{Scancode: sdl.SCANCODE_Q, Sym: sdl.K_q},
		WindowID: 1,
	}) {
		t.Fatal("unexpected quit on Cmd+Q keydown")
	}
	if got := string(app.ed.Leap.Query); got != "q" {
		t.Fatalf("leap query: want %q, got %q", "q", got)
	}

	// A QuitEvent arriving while leaping should be ignored.
	if !handleEvent(&app, &sdl.QuitEvent{Type: sdl.QUIT}) {
		t.Fatal("quit event should be ignored during leap")
	}

	// Release Cmd to commit
	sdl.SetModState(0)
	if !handleEvent(&app, &sdl.KeyboardEvent{
		Type:     sdl.KEYUP,
		Repeat:   0,
		Keysym:   sdl.Keysym{Scancode: sdl.SCANCODE_RGUI, Sym: sdl.K_RGUI},
		WindowID: 1,
	}) {
		t.Fatal("unexpected quit on Cmd up")
	}

	if got := string(app.ed.Leap.LastCommit); got != "q" {
		t.Fatalf("last commit: want %q, got %q", "q", got)
	}
}

// GUI input: Cmd+H should be captured by Leap (macOS hides windows on Cmd+H).
// We ensure the query appends and the app keeps running.
func TestLeapCmdHDoesNotHide(t *testing.T) {
	_ = os.Setenv("SDL_VIDEODRIVER", "dummy")
	if err := sdl.Init(sdl.INIT_VIDEO); err != nil {
		t.Skipf("skip: SDL init failed (%v)", err)
	}
	defer sdl.Quit()

	app := appState{ed: editor.NewEditor("hhhat")}

	sdl.SetModState(sdl.KMOD_RGUI)
	handleEvent(&app, &sdl.KeyboardEvent{
		Type:     sdl.KEYDOWN,
		Repeat:   0,
		Keysym:   sdl.Keysym{Scancode: sdl.SCANCODE_RGUI, Sym: sdl.K_RGUI},
		WindowID: 1,
	})

	if !handleEvent(&app, &sdl.KeyboardEvent{
		Type:     sdl.KEYDOWN,
		Repeat:   0,
		Keysym:   sdl.Keysym{Scancode: sdl.SCANCODE_H, Sym: sdl.K_h},
		WindowID: 1,
	}) {
		t.Fatal("unexpected quit on Cmd+H")
	}

	if got := string(app.ed.Leap.Query); got != "h" {
		t.Fatalf("leap query: want %q, got %q", "h", got)
	}
	if app.ed.Caret != 0 {
		t.Fatalf("caret: want 0, got %d", app.ed.Caret)
	}

	// Simulate hide event; should be ignored/undone.
	if !handleEvent(&app, &sdl.WindowEvent{Event: sdl.WINDOWEVENT_HIDDEN}) {
		t.Fatal("unexpected quit on hide")
	}
}

// GUI input: Cmd+M normally minimizes the window. Ensure we ignore the window
// minimize event and still capture the Leap query.
func TestLeapCmdMDoesNotMinimize(t *testing.T) {
	_ = os.Setenv("SDL_VIDEODRIVER", "dummy")
	if err := sdl.Init(sdl.INIT_VIDEO); err != nil {
		t.Skipf("skip: SDL init failed (%v)", err)
	}
	defer sdl.Quit()

	win, _, err := sdl.CreateWindowAndRenderer(200, 100, sdl.WINDOW_HIDDEN)
	if err != nil {
		t.Skipf("skip: cannot create window (%v)", err)
	}
	defer win.Destroy()

	app := appState{ed: editor.NewEditor("mmm"), win: win, lastW: 200, lastH: 100}

	sdl.SetModState(sdl.KMOD_RGUI)
	handleEvent(&app, &sdl.KeyboardEvent{
		Type:     sdl.KEYDOWN,
		Repeat:   0,
		Keysym:   sdl.Keysym{Scancode: sdl.SCANCODE_RGUI, Sym: sdl.K_RGUI},
		WindowID: 1,
	})

	if !handleEvent(&app, &sdl.KeyboardEvent{
		Type:     sdl.KEYDOWN,
		Repeat:   0,
		Keysym:   sdl.Keysym{Scancode: sdl.SCANCODE_M, Sym: sdl.K_m},
		WindowID: 1,
	}) {
		t.Fatal("unexpected quit on Cmd+M keydown")
	}

	// Simulate minimize window event arriving from OS; handler should restore and keep running.
	if !handleEvent(&app, &sdl.WindowEvent{Event: sdl.WINDOWEVENT_MINIMIZED}) {
		t.Fatal("unexpected quit on window minimize")
	}

	if got := string(app.ed.Leap.Query); got != "m" {
		t.Fatalf("leap query: want %q, got %q", "m", got)
	}

	// Verify window size preserved
	w, h := win.GetSize()
	if int(w) != 200 || int(h) != 100 {
		t.Fatalf("window size changed after minimize handling: got %dx%d", w, h)
	}

	flags := win.GetFlags()
	if flags&sdl.WINDOW_MINIMIZED != 0 {
		t.Fatalf("window remained minimized after handling Cmd+M (flags=%x)", flags)
	}
}
