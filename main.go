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

	"github.com/veandco/go-sdl2/sdl"
	"github.com/veandco/go-sdl2/ttf"
)

const Debug = false

type Dir int

const (
	DirBack Dir = -1
	DirFwd  Dir = 1
)

type Sel struct {
	active bool
	a      int // inclusive
	b      int // exclusive-ish in rendering; we normalise anyway
}

func (s Sel) normalised() (int, int) {
	if !s.active {
		return 0, 0
	}
	if s.a <= s.b {
		return s.a, s.b
	}
	return s.b, s.a
}

type LeapState struct {
	active       bool
	dir          Dir
	query        []rune
	originCaret  int
	lastFoundPos int

	heldL bool
	heldR bool

	// Selection while both Leap keys are involved
	selecting  bool
	selAnchor  int
	lastSrc    string // "textinput" or "keydown"
	lastCommit []rune // last committed query for Leap Again
}

type Editor struct {
	buf   []rune
	caret int
	sel   Sel
	leap  LeapState

	lastEvent string
	lastMods  sdl.Keymod
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

	ed := &Editor{
		buf: []rune(
			"B mode implemented:\n" +
				"- Cmd-only Leap quasimode: hold Right Cmd = forward, hold Left Cmd = back.\n" +
				"- Dual-Leap selection: hold one Cmd, press the other Cmd to start selection.\n" +
				"- Leap Again (repeat): Ctrl+RightCmd / Ctrl+LeftCmd uses last committed pattern.\n" +
				"- Wrap-around when repeating.\n" +
				"- Ctrl+C/Ctrl+X/Ctrl+V clipboard for selection.\n\n" +
				"Type some text below. Try leaping for 'Cmd' or 'selection' etc.\n\n",
		),
		leap: LeapState{lastFoundPos: -1},
	}

	sdl.StartTextInput()
	defer sdl.StopTextInput()

	running := true
	for running {
		for ev := sdl.PollEvent(); ev != nil; ev = sdl.PollEvent() {
			switch e := ev.(type) {

			case *sdl.QuitEvent:
				running = false

			case *sdl.KeyboardEvent:
				sc := e.Keysym.Scancode
				sym := e.Keysym.Sym
				mods := sdl.GetModState()
				ed.lastMods = mods

				// Basic event string
				if e.Type == sdl.KEYDOWN {
					ed.lastEvent = fmt.Sprintf("KEYDOWN sc=%s key=%s repeat=%d mods=%s",
						sdl.GetScancodeName(sc), sdl.GetKeyName(sym), e.Repeat, modsString(mods))
				} else {
					ed.lastEvent = fmt.Sprintf("KEYUP   sc=%s key=%s mods=%s",
						sdl.GetScancodeName(sc), sdl.GetKeyName(sym), modsString(mods))
				}
				if Debug {
					fmt.Println(ed.lastEvent)
				}

				// Quit (only when not leaping)
				if e.Type == sdl.KEYDOWN && e.Repeat == 0 && sym == sdl.K_ESCAPE && !ed.leap.active {
					running = false
					continue
				}

				// Clipboard ops on Ctrl (not Cmd, because Cmd is Leap keys)
				if e.Type == sdl.KEYDOWN && e.Repeat == 0 {
					ctrlHeld := (mods & sdl.KMOD_CTRL) != 0
					if ctrlHeld {
						switch sym {
						case sdl.K_c:
							ed.copySelection()
							continue
						case sdl.K_x:
							ed.cutSelection()
							continue
						case sdl.K_v:
							ed.pasteClipboard()
							continue
						}
					}
				}

				// Leap Again: Ctrl + Cmd (Left or Right) without entering quasimode typing
				// We trigger on KEYDOWN of LGUI/RGUI while Ctrl is held and leap is NOT active.
				if e.Type == sdl.KEYDOWN && e.Repeat == 0 && !ed.leap.active {
					ctrlHeld := (mods & sdl.KMOD_CTRL) != 0
					if ctrlHeld {
						if sc == sdl.SCANCODE_RGUI {
							ed.leapAgain(DirFwd)
							continue
						}
						if sc == sdl.SCANCODE_LGUI {
							ed.leapAgain(DirBack)
							continue
						}
					}
				}

				// Track held Cmd state
				if e.Type == sdl.KEYDOWN && e.Repeat == 0 {
					if sc == sdl.SCANCODE_LGUI {
						ed.leap.heldL = true
					}
					if sc == sdl.SCANCODE_RGUI {
						ed.leap.heldR = true
					}
				}
				if e.Type == sdl.KEYUP {
					if sc == sdl.SCANCODE_LGUI {
						ed.leap.heldL = false
					}
					if sc == sdl.SCANCODE_RGUI {
						ed.leap.heldR = false
					}
				}

				// Start Leap quasimode when first Cmd goes down (and Ctrl is NOT held)
				if e.Type == sdl.KEYDOWN && e.Repeat == 0 && !ed.leap.active {
					ctrlHeld := (mods & sdl.KMOD_CTRL) != 0
					if !ctrlHeld {
						if sc == sdl.SCANCODE_RGUI {
							ed.leapStart(DirFwd)
							continue
						}
						if sc == sdl.SCANCODE_LGUI {
							ed.leapStart(DirBack)
							continue
						}
					}
				}

				// While leaping, if the OTHER Cmd is pressed, enable selection mode + anchor
				if ed.leap.active && e.Type == sdl.KEYDOWN && e.Repeat == 0 {
					if (sc == sdl.SCANCODE_LGUI && ed.leap.heldR) || (sc == sdl.SCANCODE_RGUI && ed.leap.heldL) {
						// We now have both held; start selecting if not already.
						if !ed.leap.selecting {
							ed.leap.selecting = true
							ed.leap.selAnchor = ed.caret
							ed.sel.active = true
							ed.sel.a = ed.leap.selAnchor
							ed.sel.b = ed.caret
							if Debug {
								fmt.Printf("SELECTION START anchor=%d\n", ed.leap.selAnchor)
							}
						}
					}
				}

				// End Leap when BOTH Cmd keys are up
				if e.Type == sdl.KEYUP && ed.leap.active {
					if !ed.leap.heldL && !ed.leap.heldR {
						ed.leapEndCommit()
						continue
					}
				}

				// While leaping: lifecycle keys and KEYDOWN fallback for pattern capture
				if ed.leap.active && e.Type == sdl.KEYDOWN && e.Repeat == 0 {
					switch sym {
					case sdl.K_ESCAPE:
						ed.leapCancel()
						continue
					case sdl.K_BACKSPACE:
						ed.leapBackspace()
						continue
					case sdl.K_RETURN, sdl.K_KP_ENTER:
						ed.leapEndCommit()
						continue
					}

					// KEYDOWN fallback capture (Cmd suppresses TEXTINPUT on macOS)
					if r, ok := keyToRune(sym, mods); ok {
						ed.leap.lastSrc = "keydown"
						ed.leapAppend(string(r))
						continue
					}
				}

				// Normal editing (outside leap)
				if !ed.leap.active && e.Type == sdl.KEYDOWN && e.Repeat == 0 {
					switch sym {
					case sdl.K_BACKSPACE:
						ed.backspaceOrDeleteSelection(true)
					case sdl.K_DELETE:
						ed.backspaceOrDeleteSelection(false)
					case sdl.K_LEFT:
						ed.moveCaret(-1, (mods&sdl.KMOD_SHIFT) != 0)
					case sdl.K_RIGHT:
						ed.moveCaret(1, (mods&sdl.KMOD_SHIFT) != 0)
					case sdl.K_RETURN, sdl.K_KP_ENTER:
						ed.insertText("\n")
					}
				}

			case *sdl.TextInputEvent:
				text := textInputString(e)
				ed.lastEvent = fmt.Sprintf("TEXTINPUT %q mods=%s", text, modsString(sdl.GetModState()))
				if Debug {
					fmt.Println(ed.lastEvent)
				}

				if text == "" || !utf8.ValidString(text) {
					continue
				}

				if ed.leap.active {
					ed.leap.lastSrc = "textinput"
					ed.leapAppend(text)
				} else {
					ed.insertText(text)
				}
			}
		}

		render(ren, win, font, ed)
		time.Sleep(2 * time.Millisecond)
	}
}

// ======================
// Leap + selection logic
// ======================

func (e *Editor) leapStart(dir Dir) {
	e.leap.active = true
	e.leap.dir = dir
	e.leap.originCaret = e.caret
	e.leap.query = e.leap.query[:0]
	e.leap.lastFoundPos = -1
	e.leap.selecting = false
	e.leap.lastSrc = ""
	// Starting a leap does not clear an existing selection (Cat keeps it until you do something),
	// but editing will replace it.
	if Debug {
		fmt.Printf("LEAP START dir=%v origin=%d\n", dir, e.leap.originCaret)
	}
}

func (e *Editor) leapEndCommit() {
	// Commit: keep caret where it is.
	// Store query for Leap Again.
	if len(e.leap.query) > 0 {
		e.leap.lastCommit = append(e.leap.lastCommit[:0], e.leap.query...)
	}
	if Debug {
		fmt.Printf("LEAP COMMIT caret=%d query=%q lastCommit=%q selecting=%v sel=%v\n",
			e.caret, string(e.leap.query), string(e.leap.lastCommit), e.leap.selecting, e.sel.active)
	}

	// If selecting, selection remains active (already tracked).
	// If not selecting, we leave selection as-is.

	e.leap.active = false
	e.leap.query = e.leap.query[:0]
	e.leap.lastFoundPos = -1
	e.leap.selecting = false
	e.leap.lastSrc = ""
}

func (e *Editor) leapCancel() {
	// Cancel leap: return to origin; also cancel selection that started during this leap.
	e.caret = e.leap.originCaret
	if e.leap.selecting {
		e.sel.active = false
	}
	e.leap.active = false
	e.leap.query = e.leap.query[:0]
	e.leap.lastFoundPos = -1
	e.leap.selecting = false
	e.leap.lastSrc = ""
	if Debug {
		fmt.Printf("LEAP CANCEL -> origin=%d\n", e.caret)
	}
}

func (e *Editor) leapAppend(text string) {
	e.leap.query = append(e.leap.query, []rune(text)...)
	e.leapSearch()
}

func (e *Editor) leapBackspace() {
	if len(e.leap.query) == 0 {
		return
	}
	e.leap.query = e.leap.query[:len(e.leap.query)-1]
	e.leapSearch()
}

func (e *Editor) leapSearch() {
	if len(e.leap.query) == 0 {
		e.caret = e.leap.originCaret
		e.leap.lastFoundPos = -1
		if e.leap.selecting {
			e.updateSelectionWithCaret()
		}
		return
	}

	// Canon Cat feel: refine anchored at origin
	start := e.leap.originCaret

	if pos, ok := findInDir(e.buf, e.leap.query, start, e.leap.dir, true /*wrap*/); ok {
		e.caret = pos
		e.leap.lastFoundPos = pos
	} else {
		e.leap.lastFoundPos = -1
	}
	if e.leap.selecting {
		e.updateSelectionWithCaret()
	}
}

func (e *Editor) updateSelectionWithCaret() {
	e.sel.active = true
	e.sel.a = e.leap.selAnchor
	e.sel.b = e.caret
}

func (e *Editor) leapAgain(dir Dir) {
	if len(e.leap.lastCommit) == 0 {
		if Debug {
			fmt.Println("LEAP AGAIN: no lastCommit")
		}
		return
	}
	q := e.leap.lastCommit

	// Start after/before caret to get "next" behaviour.
	start := e.caret
	if dir == DirFwd {
		start = min(len(e.buf), e.caret+1)
	} else {
		start = max(0, e.caret-1)
	}

	if pos, ok := findInDir(e.buf, q, start, dir, true /*wrap*/); ok {
		e.caret = pos
	} else if Debug {
		fmt.Printf("LEAP AGAIN miss query=%q\n", string(q))
	}
}

// ======================
// Editing + selection
// ======================

func (e *Editor) insertText(text string) {
	// Replace selection if active
	if e.sel.active {
		e.deleteSelection()
	}
	rs := []rune(text)
	if len(rs) == 0 {
		return
	}
	e.caret = clamp(e.caret, 0, len(e.buf))
	e.buf = append(e.buf[:e.caret], append(rs, e.buf[e.caret:]...)...)
	e.caret += len(rs)
}

func (e *Editor) backspaceOrDeleteSelection(isBackspace bool) {
	if e.sel.active {
		e.deleteSelection()
		return
	}
	if len(e.buf) == 0 {
		return
	}
	if isBackspace {
		if e.caret <= 0 {
			return
		}
		e.buf = append(e.buf[:e.caret-1], e.buf[e.caret:]...)
		e.caret--
		return
	}
	// delete forward
	if e.caret >= len(e.buf) {
		return
	}
	e.buf = append(e.buf[:e.caret], e.buf[e.caret+1:]...)
}

func (e *Editor) deleteSelection() {
	a, b := e.sel.normalised()
	a = clamp(a, 0, len(e.buf))
	b = clamp(b, 0, len(e.buf))
	if a == b {
		e.sel.active = false
		return
	}
	e.buf = append(e.buf[:a], e.buf[b:]...)
	e.caret = a
	e.sel.active = false
}

func (e *Editor) moveCaret(delta int, extendSelection bool) {
	newPos := clamp(e.caret+delta, 0, len(e.buf))
	if extendSelection {
		if !e.sel.active {
			e.sel.active = true
			e.sel.a = e.caret
			e.sel.b = newPos
		} else {
			e.sel.b = newPos
		}
	} else {
		e.sel.active = false
	}
	e.caret = newPos
}

func (e *Editor) copySelection() {
	if !e.sel.active {
		return
	}
	a, b := e.sel.normalised()
	a = clamp(a, 0, len(e.buf))
	b = clamp(b, 0, len(e.buf))
	if a == b {
		return
	}
	_ = sdl.SetClipboardText(string(e.buf[a:b]))
}

func (e *Editor) cutSelection() {
	if !e.sel.active {
		return
	}
	e.copySelection()
	e.deleteSelection()
}

func (e *Editor) pasteClipboard() {
	txt, err := sdl.GetClipboardText()
	if err != nil || txt == "" {
		return
	}
	e.insertText(txt)
}

// ======================
// Rendering
// ======================

func render(r *sdl.Renderer, win *sdl.Window, font *ttf.Font, ed *Editor) {
	_, h32 := win.GetSize()
	h := int(h32)

	bg := sdl.Color{R: 20, G: 20, B: 24, A: 255}
	fg := sdl.Color{R: 230, G: 230, B: 235, A: 255}
	dim := sdl.Color{R: 160, G: 160, B: 170, A: 255}
	green := sdl.Color{R: 120, G: 220, B: 140, A: 255}
	blue := sdl.Color{R: 120, G: 170, B: 255, A: 255}
	orange := sdl.Color{R: 255, G: 170, B: 120, A: 255}
	selCol := sdl.Color{R: 60, G: 90, B: 140, A: 255}

	r.SetDrawColor(bg.R, bg.G, bg.B, bg.A)
	r.Clear()

	cellW, _, _ := font.SizeUTF8("M")
	lineH := font.Height() + 4
	left := 12
	top := 12

	lines := splitLines(ed.buf)

	// Status lines
	if ed.leap.active {
		col := blue
		dirArrow := "→"
		if ed.leap.dir == DirBack {
			col = orange
			dirArrow = "←"
		}
		drawText(r, font, left, top,
			fmt.Sprintf("LEAP %s heldL=%v heldR=%v selecting=%v src=%s query=%q last=%q",
				dirArrow, ed.leap.heldL, ed.leap.heldR, ed.leap.selecting, ed.leap.lastSrc,
				string(ed.leap.query), string(ed.leap.lastCommit)),
			col,
		)
	} else {
		drawText(r, font, left, top,
			fmt.Sprintf("EDIT  (Cmd-only Leap. Ctrl+Cmd = Leap Again. Ctrl+C/X/V clipboard)  last=%q",
				string(ed.leap.lastCommit)),
			dim,
		)
	}
	drawText(r, font, left, top+lineH+2, ed.lastEvent, dim)

	// Text start
	y0 := top + (lineH * 2) + 12
	y := y0

	// Draw selection background (monospace-based)
	if ed.sel.active {
		a, b := ed.sel.normalised()
		a = clamp(a, 0, len(ed.buf))
		b = clamp(b, 0, len(ed.buf))
		drawSelectionRects(r, lines, ed.buf, a, b, left, y0, lineH, cellW, selCol)
	}

	// Caret position in line/col space
	cLine := caretLineAt(lines, ed.caret)
	cCol := caretColAt(lines, ed.caret)

	for i, line := range lines {
		drawText(r, font, left, y, line, fg)

		if i == cLine {
			x := left + cCol*cellW
			r.SetDrawColor(255, 255, 255, 255)
			_ = r.DrawLine(int32(x), int32(y), int32(x), int32(y+lineH-2))
		}

		y += lineH
		if y > h-60 {
			break
		}
	}

	// underline found position while leaping
	if ed.leap.active && ed.leap.lastFoundPos >= 0 {
		fLine := caretLineAt(lines, ed.leap.lastFoundPos)
		fCol := caretColAt(lines, ed.leap.lastFoundPos)
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
	aLine, aCol := lineColForPos(lines, a)
	bLine, bCol := lineColForPos(lines, b)

	r.SetDrawColor(col.R, col.G, col.B, col.A)

	if aLine == bLine {
		x1 := left + aCol*cellW
		x2 := left + bCol*cellW
		if x2 < x1 {
			x1, x2 = x2, x1
		}
		_ = r.FillRect(&sdl.Rect{X: int32(x1), Y: int32(y0 + aLine*lineH), W: int32(max(2, x2-x1)), H: int32(lineH)})
		return
	}

	// First line: from aCol to end of line
	firstLen := len([]rune(lines[aLine]))
	x1 := left + aCol*cellW
	x2 := left + firstLen*cellW
	_ = r.FillRect(&sdl.Rect{X: int32(x1), Y: int32(y0 + aLine*lineH), W: int32(max(2, x2-x1)), H: int32(lineH)})

	// Middle full lines
	for ln := aLine + 1; ln < bLine; ln++ {
		lineLen := len([]rune(lines[ln]))
		_ = r.FillRect(&sdl.Rect{X: int32(left), Y: int32(y0 + ln*lineH), W: int32(max(2, lineLen*cellW)), H: int32(lineH)})
	}

	// Last line: from start to bCol
	x3 := left
	x4 := left + bCol*cellW
	_ = r.FillRect(&sdl.Rect{X: int32(x3), Y: int32(y0 + bLine*lineH), W: int32(max(2, x4-x3)), H: int32(lineH)})

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
// Line/col mapping
// ======================

func splitLines(buf []rune) []string {
	lines := make([]string, 0, 64)
	var cur []rune
	for _, r := range buf {
		if r == '\n' {
			lines = append(lines, string(cur))
			cur = cur[:0]
			continue
		}
		cur = append(cur, r)
	}
	lines = append(lines, string(cur))
	return lines
}

// Convert a buffer position to (line, col) assuming lines from splitLines.
func lineColForPos(lines []string, pos int) (int, int) {
	if pos <= 0 {
		return 0, 0
	}
	p := 0
	for i, line := range lines {
		l := len([]rune(line))
		if pos <= p+l {
			return i, pos - p
		}
		p += l + 1
	}
	// end
	if len(lines) == 0 {
		return 0, 0
	}
	last := len(lines) - 1
	return last, len([]rune(lines[last]))
}

func caretLineAt(lines []string, caret int) int {
	ln, _ := lineColForPos(lines, caret)
	return ln
}

func caretColAt(lines []string, caret int) int {
	_, col := lineColForPos(lines, caret)
	return col
}

// ======================
// Search
// ======================

func findInDir(hay []rune, needle []rune, start int, dir Dir, wrap bool) (int, bool) {
	if len(needle) == 0 {
		return start, true
	}
	if len(hay) == 0 || len(needle) > len(hay) {
		return -1, false
	}
	start = clamp(start, 0, len(hay))

	if dir == DirFwd {
		if pos, ok := scanFwd(hay, needle, start); ok {
			return pos, true
		}
		if wrap {
			return scanFwd(hay, needle, 0)
		}
		return -1, false
	}

	// backward
	searchStart := start - 1 // search strictly before start to get the previous match
	if pos, ok := scanBack(hay, needle, searchStart); ok {
		return pos, true
	}
	if wrap {
		return scanBack(hay, needle, len(hay))
	}
	return -1, false
}

func scanFwd(hay, needle []rune, start int) (int, bool) {
	for i := start; i+len(needle) <= len(hay); i++ {
		if matchAt(hay, needle, i) {
			return i, true
		}
	}
	return -1, false
}

func scanBack(hay, needle []rune, start int) (int, bool) {
	if start < 0 {
		return -1, false
	}
	lastStart := min(start, len(hay)-len(needle))
	for i := lastStart; i >= 0; i-- {
		if matchAt(hay, needle, i) {
			return i, true
		}
	}
	return -1, false
}

func matchAt(hay, needle []rune, i int) bool {
	for j := 0; j < len(needle); j++ {
		if hay[i+j] != needle[j] {
			return false
		}
	}
	return true
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
func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
