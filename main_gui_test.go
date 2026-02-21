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
