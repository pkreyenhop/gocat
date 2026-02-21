package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
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

type bufferSlot struct {
	ed   *editor.Editor
	path string
	// picker buffers are temporary file-list views
	picker     bool
	pickerRoot string
}

type appState struct {
	ed          *editor.Editor // active buffer editor (mirrors buffers[bufIdx].ed)
	lastEvent   string
	lastMods    sdl.Keymod
	blinkAt     time.Time
	win         *sdl.Window
	lastW       int
	lastH       int
	lastX       int32
	lastY       int32
	openRoot    string
	open        openPrompt
	buffers     []bufferSlot
	bufIdx      int
	currentPath string // mirrors active buffer path for status messages
	showHelp    bool
}

type helpEntry struct {
	action string
	keys   string
}

// helpEntries drives both the in-app help overlay/buffer and README sync (see main_help_test.go).
var helpEntries = []helpEntry{
	{"Leap forward / backward", "Hold Right Cmd / Left Cmd (type query)"},
	{"Leap Again", "Ctrl+Right Cmd / Ctrl+Left Cmd"},
	{"New buffer / cycle buffers", "Ctrl+B / Tab"},
	{"File picker / load match", "Ctrl+O / Ctrl+L"},
	{"Save current / save all", "Ctrl+W / Ctrl+Shift+S"},
	{"Close buffer / quit", "Ctrl+Q / Ctrl+Shift+Q"},
	{"Undo", "Ctrl+U"},
	{"Comment / uncomment", "Ctrl+/ (selection or current line)"},
	{"Line start / end", "Ctrl+A / Ctrl+E (Shift = select)"},
	{"Buffer start / end", "Ctrl+Shift+A / Ctrl+Shift+E"},
	{"Kill to EOL", "Ctrl+K"},
	{"Copy / Cut / Paste", "Ctrl+C / Ctrl+X / Ctrl+V"},
	{"Navigation", "Arrows, PageUp/Down (Shift = select)"},
	{"Escape", "Clear selection; close picker"},
	{"Help buffer", "Ctrl+Shift+/ (Ctrl+?)"},
}

type openPrompt struct {
	Active  bool
	Query   string
	Matches []string
}

func (app *appState) initBuffers(ed *editor.Editor) {
	app.buffers = []bufferSlot{{ed: ed}}
	app.bufIdx = 0
	app.ed = ed
	app.currentPath = ""
}

func (app *appState) syncActiveBuffer() {
	if app == nil {
		return
	}
	if len(app.buffers) == 0 {
		app.ed = nil
		app.currentPath = ""
		return
	}
	app.bufIdx = clamp(app.bufIdx, 0, len(app.buffers)-1)
	b := app.buffers[app.bufIdx]
	app.ed = b.ed
	app.currentPath = b.path
}

func (app *appState) addBuffer() {
	nb := bufferSlot{ed: editor.NewEditor("")}
	nb.ed.SetClipboard(sdlClipboard{})
	app.buffers = append(app.buffers, nb)
	app.bufIdx = len(app.buffers) - 1
	app.syncActiveBuffer()
}

func (app *appState) addPickerBuffer(lines []string) {
	nb := bufferSlot{
		ed:         editor.NewEditor(strings.Join(lines, "\n")),
		picker:     true,
		pickerRoot: app.openRoot,
	}
	nb.ed.SetClipboard(sdlClipboard{})
	app.buffers = append(app.buffers, nb)
	app.bufIdx = len(app.buffers) - 1
	app.syncActiveBuffer()
}

func (app *appState) switchBuffer(delta int) {
	if len(app.buffers) == 0 {
		return
	}
	n := len(app.buffers)
	app.bufIdx = (app.bufIdx + delta + n) % n
	app.syncActiveBuffer()
}

// closeBuffer removes the active buffer. It returns the number of buffers
// remaining after the close.
func (app *appState) closeBuffer() int {
	if app == nil || len(app.buffers) == 0 {
		return 0
	}
	// Remove active slot
	app.buffers = append(app.buffers[:app.bufIdx], app.buffers[app.bufIdx+1:]...)
	if app.bufIdx >= len(app.buffers) {
		app.bufIdx = len(app.buffers) - 1
	}
	app.syncActiveBuffer()
	// Closing a buffer cancels any open prompt.
	app.open = openPrompt{}
	return len(app.buffers)
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
	// Start in fullscreen to avoid platform window chrome handling (e.g., Cmd+M minimizing).
	_ = win.SetFullscreen(sdl.WINDOW_FULLSCREEN_DESKTOP)

	ren := mustRenderer(sdl.CreateRenderer(
		win, -1,
		sdl.RENDERER_ACCELERATED|sdl.RENDERER_PRESENTVSYNC,
	))
	defer ren.Destroy()

	font := mustFont(ttf.OpenFont(pickFont(), 22))
	defer font.Close()

	ed := editor.NewEditor(
		"",
	)
	ed.SetClipboard(sdlClipboard{})

	wW, wH := win.GetSize()
	wX, wY := win.GetPosition()
	root, _ := os.Getwd()
	app := appState{
		blinkAt:  time.Now(),
		win:      win,
		lastW:    int(wW),
		lastH:    int(wH),
		lastX:    wX,
		lastY:    wY,
		openRoot: root,
	}
	app.initBuffers(ed)

	// If filenames are provided on the command line, load them into buffers.
	if len(os.Args) > 1 {
		loadStartupFiles(&app, filterArgsToFiles(os.Args[1:]))
	} else {
		// Empty buffer when no file passed
		app.ed.Buf = nil
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

	// Modal open prompt takes precedence over other input
	if app.open.Active {
		return handleOpenEvent(app, ev)
	}

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

		// Escape: clear selection or close picker; no global quit
		if e.Type == sdl.KEYDOWN && e.Repeat == 0 && sym == sdl.K_ESCAPE && !ed.Leap.Active {
			if ed.Sel.Active {
				ed.Sel.Active = false
				ed.Leap.Selecting = false
				return true
			}
			if len(app.buffers) > 0 && app.buffers[app.bufIdx].picker {
				app.closeBuffer()
				app.lastEvent = "Closed file picker"
				return true
			}
			return true
		}

		// Clipboard + file ops on Ctrl (not Cmd, because Cmd is Leap keys)
		if e.Type == sdl.KEYDOWN && e.Repeat == 0 {
			ctrlHeld := (mods & sdl.KMOD_CTRL) != 0
			if ctrlHeld {
				switch sym {
				case sdl.K_q:
					if (mods & sdl.KMOD_SHIFT) != 0 {
						app.lastEvent = "Quit (discard all buffers)"
						return false
					}
					// quit current buffer only
					remaining := app.closeBuffer()
					if remaining == 0 {
						app.lastEvent = "Closed last buffer, quitting"
						return false
					}
					app.lastEvent = fmt.Sprintf("Closed buffer, now %d/%d", app.bufIdx+1, remaining)
					return true
				case sdl.K_b:
					app.addBuffer()
					app.lastEvent = fmt.Sprintf("New buffer %d/%d", app.bufIdx+1, len(app.buffers))
					return true
				case sdl.K_w:
					if err := saveCurrent(app); err != nil {
						app.lastEvent = fmt.Sprintf("SAVE ERR: %v", err)
					} else {
						app.lastEvent = fmt.Sprintf("Saved %s", app.currentPath)
					}
					return true
				case sdl.K_s:
					if (mods & sdl.KMOD_SHIFT) != 0 {
						if err := saveAll(app); err != nil {
							app.lastEvent = fmt.Sprintf("SAVE ALL ERR: %v", err)
						} else {
							app.lastEvent = "Saved all buffers"
						}
						return true
					}
					if err := saveCurrent(app); err != nil {
						app.lastEvent = fmt.Sprintf("SAVE ERR: %v", err)
					} else {
						app.lastEvent = fmt.Sprintf("Saved %s", app.currentPath)
					}
					return true
				case sdl.K_a:
					lines := editor.SplitLines(ed.Buf)
					if (mods & sdl.KMOD_SHIFT) != 0 {
						ed.CaretToBufferEdge(lines, false, (mods&sdl.KMOD_SHIFT) != 0)
					} else {
						ed.CaretToLineEdge(lines, false, false)
					}
					return true
				case sdl.K_e:
					lines := editor.SplitLines(ed.Buf)
					if (mods & sdl.KMOD_SHIFT) != 0 {
						ed.CaretToBufferEdge(lines, true, (mods&sdl.KMOD_SHIFT) != 0)
					} else {
						ed.CaretToLineEdge(lines, true, false)
					}
					return true
				case sdl.K_k:
					ed.KillToLineEnd(editor.SplitLines(ed.Buf))
					return true
				case sdl.K_u:
					ed.Undo()
					app.lastEvent = "Undo"
					return true
				case sdl.K_SLASH:
					if (mods & sdl.KMOD_SHIFT) != 0 {
						app.addBuffer()
						app.ed.Buf = []rune(helpText())
						app.currentPath = ""
						app.buffers[app.bufIdx].path = ""
						app.lastEvent = "Opened shortcuts buffer"
						return true
					}
					toggleComment(ed)
					app.lastEvent = "Toggled comment"
					return true
				case sdl.K_o:
					list, err := listFiles(app.openRoot, 500)
					if err != nil {
						app.lastEvent = fmt.Sprintf("OPEN ERR: %v", err)
						return true
					}
					if len(list) == 0 {
						app.lastEvent = "OPEN: no files under root"
						return true
					}
					app.addPickerBuffer(list)
					app.lastEvent = fmt.Sprintf("OPEN: file picker (%d files). Leap to a line, Ctrl+L to load", len(list))
					return true
				case sdl.K_l:
					if err := loadFileAtCaret(app); err != nil {
						app.lastEvent = fmt.Sprintf("LOAD ERR: %v", err)
					} else {
						app.lastEvent = fmt.Sprintf("Opened %s", app.currentPath)
					}
					return true
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
			if sym == sdl.K_TAB && !ed.Leap.Active {
				app.switchBuffer(1)
				app.lastEvent = fmt.Sprintf("Switched to buffer %d/%d", app.bufIdx+1, len(app.buffers))
				return true
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

		// Normal editing (outside leap). Key repeat moves repeatedly.
		if !ed.Leap.Active && e.Type == sdl.KEYDOWN {
			lines := editor.SplitLines(ed.Buf)
			switch sym {
			case sdl.K_BACKSPACE:
				ed.BackspaceOrDeleteSelection(true)
			case sdl.K_DELETE:
				ed.BackspaceOrDeleteSelection(false)
			case sdl.K_LEFT:
				ed.MoveCaret(-1, (mods&sdl.KMOD_SHIFT) != 0)
			case sdl.K_RIGHT:
				ed.MoveCaret(1, (mods&sdl.KMOD_SHIFT) != 0)
			case sdl.K_UP:
				ed.MoveCaretLine(lines, -1, (mods&sdl.KMOD_SHIFT) != 0)
			case sdl.K_DOWN:
				ed.MoveCaretLine(lines, 1, (mods&sdl.KMOD_SHIFT) != 0)
			case sdl.K_PAGEDOWN:
				ed.MoveCaretPage(lines, 20, editor.DirFwd, (mods&sdl.KMOD_SHIFT) != 0)
			case sdl.K_PAGEUP:
				ed.MoveCaretPage(lines, 20, editor.DirBack, (mods&sdl.KMOD_SHIFT) != 0)
			case sdl.K_RETURN, sdl.K_KP_ENTER:
				if e.Repeat == 0 { // avoid spamming newlines on OS-level repeat
					ed.InsertText("\n")
				}
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
		if text == "\t" {
			// Tab is reserved for buffer switching.
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

func handleOpenEvent(app *appState, ev sdl.Event) bool {
	switch e := ev.(type) {
	case *sdl.KeyboardEvent:
		if e.Type != sdl.KEYDOWN || e.Repeat != 0 {
			return true
		}
		switch e.Keysym.Sym {
		case sdl.K_ESCAPE:
			app.open.Active = false
			app.lastEvent = "Open cancelled"
			return true
		case sdl.K_BACKSPACE:
			if len(app.open.Query) > 0 {
				rs := []rune(app.open.Query)
				app.open.Query = string(rs[:len(rs)-1])
				app.open.Matches = findMatches(app.openRoot, app.open.Query, 50)
			}
			return true
		case sdl.K_RETURN, sdl.K_KP_ENTER:
			app.open.Matches = findMatches(app.openRoot, app.open.Query, 50)
			if len(app.open.Matches) == 1 {
				if err := openPath(app, app.open.Matches[0]); err != nil {
					app.lastEvent = fmt.Sprintf("OPEN ERR: %v", err)
				} else {
					app.lastEvent = fmt.Sprintf("Opened %s", app.currentPath)
				}
				app.open.Active = false
			} else {
				app.lastEvent = fmt.Sprintf("OPEN: %d matches; refine", len(app.open.Matches))
			}
			return true
		default:
			if r, ok := keyToRune(e.Keysym.Sym, sdl.Keymod(e.Keysym.Mod)); ok {
				app.open.Query += string(r)
				app.open.Matches = findMatches(app.openRoot, app.open.Query, 50)
			}
			return true
		}
	case *sdl.TextInputEvent:
		text := textInputString(e)
		if text != "" && utf8.ValidString(text) {
			app.open.Query += text
			app.open.Matches = findMatches(app.openRoot, app.open.Query, 50)
		}
		return true
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
// File helpers (very simple)
// ======================

func saveCurrent(app *appState) error {
	if app == nil || app.ed == nil || len(app.buffers) == 0 {
		return fmt.Errorf("no editor to save")
	}
	path := app.currentPath
	if path == "" {
		path = defaultPath(app)
		app.currentPath = path
	}
	if err := os.WriteFile(path, []byte(string(app.ed.Buf)), 0644); err != nil {
		return err
	}
	app.buffers[app.bufIdx].path = path
	return nil
}

func saveAll(app *appState) error {
	if app == nil || len(app.buffers) == 0 {
		return fmt.Errorf("no buffers to save")
	}
	for i := range app.buffers {
		app.bufIdx = i
		app.syncActiveBuffer()
		if err := saveCurrent(app); err != nil {
			return err
		}
	}
	return nil
}

func openCurrent(app *appState) error {
	if app == nil || app.ed == nil || len(app.buffers) == 0 {
		return fmt.Errorf("no editor to open into")
	}
	path := app.currentPath
	if path == "" {
		path = defaultPath(app)
	}
	return openPath(app, path)
}

func openPath(app *appState, path string) error {
	if app == nil || app.ed == nil || len(app.buffers) == 0 {
		return fmt.Errorf("no active buffer")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	// Only files under openRoot are allowed.
	if app.openRoot != "" {
		if rel, err := filepath.Rel(app.openRoot, path); err != nil || strings.HasPrefix(rel, "..") {
			return fmt.Errorf("refusing to open outside %s", app.openRoot)
		}
	}
	app.currentPath = path
	app.buffers[app.bufIdx].path = path
	app.ed.Buf = []rune(string(data))
	app.ed.Caret = 0
	app.ed.Sel = editor.Sel{}
	app.ed.Leap = editor.LeapState{LastFoundPos: -1}
	return nil
}

func defaultPath(app *appState) string {
	if app == nil {
		return "leap.txt"
	}
	if app.bufIdx <= 0 {
		return "leap.txt"
	}
	return fmt.Sprintf("leap-%d.txt", app.bufIdx+1)
}

// loadFileAtCaret loads the file referenced by the current line in a picker buffer.
// The picker buffer is reused for the opened file content.
func loadFileAtCaret(app *appState) error {
	if app == nil || app.ed == nil || len(app.buffers) == 0 {
		return fmt.Errorf("no active buffer")
	}
	slot := &app.buffers[app.bufIdx]
	if !slot.picker {
		return fmt.Errorf("not in file picker")
	}
	lines := editor.SplitLines(app.ed.Buf)
	lineIdx := editor.CaretLineAt(lines, app.ed.Caret)
	if lineIdx < 0 || lineIdx >= len(lines) {
		return fmt.Errorf("no line under caret")
	}
	line := strings.TrimSpace(lines[lineIdx])
	if line == "" {
		return fmt.Errorf("empty line")
	}
	if filepath.IsAbs(line) || strings.Contains(line, "..") {
		return fmt.Errorf("invalid path")
	}
	full := filepath.Join(slot.pickerRoot, line)
	slot.picker = false
	slot.pickerRoot = ""
	return openPath(app, full)
}

// findMatches searches for files under root whose basename contains query (case-insensitive).
// It skips hidden directories and stops after limit hits.
func findMatches(root, query string, limit int) []string {
	if query == "" {
		return nil
	}
	lq := strings.ToLower(query)
	matches := make([]string, 0, 8)
	errStop := fmt.Errorf("stop")

	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if len(matches) >= limit {
			return errStop
		}
		if d.IsDir() {
			base := d.Name()
			if strings.HasPrefix(base, ".") || base == "vendor" {
				if path == root {
					return nil
				}
				return filepath.SkipDir
			}
			return nil
		}
		base := strings.ToLower(d.Name())
		if strings.Contains(base, lq) {
			matches = append(matches, path)
		}
		return nil
	})
	return matches
}

// listFiles returns relative file paths under root (non-hidden/vendor), sorted.
func listFiles(root string, limit int) ([]string, error) {
	if root == "" {
		return nil, fmt.Errorf("no root")
	}
	root = filepath.Clean(root)
	files := make([]string, 0, 16)
	errStop := fmt.Errorf("stop")

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if len(files) >= limit {
			return errStop
		}
		if d.IsDir() {
			base := d.Name()
			if strings.HasPrefix(base, ".") || base == "vendor" {
				if path == root {
					return nil
				}
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		files = append(files, rel)
		return nil
	})
	if err != nil && err != errStop {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

// loadStartupFiles loads or creates files provided on the CLI into buffers and
// updates openRoot per file. The last file becomes the active buffer.
func loadStartupFiles(app *appState, args []string) {
	if app == nil {
		return
	}
	for i, arg := range args {
		if i > 0 {
			app.addBuffer()
		}
		abs, err := filepath.Abs(arg)
		if err != nil {
			app.lastEvent = fmt.Sprintf("OPEN ERR: %v", err)
			continue
		}
		app.openRoot = filepath.Dir(abs)
		if _, err := os.Stat(abs); errors.Is(err, os.ErrNotExist) {
			// Create an empty file so the user can start editing immediately.
			if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
				app.lastEvent = fmt.Sprintf("OPEN ERR: %v", err)
				continue
			}
			if writeErr := os.WriteFile(abs, []byte(""), 0644); writeErr != nil {
				app.lastEvent = fmt.Sprintf("OPEN ERR: %v", writeErr)
				continue
			}
		}
		if err := openPath(app, abs); err != nil {
			app.lastEvent = fmt.Sprintf("OPEN ERR: %v", err)
			continue
		}
		app.lastEvent = fmt.Sprintf("Opened %s", app.currentPath)
	}
}

// filterArgsToFiles keeps only regular files from CLI args, skipping directories/globs that expand to dirs.
func filterArgsToFiles(args []string) []string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		info, err := os.Stat(a)
		if err == nil && info.Mode().IsRegular() {
			out = append(out, a)
		}
	}
	return out
}

func bufferLabel(app *appState) string {
	if app == nil {
		return "buf ?"
	}
	total := len(app.buffers)
	if total == 0 {
		return "buf 0/0"
	}
	name := app.currentPath
	if name == "" {
		name = "<untitled>"
	} else {
		name = filepath.Base(name)
	}
	return fmt.Sprintf("buf %d/%d [%s]", app.bufIdx+1, total, name)
}

func helpText() string {
	var sb strings.Builder
	sb.WriteString("Shortcuts\n\n")
	for _, h := range helpEntries {
		sb.WriteString(h.action)
		sb.WriteString(": ")
		sb.WriteString(h.keys)
		sb.WriteString("\n")
	}
	return sb.String()
}

func toggleComment(ed *editor.Editor) {
	if ed == nil {
		return
	}
	oldLines := editor.SplitLines(ed.Buf)
	if len(oldLines) == 0 {
		return
	}
	origSel := ed.Sel
	startLine := editor.CaretLineAt(oldLines, ed.Caret)
	endLine := startLine
	selA, selB := ed.Caret, ed.Caret
	if ed.Sel.Active {
		selA, selB = ed.Sel.Normalised()
		sl, _ := editor.LineColForPos(oldLines, selA)
		el, _ := editor.LineColForPos(oldLines, selB)
		startLine, endLine = sl, el
	}
	startLine = clamp(startLine, 0, len(oldLines)-1)
	endLine = clamp(endLine, startLine, len(oldLines)-1)

	allCommented := true
	for i := startLine; i <= endLine; i++ {
		if !strings.HasPrefix(oldLines[i], "//") {
			allCommented = false
			break
		}
	}

	lines := append([]string(nil), oldLines...)
	deltas := make([]int, len(lines))
	for i := startLine; i <= endLine; i++ {
		if allCommented {
			lines[i] = strings.TrimPrefix(lines[i], "//")
			deltas[i] = -2
		} else {
			lines[i] = "//" + lines[i]
			deltas[i] = 2
		}
	}

	cum := make([]int, len(deltas)+1)
	for i := 0; i < len(deltas); i++ {
		cum[i+1] = cum[i] + deltas[i]
	}
	adjustPos := func(oldPos int) int {
		ln, _ := editor.LineColForPos(oldLines, oldPos)
		if ln < 0 || ln >= len(oldLines) {
			return oldPos
		}
		return oldPos + cum[ln] + deltas[ln]
	}

	ed.Buf = []rune(strings.Join(lines, "\n"))
	if origSel.Active {
		ed.Sel.Active = true
		ed.Sel.A = adjustPos(selA)
		ed.Sel.B = adjustPos(selB)
	} else {
		ed.Sel.Active = false
	}
	ed.Caret = adjustPos(ed.Caret)
	ed.Caret = clamp(ed.Caret, 0, len(ed.Buf))
}

func drawHelp(r *sdl.Renderer, font *ttf.Font, x, y int, col sdl.Color) int {
	lines := make([]string, 0, len(helpEntries))
	for _, h := range helpEntries {
		lines = append(lines, fmt.Sprintf("%s: %s", h.action, h.keys))
	}
	h := 0
	lineH := font.Height() + 4
	for i, l := range lines {
		drawText(r, font, x, y+i*lineH, l, col)
		h = y + i*lineH
	}
	return h + lineH
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

	// Helix-inspired default palette (base16-default-dark-ish)
	bg := sdl.Color{R: 24, G: 24, B: 24, A: 255}          // base00
	fg := sdl.Color{R: 216, G: 216, B: 216, A: 255}       // base05
	dim := sdl.Color{R: 184, G: 184, B: 184, A: 255}      // base04
	green := sdl.Color{R: 161, G: 181, B: 108, A: 255}    // base0B
	blue := sdl.Color{R: 124, G: 175, B: 194, A: 255}     // base0D
	orange := sdl.Color{R: 220, G: 150, B: 86, A: 255}    // base09
	selCol := sdl.Color{R: 56, G: 56, B: 56, A: 255}      // base02
	caretCol := sdl.Color{R: 247, G: 202, B: 136, A: 255} // base0A

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
	bufStatus := bufferLabel(app)

	// Status lines
	if app.open.Active {
		drawText(r, font, left, top,
			fmt.Sprintf("%s OPEN query=%q matches=%d (Enter opens if single, Esc cancels)",
				bufStatus, app.open.Query, len(app.open.Matches)),
			blue,
		)
	} else if app.ed.Leap.Active {
		col := blue
		dirArrow := "→"
		if app.ed.Leap.Dir == editor.DirBack {
			col = orange
			dirArrow = "←"
		}
		drawText(r, font, left, top,
			fmt.Sprintf("%s LEAP %s heldL=%v heldR=%v selecting=%v src=%s query=%q last=%q",
				bufStatus, dirArrow, app.ed.Leap.HeldL, app.ed.Leap.HeldR, app.ed.Leap.Selecting, app.ed.Leap.LastSrc,
				string(app.ed.Leap.Query), string(app.ed.Leap.LastCommit)),
			col,
		)
	} else {
		drawText(r, font, left, top,
			fmt.Sprintf("%s EDIT  (Cmd-only Leap. Ctrl+Cmd = Leap Again. Ctrl+C/X/V clipboard)  last=%q",
				bufStatus, string(app.ed.Leap.LastCommit)),
			dim,
		)
	}
	drawText(r, font, left, top+lineH+2, app.lastEvent, dim)

	// Text start
	y0 := top + (lineH * 2) + 12
	y := y0

	// Help overlay
	if app.showHelp {
		helpY := y0
		drawText(r, font, left, helpY-lineH, "Shortcuts", blue)
		y = drawHelp(r, font, left, helpY, fg) + lineH
	}

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
			w := maxInt(2, min(cellW, lineH-2))
			hh := lineH - 2
			r.SetDrawColor(caretCol.R, caretCol.G, caretCol.B, caretCol.A)
			_ = r.FillRect(&sdl.Rect{
				X: int32(x),
				Y: int32(y),
				W: int32(w),
				H: int32(hh),
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
		"/Library/Fonts/JetBrainsMono-Regular.ttf",
		"/System/Library/Fonts/SFMono-Regular.otf",
		"/Library/Fonts/FiraCode-Regular.ttf",
		"/Library/Fonts/CascadiaMono.ttf",
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
	panic("No usable mono font found. Install JetBrains Mono / Fira Code / Cascadia / Menlo / DejaVu/Liberation or place DejaVuSansMono.ttf next to main.go")
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

func min(a, b int) int {
	if a < b {
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
