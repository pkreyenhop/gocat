package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"fmt"
	"os"
	"time"
	"unicode/utf8"
	"unsafe"

	"sdl-alt-test/editor"

	"github.com/veandco/go-sdl2/sdl"
	"github.com/veandco/go-sdl2/ttf"
)

const Debug = false

type sdlClipboard struct{}

func (sdlClipboard) GetText() (string, error) {
	return sdl.GetClipboardText()
}
func (sdlClipboard) SetText(text string) error {
	return sdl.SetClipboardText(text)
}

type appState struct {
	ed        *editor.Editor
	lastEvent string
	lastMods  sdl.Keymod
	blinkAt   time.Time
	win       *sdl.Window
	lastW     int
	lastH     int
	lastX     int32
	lastY     int32
}

func main() {
	must(sdl.Init(sdl.INIT_VIDEO))
	defer sdl.Quit()

	must(ttf.Init())
	defer ttf.Quit()

	win := mustWindow(sdl.CreateWindow(
		"RustCat Leap Prototype (B: repeat + selection)",
		sdl.WINDOWPOS_CENTERED,
		sdl.WINDOWPOS_CENTERED,
		1200, 780,
		sdl.WINDOW_SHOWN|sdl.WINDOW_RESIZABLE,
	))
	defer win.Destroy()

	ren := mustRenderer(sdl.CreateRenderer(
		win, -1,
		sdl.RENDERER_ACCELERATED|sdl.RENDERER_PRESENTVSYNC,
	))
	defer ren.Destroy()

	font := mustFont(ttf.OpenFont(pickFont(), 18))
	defer font.Close()

	ed := editor.NewEditor(
		"B mode implemented:\n" +
			"- Cmd-only Leap quasimode: hold Right Cmd = forward, hold Left Cmd = back.\n" +
			"- Dual-Leap selection: hold one Cmd, press the other Cmd to start selection.\n" +
			"- Leap Again (repeat): Ctrl+RightCmd / Ctrl+LeftCmd uses last committed pattern.\n" +
			"- Wrap-around when repeating.\n" +
			"- Ctrl+C/Ctrl+X/Ctrl+V clipboard for selection.\n\n" +
			"Type some text below. Try leaping for 'Cmd' or 'selection' etc.\n\n",
	)
	ed.SetClipboard(sdlClipboard{})

	wW, wH := win.GetSize()
	wX, wY := win.GetPosition()
	app := appState{
		ed:      ed,
		blinkAt: time.Now(),
		win:     win,
		lastW:   int(wW),
		lastH:   int(wH),
		lastX:   wX,
		lastY:   wY,
	}

	sdl.StartTextInput()
	defer sdl.StopTextInput()

	running := true
	for running {
		for ev := sdl.PollEvent(); ev != nil; ev = sdl.PollEvent() {
			if !handleEvent(&app, ev) {
				running = false
				break
			}
		}

		render(ren, win, font, &app)
		time.Sleep(2 * time.Millisecond)
	}
}

// handleEvent processes a single SDL event and mutates app/editor state.
// It returns false when the app should quit.
func handleEvent(app *appState, ev sdl.Event) bool {
	ed := app.ed

	switch e := ev.(type) {

	case *sdl.QuitEvent:
		// Ignore quit while in Leap quasimode so Cmd+Q doesn't kill the app mid-leap.
		if ed.Leap.Active {
			return true
		}
		return false

	case *sdl.WindowEvent:
		// Avoid minimizing/hiding the window from system combos during Leap; restore it.
		if (e.Event == sdl.WINDOWEVENT_MINIMIZED || e.Event == sdl.WINDOWEVENT_HIDDEN) && app.win != nil {
			restoreWindow(app)
			return true
		}
		return true

	case *sdl.KeyboardEvent:
		app.blinkAt = time.Now()
		sc := e.Keysym.Scancode
		sym := e.Keysym.Sym
		mods := sdl.GetModState()
		app.lastMods = mods

		// Basic event string
		if e.Type == sdl.KEYDOWN {
			app.lastEvent = fmt.Sprintf("KEYDOWN sc=%s key=%s repeat=%d mods=%s",
				sdl.GetScancodeName(sc), sdl.GetKeyName(sym), e.Repeat, modsString(mods))
		} else {
			app.lastEvent = fmt.Sprintf("KEYUP   sc=%s key=%s mods=%s",
				sdl.GetScancodeName(sc), sdl.GetKeyName(sym), modsString(mods))
		}
		if Debug {
			fmt.Println(app.lastEvent)
		}

		// Quit (only when not leaping)
		if e.Type == sdl.KEYDOWN && e.Repeat == 0 && sym == sdl.K_ESCAPE && !ed.Leap.Active {
			return false
		}

		// Clipboard ops on Ctrl (not Cmd, because Cmd is Leap keys)
		if e.Type == sdl.KEYDOWN && e.Repeat == 0 {
			ctrlHeld := (mods & sdl.KMOD_CTRL) != 0
			if ctrlHeld {
				switch sym {
				case sdl.K_c:
					ed.CopySelection()
					return true
				case sdl.K_x:
					ed.CutSelection()
					return true
				case sdl.K_v:
					ed.PasteClipboard()
					return true
				}
			}
		}

		// Leap Again: Ctrl + Cmd (Left or Right) without entering quasimode typing
		// We trigger on KEYDOWN of LGUI/RGUI while Ctrl is held and leap is NOT active.
		if e.Type == sdl.KEYDOWN && e.Repeat == 0 && !ed.Leap.Active {
			ctrlHeld := (mods & sdl.KMOD_CTRL) != 0
			if ctrlHeld {
				if sc == sdl.SCANCODE_RGUI {
					ed.LeapAgain(editor.DirFwd)
					return true
				}
				if sc == sdl.SCANCODE_LGUI {
					ed.LeapAgain(editor.DirBack)
					return true
				}
			}
		}

		// Track held Cmd state
		if e.Type == sdl.KEYDOWN && e.Repeat == 0 {
			if sc == sdl.SCANCODE_LGUI {
				ed.Leap.HeldL = true
			}
			if sc == sdl.SCANCODE_RGUI {
				ed.Leap.HeldR = true
			}
		}
		if e.Type == sdl.KEYUP {
			if sc == sdl.SCANCODE_LGUI {
				ed.Leap.HeldL = false
			}
			if sc == sdl.SCANCODE_RGUI {
				ed.Leap.HeldR = false
			}
		}

		// Start Leap quasimode when first Cmd goes down (and Ctrl is NOT held)
		if e.Type == sdl.KEYDOWN && e.Repeat == 0 && !ed.Leap.Active {
			ctrlHeld := (mods & sdl.KMOD_CTRL) != 0
			if !ctrlHeld {
				if sc == sdl.SCANCODE_RGUI {
					ed.LeapStart(editor.DirFwd)
					beginLeapGrab(app)
					return true
				}
				if sc == sdl.SCANCODE_LGUI {
					ed.LeapStart(editor.DirBack)
					beginLeapGrab(app)
					return true
				}
			}
		}

		// While leaping, if the OTHER Cmd is pressed, enable selection mode + anchor
		if ed.Leap.Active && e.Type == sdl.KEYDOWN && e.Repeat == 0 {
			if (sc == sdl.SCANCODE_LGUI && ed.Leap.HeldR) || (sc == sdl.SCANCODE_RGUI && ed.Leap.HeldL) {
				ed.BeginLeapSelection()
			}
		}

		// End Leap when BOTH Cmd keys are up
		if e.Type == sdl.KEYUP && ed.Leap.Active {
			if !ed.Leap.HeldL && !ed.Leap.HeldR {
				ed.LeapEndCommit()
				endLeapGrab(app)
				return true
			}
		}

		// While leaping: lifecycle keys and KEYDOWN fallback for pattern capture
		if ed.Leap.Active && e.Type == sdl.KEYDOWN && e.Repeat == 0 {
			switch sym {
			case sdl.K_ESCAPE:
				ed.LeapCancel()
				endLeapGrab(app)
				return true
			case sdl.K_BACKSPACE:
				ed.LeapBackspace()
				return true
			case sdl.K_RETURN, sdl.K_KP_ENTER:
				ed.LeapEndCommit()
				endLeapGrab(app)
				return true
			}

			// KEYDOWN fallback capture (Cmd suppresses TEXTINPUT on macOS)
			if r, ok := keyToRune(sym, mods); ok {
				ed.Leap.LastSrc = "keydown"
				ed.LeapAppend(string(r))
				return true
			}
		}

		// Normal editing (outside leap)
		if !ed.Leap.Active && e.Type == sdl.KEYDOWN && e.Repeat == 0 {
			switch sym {
			case sdl.K_BACKSPACE:
				ed.BackspaceOrDeleteSelection(true)
			case sdl.K_DELETE:
				ed.BackspaceOrDeleteSelection(false)
			case sdl.K_LEFT:
				ed.MoveCaret(-1, (mods&sdl.KMOD_SHIFT) != 0)
			case sdl.K_RIGHT:
				ed.MoveCaret(1, (mods&sdl.KMOD_SHIFT) != 0)
			case sdl.K_RETURN, sdl.K_KP_ENTER:
				ed.InsertText("\n")
			}
		}

	case *sdl.TextInputEvent:
		app.blinkAt = time.Now()
		text := textInputString(e)
		app.lastEvent = fmt.Sprintf("TEXTINPUT %q mods=%s", text, modsString(sdl.GetModState()))
		if Debug {
			fmt.Println(app.lastEvent)
		}

		if text == "" || !utf8.ValidString(text) {
			return true
		}

		if ed.Leap.Active {
			ed.Leap.LastSrc = "textinput"
			ed.LeapAppend(text)
		} else {
			ed.InsertText(text)
		}
	}

	return true
}

func restoreWindow(app *appState) {
	if app.win == nil {
		return
	}
	app.win.Restore()
	app.win.Show()
	if app.lastW > 0 && app.lastH > 0 {
		app.win.SetSize(int32(app.lastW), int32(app.lastH))
	}
	if app.lastX != 0 || app.lastY != 0 {
		app.win.SetPosition(app.lastX, app.lastY)
	}
	app.win.Raise()
}

func beginLeapGrab(app *appState) {
	if app.win == nil {
		return
	}
	app.win.SetKeyboardGrab(true)
	app.win.SetAlwaysOnTop(true)
	app.win.Raise()
}

func endLeapGrab(app *appState) {
	if app.win == nil {
		return
	}
	app.win.SetKeyboardGrab(false)
	app.win.SetAlwaysOnTop(false)
}

// ======================
// Rendering
// ======================

func render(r *sdl.Renderer, win *sdl.Window, font *ttf.Font, app *appState) {
	_, h32 := win.GetSize()
	h := int(h32)
	if app != nil && app.win != nil {
		x, y := win.GetPosition()
		w, hh := win.GetSize()
		app.lastX = x
		app.lastY = y
		app.lastW = int(w)
		app.lastH = int(hh)
	}

	bg := sdl.Color{R: 20, G: 20, B: 24, A: 255}
	fg := sdl.Color{R: 230, G: 230, B: 235, A: 255}
	dim := sdl.Color{R: 160, G: 160, B: 170, A: 255}
	green := sdl.Color{R: 120, G: 220, B: 140, A: 255}
	blue := sdl.Color{R: 120, G: 170, B: 255, A: 255}
	orange := sdl.Color{R: 255, G: 170, B: 120, A: 255}
	selCol := sdl.Color{R: 60, G: 90, B: 140, A: 255}
	caretCol := sdl.Color{R: 255, G: 255, B: 180, A: 255}

	r.SetDrawColor(bg.R, bg.G, bg.B, bg.A)
	r.Clear()

	cellW, _, _ := font.SizeUTF8("M")
	lineH := font.Height() + 4
	left := 12
	top := 12

	// Blink caret: visible for 650ms, hidden for 350ms. Reset on input.
	blinkOn := true
	if app.blinkAt.IsZero() {
		app.blinkAt = time.Now()
	}
	elapsedMs := time.Since(app.blinkAt).Milliseconds()
	if elapsedMs%1000 >= 650 {
		blinkOn = false
	}

	lines := editor.SplitLines(app.ed.Buf)

	// Status lines
	if app.ed.Leap.Active {
		col := blue
		dirArrow := "→"
		if app.ed.Leap.Dir == editor.DirBack {
			col = orange
			dirArrow = "←"
		}
		drawText(r, font, left, top,
			fmt.Sprintf("LEAP %s heldL=%v heldR=%v selecting=%v src=%s query=%q last=%q",
				dirArrow, app.ed.Leap.HeldL, app.ed.Leap.HeldR, app.ed.Leap.Selecting, app.ed.Leap.LastSrc,
				string(app.ed.Leap.Query), string(app.ed.Leap.LastCommit)),
			col,
		)
	} else {
		drawText(r, font, left, top,
			fmt.Sprintf("EDIT  (Cmd-only Leap. Ctrl+Cmd = Leap Again. Ctrl+C/X/V clipboard)  last=%q",
				string(app.ed.Leap.LastCommit)),
			dim,
		)
	}
	drawText(r, font, left, top+lineH+2, app.lastEvent, dim)

	// Text start
	y0 := top + (lineH * 2) + 12
	y := y0

	// Draw selection background (monospace-based)
	if app.ed.Sel.Active {
		a, b := app.ed.Sel.Normalised()
		a = clamp(a, 0, len(app.ed.Buf))
		b = clamp(b, 0, len(app.ed.Buf))
		drawSelectionRects(r, lines, app.ed.Buf, a, b, left, y0, lineH, cellW, selCol)
	}

	// Caret position in line/col space
	cLine := editor.CaretLineAt(lines, app.ed.Caret)
	cCol := editor.CaretColAt(lines, app.ed.Caret)

	for i, line := range lines {
		drawText(r, font, left, y, line, fg)

		if i == cLine && blinkOn {
			x := left + cCol*cellW
			w := maxInt(2, cellW/3)
			r.SetDrawColor(caretCol.R, caretCol.G, caretCol.B, caretCol.A)
			_ = r.FillRect(&sdl.Rect{
				X: int32(x),
				Y: int32(y),
				W: int32(w),
				H: int32(lineH - 2),
			})
		}

		y += lineH
		if y > h-60 {
			break
		}
	}

	// underline found position while leaping
	if app.ed.Leap.Active && app.ed.Leap.LastFoundPos >= 0 {
		fLine := editor.CaretLineAt(lines, app.ed.Leap.LastFoundPos)
		fCol := editor.CaretColAt(lines, app.ed.Leap.LastFoundPos)
		yFound := y0 + fLine*lineH
		if yFound >= top && yFound <= h-60 {
			x := left + fCol*cellW
			yy := yFound + lineH - 3
			r.SetDrawColor(green.R, green.G, green.B, green.A)
			_ = r.DrawLine(int32(x), int32(yy), int32(x+cellW), int32(yy))
		}
	}

	r.Present()
}

func drawSelectionRects(r *sdl.Renderer, lines []string, buf []rune, a, b int, left, y0, lineH, cellW int, col sdl.Color) {
	aLine, aCol := editor.LineColForPos(lines, a)
	bLine, bCol := editor.LineColForPos(lines, b)

	r.SetDrawColor(col.R, col.G, col.B, col.A)

	if aLine == bLine {
		x1 := left + aCol*cellW
		x2 := left + bCol*cellW
		if x2 < x1 {
			x1, x2 = x2, x1
		}
		_ = r.FillRect(&sdl.Rect{X: int32(x1), Y: int32(y0 + aLine*lineH), W: int32(maxInt(2, x2-x1)), H: int32(lineH)})
		return
	}

	// First line: from aCol to end of line
	firstLen := len([]rune(lines[aLine]))
	x1 := left + aCol*cellW
	x2 := left + firstLen*cellW
	_ = r.FillRect(&sdl.Rect{X: int32(x1), Y: int32(y0 + aLine*lineH), W: int32(maxInt(2, x2-x1)), H: int32(lineH)})

	// Middle full lines
	for ln := aLine + 1; ln < bLine; ln++ {
		lineLen := len([]rune(lines[ln]))
		_ = r.FillRect(&sdl.Rect{X: int32(left), Y: int32(y0 + ln*lineH), W: int32(maxInt(2, lineLen*cellW)), H: int32(lineH)})
	}

	// Last line: from start to bCol
	x3 := left
	x4 := left + bCol*cellW
	_ = r.FillRect(&sdl.Rect{X: int32(x3), Y: int32(y0 + bLine*lineH), W: int32(maxInt(2, x4-x3)), H: int32(lineH)})

	_ = buf // keep signature stable if you later want buffer-aware selection
}

func drawText(r *sdl.Renderer, font *ttf.Font, x, y int, text string, col sdl.Color) {
	if text == "" {
		return
	}
	surf, err := font.RenderUTF8Blended(text, col)
	if err != nil {
		return
	}
	defer surf.Free()

	tex, err := r.CreateTextureFromSurface(surf)
	if err != nil {
		return
	}
	defer tex.Destroy()

	dst := sdl.Rect{X: int32(x), Y: int32(y), W: surf.W, H: surf.H}
	_ = r.Copy(tex, nil, &dst)
}

// ======================
// Input helpers
// ======================

func textInputString(e *sdl.TextInputEvent) string {
	return C.GoString((*C.char)(unsafe.Pointer(&e.Text[0])))
}

// KEYDOWN fallback capture (US-ish layout) for when Cmd suppresses TEXTINPUT.
func keyToRune(sym sdl.Keycode, mods sdl.Keymod) (rune, bool) {
	shift := (mods & sdl.KMOD_SHIFT) != 0

	// letters
	if sym >= sdl.K_a && sym <= sdl.K_z {
		r := rune('a' + (sym - sdl.K_a))
		if shift {
			r = rune('A' + (sym - sdl.K_a))
		}
		return r, true
	}

	// digits
	if sym >= sdl.K_0 && sym <= sdl.K_9 {
		// (no shifted symbols for simplicity)
		r := rune('0' + (sym - sdl.K_0))
		return r, true
	}

	// some punctuation
	switch sym {
	case sdl.K_SPACE:
		return ' ', true
	case sdl.K_PERIOD:
		if shift {
			return '>', true
		}
		return '.', true
	case sdl.K_COMMA:
		if shift {
			return '<', true
		}
		return ',', true
	case sdl.K_MINUS:
		if shift {
			return '_', true
		}
		return '-', true
	case sdl.K_EQUALS:
		if shift {
			return '+', true
		}
		return '=', true
	case sdl.K_SLASH:
		if shift {
			return '?', true
		}
		return '/', true
	}

	return 0, false
}

func modsString(m sdl.Keymod) string {
	parts := ""
	add := func(s string) {
		if parts != "" {
			parts += "|"
		}
		parts += s
	}
	if (m & sdl.KMOD_LSHIFT) != 0 {
		add("LSHIFT")
	}
	if (m & sdl.KMOD_RSHIFT) != 0 {
		add("RSHIFT")
	}
	if (m & sdl.KMOD_LCTRL) != 0 {
		add("LCTRL")
	}
	if (m & sdl.KMOD_RCTRL) != 0 {
		add("RCTRL")
	}
	if (m & sdl.KMOD_LGUI) != 0 {
		add("LCMD")
	}
	if (m & sdl.KMOD_RGUI) != 0 {
		add("RCMD")
	}
	if (m & sdl.KMOD_LALT) != 0 {
		add("LALT")
	}
	if (m & sdl.KMOD_RALT) != 0 {
		add("RALT")
	}
	if parts == "" {
		return "none"
	}
	return parts
}

// ======================
// Fonts + util
// ======================

func pickFont() string {
	candidates := []string{
		"/System/Library/Fonts/Menlo.ttc",
		"/Library/Fonts/Menlo.ttc",
		"/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf",
		"/usr/share/fonts/truetype/liberation/LiberationMono-Regular.ttf",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	local := "DejaVuSansMono.ttf"
	if _, err := os.Stat(local); err == nil {
		return local
	}
	panic("No usable mono font found. Install DejaVu/Liberation or place DejaVuSansMono.ttf next to main.go")
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func mustWindow(w *sdl.Window, err error) *sdl.Window {
	if err != nil {
		panic(err)
	}
	return w
}
func mustRenderer(r *sdl.Renderer, err error) *sdl.Renderer {
	if err != nil {
		panic(err)
	}
	return r
}
func mustFont(f *ttf.Font, err error) *ttf.Font {
	if err != nil {
		panic(err)
	}
	return f
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
