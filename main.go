package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
	"unsafe"

	"gc/editor"

	"github.com/veandco/go-sdl2/sdl"
	"github.com/veandco/go-sdl2/ttf"
)

const Debug = false
const tabWidth = 4

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
	dirty      bool
}

type appState struct {
	ed               *editor.Editor // active buffer editor (mirrors buffers[bufIdx].ed)
	lastEvent        string
	lastMods         modMask
	blinkAt          time.Time
	lastSpaceAt      time.Time
	lastSpaceLn      int
	inputActive      bool
	inputPrompt      string
	inputValue       string
	inputKind        string
	win              *sdl.Window
	lastW            int
	lastH            int
	lastX            int32
	lastY            int32
	openRoot         string
	open             openPrompt
	buffers          []bufferSlot
	bufIdx           int
	currentPath      string // mirrors active buffer path for status messages
	scrollLine       int
	showHelp         bool
	symbolInfoPopup  string
	symbolInfoScroll int
	syntaxHL         *syntaxHighlighter
	syntaxCheck      *goSyntaxChecker
	gopls            *goplsClient
	noGopls          bool
	clipboard        editor.Clipboard
}

type helpEntry struct {
	action string
	keys   string
}

// helpEntries drives both the in-app help overlay/buffer and README sync (see main_help_test.go).
var helpEntries = []helpEntry{
	{"Leap forward / backward", "Hold Right Cmd / Left Cmd (type query)"},
	{"Leap Again", "Ctrl+Right Cmd / Ctrl+Left Cmd"},
	{"New buffer / cycle buffers", "Ctrl+B / Ctrl+Tab"},
	{"File picker / load line path", "Ctrl+O / Ctrl+L"},
	{"Save current / save all", "Ctrl+W / Ctrl+Shift+S"},
	{"Save + fmt/fix + reload", "Ctrl+F"},
	{"Run package (go run .)", "Ctrl+R"},
	{"Close buffer / quit", "Ctrl+Q / Ctrl+Shift+Q"},
	{"Undo", "Ctrl+U"},
	{"Comment / uncomment", "Ctrl+/ (selection or current line)"},
	{"Line start / end", "Ctrl+A / Ctrl+E (Shift = select)"},
	{"Buffer start / end", "Ctrl+Shift+A / Ctrl+Shift+E"},
	{"Kill to EOL", "Ctrl+K"},
	{"Copy / Cut / Paste", "Ctrl+C / Ctrl+X / Ctrl+V"},
	{"Symbol info under cursor (Go)", "Ctrl+I"},
	{"Autocomplete (Go mode)", "Tab"},
	{"Navigation", "Arrows, PageUp/Down, Ctrl+, Ctrl+. (Shift = select)"},
	{"Escape", "Clear selection; close symbol popup; close picker or clean buffer"},
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
	app.lastSpaceLn = -1
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
	if app.clipboard != nil {
		nb.ed.SetClipboard(app.clipboard)
	}
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
	if app.clipboard != nil {
		nb.ed.SetClipboard(app.clipboard)
	}
	app.buffers = append(app.buffers, nb)
	app.bufIdx = len(app.buffers) - 1
	app.syncActiveBuffer()
}

func (app *appState) markDirty() {
	if app == nil || len(app.buffers) == 0 {
		return
	}
	app.buffers[app.bufIdx].dirty = true
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
	clip := sdlClipboard{}
	ed.SetClipboard(clip)

	wW, wH := win.GetSize()
	wX, wY := win.GetPosition()
	root, _ := os.Getwd()
	app := appState{
		blinkAt:     time.Now(),
		win:         win,
		lastW:       int(wW),
		lastH:       int(wH),
		lastX:       wX,
		lastY:       wY,
		openRoot:    root,
		syntaxHL:    newGoHighlighter(),
		syntaxCheck: newGoSyntaxChecker(),
		gopls:       newGoplsClient(),
		clipboard:   clip,
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
	defer app.gopls.close()

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
	if app.inputActive {
		return handleInputEvent(app, ev)
	}
	if app.open.Active {
		return handleOpenEvent(app, ev)
	}
	switch e := ev.(type) {
	case *sdl.QuitEvent:
		if app.ed != nil && app.ed.Leap.Active {
			return true
		}
		return false
	case *sdl.WindowEvent:
		if (e.Event == sdl.WINDOWEVENT_MINIMIZED || e.Event == sdl.WINDOWEVENT_HIDDEN) && app.win != nil {
			restoreWindow(app)
		}
		return true
	case *sdl.KeyboardEvent:
		return handleKeyEvent(app, sdlKeyEvent(e))
	case *sdl.TextInputEvent:
		return handleTextEvent(app, textInputString(e), sdlToMods(sdl.GetModState()))
	default:
		return true
	}
}

func handleOpenEvent(app *appState, ev sdl.Event) bool {
	switch e := ev.(type) {
	case *sdl.KeyboardEvent:
		return handleOpenKeyEvent(app, sdlKeyEvent(e))
	case *sdl.TextInputEvent:
		return handleOpenTextEvent(app, textInputString(e))
	default:
		return true
	}
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
		app.inputActive = true
		app.inputPrompt = "Save as: "
		app.inputValue = ""
		app.inputKind = "save"
		app.lastEvent = "Save: enter filename in input line, Enter to confirm, Esc to cancel"
		return fmt.Errorf("no path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(string(app.ed.Buf)), 0644); err != nil {
		return err
	}
	app.buffers[app.bufIdx].path = path
	app.buffers[app.bufIdx].dirty = false
	return nil
}

func saveAll(app *appState) error {
	if app == nil || len(app.buffers) == 0 {
		return fmt.Errorf("no buffers to save")
	}
	orig := app.bufIdx
	saved := 0
	for i := range app.buffers {
		app.bufIdx = i
		app.syncActiveBuffer()
		if !app.buffers[i].dirty {
			continue
		}
		if err := saveCurrent(app); err != nil {
			app.bufIdx = orig
			app.syncActiveBuffer()
			return err
		}
		saved++
	}
	app.bufIdx = orig
	app.syncActiveBuffer()
	if saved == 0 {
		return fmt.Errorf("no dirty buffers to save")
	}
	return nil
}

var runFmtFix = goFmtAndFix
var startGoRun = startGoRunProcess

func formatFixReloadCurrent(app *appState) error {
	if app == nil || app.ed == nil || len(app.buffers) == 0 {
		return fmt.Errorf("no active buffer")
	}
	if err := saveCurrent(app); err != nil {
		return err
	}
	if app.currentPath == "" {
		return fmt.Errorf("no path")
	}
	opErr := runFmtFix(app.currentPath)
	reloadErr := reloadCurrentFromDisk(app)
	if opErr != nil && reloadErr != nil {
		return fmt.Errorf("%v; reload: %v", opErr, reloadErr)
	}
	if reloadErr != nil {
		return reloadErr
	}
	return opErr
}

func runCurrentPackage(app *appState) error {
	if app == nil {
		return fmt.Errorf("no app state")
	}
	dir := app.openRoot
	if app.currentPath != "" {
		dir = filepath.Dir(app.currentPath)
	}
	if strings.TrimSpace(dir) == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		dir = cwd
	}
	title := fmt.Sprintf("[run] %s", filepath.Base(dir))
	app.addBuffer()
	app.buffers[app.bufIdx].path = title
	app.buffers[app.bufIdx].dirty = false
	app.currentPath = title
	runEd := app.ed
	runEd.Buf = []rune(fmt.Sprintf("$ (cd %s && go run .)\n\n", dir))
	runEd.Caret = len(runEd.Buf)
	runEd.Sel = editor.Sel{}

	appendOut := func(s string) {
		appendRunOutput(runEd, s)
	}
	onDone := func(err error) {
		if err != nil {
			appendOut(fmt.Sprintf("\n[exit] %v\n", err))
			return
		}
		appendOut("\n[exit] ok\n")
	}
	return startGoRun(dir, appendOut, onDone)
}

func startGoRunProcess(dir string, onOut func(string), onDone func(error)) error {
	if strings.TrimSpace(dir) == "" {
		return fmt.Errorf("no run directory")
	}
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = dir
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	go func() {
		drain := func(rd io.Reader, prefix string) {
			sc := bufio.NewScanner(rd)
			for sc.Scan() {
				if onOut != nil {
					onOut(prefix + sc.Text() + "\n")
				}
			}
		}
		done := make(chan struct{}, 2)
		go func() { drain(stdout, ""); done <- struct{}{} }()
		go func() { drain(stderr, "[stderr] "); done <- struct{}{} }()
		<-done
		<-done
		if onDone != nil {
			onDone(cmd.Wait())
		}
	}()
	return nil
}

func appendRunOutput(ed *editor.Editor, s string) {
	if ed == nil || s == "" {
		return
	}
	ed.Buf = append(ed.Buf, []rune(s)...)
	ed.Caret = len(ed.Buf)
}

func goFmtAndFix(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("no file path")
	}
	errs := make([]string, 0, 2)

	fmtCmd := exec.Command("gofmt", "-w", path)
	if out, err := fmtCmd.CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		errs = append(errs, "gofmt: "+msg)
	}

	fixCmd := exec.Command("go", "fix", path)
	fixCmd.Dir = filepath.Dir(path)
	if out, err := fixCmd.CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		errs = append(errs, "go fix: "+msg)
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func reloadCurrentFromDisk(app *appState) error {
	if app == nil || app.ed == nil {
		return fmt.Errorf("no active buffer")
	}
	path := app.currentPath
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("no path")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	app.ed.Buf = []rune(string(data))
	app.ed.Caret = clamp(app.ed.Caret, 0, len(app.ed.Buf))
	app.ed.Sel = editor.Sel{}
	app.ed.Leap = editor.LeapState{LastFoundPos: -1}
	app.buffers[app.bufIdx].dirty = false
	app.buffers[app.bufIdx].path = path
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
	app.buffers[app.bufIdx].dirty = false
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
	lines := editor.SplitLines(app.ed.Buf)
	lineIdx := editor.CaretLineAt(lines, app.ed.Caret)
	if lineIdx < 0 || lineIdx >= len(lines) {
		return fmt.Errorf("no line under caret")
	}
	line := strings.TrimSpace(lines[lineIdx])
	if line == "" {
		return fmt.Errorf("empty line")
	}

	root := app.openRoot
	if root == "" {
		if cwd, err := os.Getwd(); err == nil {
			root = cwd
		}
	}
	if slot.picker && slot.pickerRoot != "" {
		root = slot.pickerRoot
	}

	if slot.picker && line == ".." {
		up := filepath.Dir(root)
		list, err := pickerLines(up, 500)
		if err != nil {
			return err
		}
		app.openRoot = up
		slot.pickerRoot = up
		slot.ed.Buf = []rune(strings.Join(list, "\n"))
		app.currentPath = ""
		app.ed = slot.ed
		return nil
	}

	if slot.picker && strings.HasSuffix(line, "/") {
		next := filepath.Join(root, strings.TrimSuffix(line, "/"))
		list, err := pickerLines(next, 500)
		if err != nil {
			return err
		}
		app.openRoot = next
		slot.pickerRoot = next
		slot.ed.Buf = []rune(strings.Join(list, "\n"))
		app.currentPath = ""
		app.ed = slot.ed
		return nil
	}

	full := line
	if !filepath.IsAbs(full) {
		full = filepath.Join(root, line)
	}
	full = filepath.Clean(full)
	if root != "" {
		if rel, err := filepath.Rel(root, full); err != nil || strings.HasPrefix(rel, "..") {
			return fmt.Errorf("refusing to open outside %s", root)
		}
	}

	// If already loaded, jump to that buffer.
	for i, b := range app.buffers {
		if filepath.Clean(b.path) == filepath.Clean(full) {
			app.bufIdx = i
			app.syncActiveBuffer()
			return nil
		}
	}

	// Otherwise, open in a new buffer and leave the picker intact.
	app.addBuffer()
	app.openRoot = filepath.Dir(full)
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

func pickerLines(root string, limit int) ([]string, error) {
	if root == "" {
		return nil, fmt.Errorf("no root")
	}
	root = filepath.Clean(root)
	entries := make([]string, 0, limit)
	entries = append(entries, "..")

	dirEntries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	for _, de := range dirEntries {
		if len(entries) >= limit {
			break
		}
		name := de.Name()
		if strings.HasPrefix(name, ".") || name == "vendor" {
			continue
		}
		if de.IsDir() {
			entries = append(entries, name+"/")
		} else {
			entries = append(entries, name)
		}
	}
	sort.Strings(entries[1:]) // keep ".." first
	return entries, nil
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
			// Missing file: set up empty buffer with path so save can create it, but don't write yet.
			app.currentPath = abs
			app.buffers[app.bufIdx].path = abs
			app.ed.Buf = nil
			app.buffers[app.bufIdx].dirty = false
			app.lastEvent = fmt.Sprintf("Buffer for %s (file will be created on save)", abs)
			continue
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
		if err == nil {
			if info.Mode().IsRegular() {
				out = append(out, a)
			}
			continue
		}
		if errors.Is(err, os.ErrNotExist) {
			// Keep missing files so they can be created at startup.
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

// ensureCaretVisible adjusts the scroll offset so the caret's line stays
// within the visible viewport.
func ensureCaretVisible(app *appState, caretLine, totalLines, visibleLines int) {
	if app == nil {
		return
	}
	if caretLine < 0 {
		caretLine = 0
	}
	if totalLines < 0 {
		totalLines = 0
	}
	if visibleLines <= 0 {
		visibleLines = 1
	}
	maxStart := maxInt(0, totalLines-visibleLines)
	if app.scrollLine > maxStart {
		app.scrollLine = maxStart
	}
	if caretLine < app.scrollLine {
		app.scrollLine = caretLine
	} else if caretLine >= app.scrollLine+visibleLines {
		app.scrollLine = caretLine - visibleLines + 1
	}
	if app.scrollLine > maxStart {
		app.scrollLine = maxStart
	}
	if app.scrollLine < 0 {
		app.scrollLine = 0
	}
}

// ======================
// Rendering
// ======================

func render(r *sdl.Renderer, win *sdl.Window, font *ttf.Font, app *appState) {
	w32, h32 := win.GetSize()
	w := int(w32)
	h := int(h32)
	if app != nil && app.win != nil {
		x, y := win.GetPosition()
		w, hh := win.GetSize()
		app.lastX = x
		app.lastY = y
		app.lastW = int(w)
		app.lastH = int(hh)
	}

	// Purple-leaning palette
	bg := sdl.Color{R: 68, G: 39, B: 84, A: 255}    // #442754
	fg := sdl.Color{R: 184, G: 169, B: 217, A: 255} // #B8A9D9
	green := sdl.Color{R: 161, G: 181, B: 108, A: 255}
	blue := sdl.Color{R: 124, G: 175, B: 194, A: 255}
	selCol := sdl.Color{R: 70, G: 50, B: 90, A: 255}
	caretCol := sdl.Color{R: 217, G: 217, B: 217, A: 255} // #D9D9D9
	syntaxKeyword := sdl.Color{R: 199, G: 150, B: 255, A: 255}
	syntaxType := sdl.Color{R: 132, G: 204, B: 246, A: 255}
	syntaxFunc := sdl.Color{R: 250, G: 211, B: 120, A: 255}
	syntaxString := sdl.Color{R: 186, G: 230, B: 126, A: 255}
	syntaxNumber := sdl.Color{R: 255, G: 173, B: 134, A: 255}
	syntaxComment := sdl.Color{R: 138, G: 157, B: 113, A: 255}
	syntaxHeading := sdl.Color{R: 246, G: 193, B: 119, A: 255}
	syntaxLink := sdl.Color{R: 128, G: 214, B: 230, A: 255}
	syntaxPunctuation := sdl.Color{R: 187, G: 161, B: 221, A: 255}
	syntaxErr := sdl.Color{R: 242, G: 118, B: 118, A: 255}

	r.SetDrawColor(bg.R, bg.G, bg.B, bg.A)
	r.Clear()

	// No active editor: draw empty screen and bail.
	if app == nil || app.ed == nil {
		drawText(r, font, 12, 12, "gc| no buffer open", fg)
		r.Present()
		return
	}

	cellW, _, _ := font.SizeUTF8("M")
	lineH := font.Height() + 4
	gutterW := 6 * cellW
	left := 12 + gutterW
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
	lineStyles := map[int][]tokenStyle(nil)
	if app.syntaxHL != nil {
		lineStyles = app.syntaxHL.lineStyleFor(app.currentPath, app.ed.Buf, lines)
	}
	lineErrs := map[int]struct{}(nil)
	if app.syntaxCheck != nil {
		lineErrs = app.syntaxCheck.lineErrorsFor(app.currentPath, app.ed.Buf)
	}
	bufStatus := bufferLabel(app)
	curDir := app.openRoot
	if curDir == "" {
		if cwd, err := os.Getwd(); err == nil {
			curDir = cwd
		}
	}

	// Build condensed status line
	parts := []string{bufStatus}
	parts = append(parts, fmt.Sprintf("lang=%s", bufferLanguageMode(app.currentPath, app.ed.Buf)))
	if len(app.buffers) > 0 && app.buffers[app.bufIdx].dirty {
		parts = append(parts, "*unsaved*")
	}
	if app.open.Active {
		parts = append(parts, fmt.Sprintf("OPEN query=%q matches=%d", app.open.Query, len(app.open.Matches)))
	} else if app.ed.Leap.Active {
		dirArrow := "→"
		if app.ed.Leap.Dir == editor.DirBack {
			dirArrow = "←"
		}
		parts = append(parts, fmt.Sprintf("LEAP %s selecting=%v src=%s query=%q last=%q", dirArrow, app.ed.Leap.Selecting, app.ed.Leap.LastSrc, string(app.ed.Leap.Query), string(app.ed.Leap.LastCommit)))
	} else {
		parts = append(parts, fmt.Sprintf("EDIT last=%q", string(app.ed.Leap.LastCommit)))
	}
	if curDir != "" {
		parts = append(parts, fmt.Sprintf("cwd=%s", curDir))
	}
	status := "gc| " + strings.Join(parts, " | ")
	rightText := app.lastEvent

	infoBarH := lineH + 8

	// Text start
	textTop := top
	y := textTop

	// Help overlay
	if app.showHelp {
		helpY := textTop
		drawText(r, font, left, helpY-lineH, "Shortcuts", blue)
		y = drawHelp(r, font, left, helpY, fg) + lineH
	}

	contentTop := y
	usableHeight := h - contentTop - infoBarH*2 - 16
	if usableHeight < lineH {
		usableHeight = lineH
	}
	visibleLines := maxInt(1, (usableHeight+lineH-1)/lineH)
	cLine := editor.CaretLineAt(lines, app.ed.Caret)
	cCol := editor.CaretColAt(lines, app.ed.Caret)
	ensureCaretVisible(app, cLine, len(lines), visibleLines)
	startLine := clamp(app.scrollLine, 0, maxInt(0, len(lines)-visibleLines))
	app.scrollLine = startLine
	y = contentTop

	// Gutter background
	gutterX := left - gutterW
	r.SetDrawColor(bg.R, bg.G, bg.B, bg.A)
	_ = r.FillRect(&sdl.Rect{X: int32(gutterX), Y: int32(contentTop), W: int32(gutterW - 6), H: int32(visibleLines * lineH)})

	// Draw selection background (monospace-based)
	if app.ed.Sel.Active {
		a, b := app.ed.Sel.Normalised()
		a = clamp(a, 0, len(app.ed.Buf))
		b = clamp(b, 0, len(app.ed.Buf))
		drawSelectionRects(r, lines, app.ed.Buf, a, b, left, contentTop, lineH, cellW, selCol, startLine, visibleLines, tabWidth)
	}

	drawn := 0
	lnDim := sdl.Color{R: 130, G: 115, B: 160, A: 255}
	lnBright := sdl.Color{R: 227, G: 207, B: 255, A: 255}
	for i := startLine; i < len(lines) && drawn < visibleLines; i++ {
		line := lines[i]
		lnText := fmt.Sprintf("%4d ", i+1)
		lnCol := lnDim
		if i == cLine {
			lnCol = lnBright
			// Highlight current line background
			r.SetDrawColor(selCol.R, selCol.G, selCol.B, selCol.A)
			_ = r.FillRect(&sdl.Rect{X: int32(left - gutterW), Y: int32(y), W: int32(w - (left - gutterW)), H: int32(lineH)})
		}
		if _, ok := lineErrs[i]; ok {
			r.SetDrawColor(syntaxErr.R, syntaxErr.G, syntaxErr.B, syntaxErr.A)
			mw := max(4, cellW/3)
			mh := max(4, lineH/3)
			my := y + (lineH-mh)/2
			_ = r.FillRect(&sdl.Rect{
				X: int32(left - gutterW + 2),
				Y: int32(my),
				W: int32(mw),
				H: int32(mh),
			})
		}
		drawText(r, font, left-gutterW, y, lnText, lnCol)
		drawStyledLine(
			r,
			font,
			left,
			y,
			line,
			lineStyles[i],
			cellW,
			fg,
			syntaxKeyword,
			syntaxType,
			syntaxFunc,
			syntaxString,
			syntaxNumber,
			syntaxComment,
			syntaxHeading,
			syntaxLink,
			syntaxPunctuation,
		)

		if i == cLine && blinkOn {
			cColVis := visualColForRuneCol(line, cCol, tabWidth)
			x := left + cColVis*cellW
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
		drawn++
	}

	// underline found position while leaping
	if app.ed.Leap.Active && app.ed.Leap.LastFoundPos >= 0 {
		fLine := editor.CaretLineAt(lines, app.ed.Leap.LastFoundPos)
		fCol := editor.CaretColAt(lines, app.ed.Leap.LastFoundPos)
		rel := fLine - startLine
		if rel >= 0 && rel < visibleLines {
			yFound := contentTop + rel*lineH
			x := left + visualColForRuneCol(lines[fLine], fCol, tabWidth)*cellW
			yy := yFound + lineH - 3
			r.SetDrawColor(green.R, green.G, green.B, green.A)
			_ = r.DrawLine(int32(x), int32(yy), int32(x+cellW), int32(yy))
		}
	}

	drawSymbolInfoPopup(r, font, app, w, h, infoBarH)

	// Status bar above input line (inverted colors)
	barY := h - infoBarH*2
	barBg := sdl.Color{R: 35, G: 18, B: 43, A: 255}    // darker info bar
	barFg := sdl.Color{R: 201, G: 182, B: 242, A: 255} // #C9B6F2
	r.SetDrawColor(barBg.R, barBg.G, barBg.B, barBg.A)
	_ = r.FillRect(&sdl.Rect{X: 0, Y: int32(barY), W: int32(w), H: int32(infoBarH)})
	drawText(r, font, left, barY+4, status, barFg)
	if rightText != "" {
		if wtxt, _, err := font.SizeUTF8(rightText); err == nil {
			rx := w - wtxt - left
			if rx < left {
				rx = left
			}
			drawText(r, font, rx, barY+4, rightText, barFg)
		}
	}

	// Input line at very bottom
	inputY := h - infoBarH
	r.SetDrawColor(bg.R, bg.G, bg.B, bg.A)
	_ = r.FillRect(&sdl.Rect{X: 0, Y: int32(inputY), W: int32(w), H: int32(infoBarH)})
	if app.inputActive {
		drawText(r, font, left, inputY+4, fmt.Sprintf("%s%s", app.inputPrompt, app.inputValue), fg)
	}

	r.Present()
}

func drawSelectionRects(r *sdl.Renderer, lines []string, buf []rune, a, b int, left, y0, lineH, cellW int, col sdl.Color, startLine, visibleLines, tabW int) {
	aLine, aCol := editor.LineColForPos(lines, a)
	bLine, bCol := editor.LineColForPos(lines, b)

	r.SetDrawColor(col.R, col.G, col.B, col.A)

	visStart := startLine
	visEnd := startLine + visibleLines - 1
	for ln := aLine; ln <= bLine; ln++ {
		if ln < visStart || ln > visEnd {
			continue
		}
		y := y0 + (ln-startLine)*lineH
		startCol := 0
		endCol := visualLen(lines[ln], tabWidth)
		if ln == aLine {
			startCol = visualColForRuneCol(lines[ln], aCol, tabWidth)
		}
		if ln == bLine {
			endCol = visualColForRuneCol(lines[ln], bCol, tabWidth)
		}
		if endCol < startCol {
			startCol, endCol = endCol, startCol
		}
		x1 := left + startCol*cellW
		x2 := left + endCol*cellW
		_ = r.FillRect(&sdl.Rect{X: int32(x1), Y: int32(y), W: int32(maxInt(2, x2-x1)), H: int32(lineH)})
	}

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

func drawSymbolInfoPopup(r *sdl.Renderer, font *ttf.Font, app *appState, winW, winH, infoBarH int) {
	if app == nil || strings.TrimSpace(app.symbolInfoPopup) == "" {
		return
	}
	text := app.symbolInfoPopup
	const pad = 12
	maxW := max(320, winW-80)
	lines := wrapPopupText(text, 70)
	if len(lines) == 0 {
		lines = []string{text}
	}
	lineH := font.Height() + 4
	maxH := winH - infoBarH*2 - 40
	boxH := min(maxH, pad*2+lineH*14)
	minH := pad*2 + lineH*3
	if boxH < minH {
		boxH = minH
	}
	boxW := maxW
	x := (winW - boxW) / 2
	y := max(20, (winH-infoBarH*2-boxH)/2)

	bg := sdl.Color{R: 28, G: 14, B: 35, A: 245}
	border := sdl.Color{R: 160, G: 126, B: 214, A: 255}
	fg := sdl.Color{R: 228, G: 214, B: 255, A: 255}
	dim := sdl.Color{R: 175, G: 154, B: 214, A: 255}

	r.SetDrawColor(bg.R, bg.G, bg.B, bg.A)
	_ = r.FillRect(&sdl.Rect{X: int32(x), Y: int32(y), W: int32(boxW), H: int32(boxH)})
	r.SetDrawColor(border.R, border.G, border.B, border.A)
	_ = r.DrawRect(&sdl.Rect{X: int32(x), Y: int32(y), W: int32(boxW), H: int32(boxH)})

	drawText(r, font, x+pad, y+pad, "Symbol Info", fg)

	contentTop := y + pad + lineH
	footerY := y + boxH - pad - lineH
	contentHeight := max(lineH, footerY-contentTop)
	visible := max(1, contentHeight/lineH)
	maxScroll := max(0, len(lines)-visible)
	app.symbolInfoScroll = clamp(app.symbolInfoScroll, 0, maxScroll)

	start := app.symbolInfoScroll
	end := min(len(lines), start+visible)
	for i := start; i < end; i++ {
		yy := contentTop + (i-start)*lineH
		if yy+lineH > footerY {
			break
		}
		drawText(r, font, x+pad, yy, lines[i], fg)
	}
	hint := "Esc close | Up/Down scroll"
	if maxScroll > 0 {
		hint = fmt.Sprintf("Esc close | Up/Down scroll (%d-%d/%d)", start+1, end, len(lines))
	}
	drawText(r, font, x+pad, footerY, hint, dim)
}

func wrapPopupText(text string, maxChars int) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	if maxChars <= 10 {
		return []string{text}
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{text}
	}
	out := make([]string, 0, 4)
	cur := words[0]
	for i := 1; i < len(words); i++ {
		w := words[i]
		if len(cur)+1+len(w) <= maxChars {
			cur += " " + w
			continue
		}
		out = append(out, cur)
		cur = w
	}
	out = append(out, cur)
	return out
}

func bufferLanguageMode(path string, buf []rune) string {
	switch detectSyntax(path, string(buf)) {
	case syntaxGo:
		return "go"
	case syntaxMarkdown:
		return "markdown"
	case syntaxC:
		return "c"
	case syntaxMiranda:
		return "miranda"
	default:
		return "text"
	}
}

func tryManualCompletion(app *appState) bool {
	if app == nil || app.ed == nil || app.inputActive || app.open.Active || app.ed.Leap.Active {
		return false
	}
	if detectSyntax(app.currentPath, string(app.ed.Buf)) != syntaxGo {
		return false
	}
	prefixStart := identPrefixStart(app.ed.Buf, app.ed.Caret)
	prefix := string(app.ed.Buf[prefixStart:app.ed.Caret])
	if len(prefix) < 1 {
		return false
	}
	lines := editor.SplitLines(app.ed.Buf)
	line := editor.CaretLineAt(lines, app.ed.Caret)
	col := editor.CaretColAt(lines, app.ed.Caret)
	if line < 0 || col < 0 {
		return false
	}
	items := []completionItem(nil)
	if !app.noGopls {
		got, err := app.gopls.complete(app.currentPath, string(app.ed.Buf), line, col)
		if err != nil {
			app.noGopls = true
			app.lastEvent = "Autocomplete disabled (gopls unavailable)"
		} else {
			items = got
		}
	}
	item, ok := extremelySureCompletion(prefix, items, 1)
	if ok {
		applyCompletionText(app, prefixStart, item.Insert)
		return true
	}

	// Fallback: deterministic keyword completion when there's exactly one match.
	if kw, ok := goKeywordFallback(prefix); ok {
		applyCompletionText(app, prefixStart, kw)
		return true
	}
	return false
}

func applyCompletionText(app *appState, prefixStart int, insertText string) {
	insert := []rune(insertText)
	next := make([]rune, 0, len(app.ed.Buf)-(app.ed.Caret-prefixStart)+len(insert))
	next = append(next, app.ed.Buf[:prefixStart]...)
	next = append(next, insert...)
	next = append(next, app.ed.Buf[app.ed.Caret:]...)
	app.ed.Buf = next
	app.ed.Caret = prefixStart + len(insert)
	app.markDirty()
}

func extremelySureCompletion(prefix string, items []completionItem, minPrefix int) (completionItem, bool) {
	if len(items) != 1 || len(prefix) < minPrefix {
		return completionItem{}, false
	}
	item := items[0]
	insert := item.Insert
	if insert == "" {
		insert = item.Label
	}
	if len(insert) <= len(prefix) {
		return completionItem{}, false
	}
	if !strings.HasPrefix(insert, prefix) {
		// For snippet/punctuation inserts, fallback to label if label is a strict prefix match.
		if strings.HasPrefix(item.Label, prefix) && isSimpleIdent(item.Label) {
			item.Insert = item.Label
			return item, true
		}
		return completionItem{}, false
	}
	if !isSimpleIdent(insert) {
		if strings.HasPrefix(item.Label, prefix) && isSimpleIdent(item.Label) {
			item.Insert = item.Label
			return item, true
		}
		return completionItem{}, false
	}
	item.Insert = insert
	return item, true
}

func isSimpleIdent(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return false
	}
	return true
}

func goKeywordFallback(prefix string) (string, bool) {
	if prefix == "" {
		return "", false
	}
	match := ""
	for kw := range goKeywordTokens {
		if strings.HasPrefix(kw, prefix) {
			if match != "" {
				return "", false
			}
			match = kw
		}
	}
	if match == "" {
		return "", false
	}
	return match, true
}

func drawStyledLine(
	r *sdl.Renderer,
	font *ttf.Font,
	x, y int,
	line string,
	lineStyle []tokenStyle,
	cellW int,
	fg sdl.Color,
	keyword sdl.Color,
	typ sdl.Color,
	function sdl.Color,
	str sdl.Color,
	number sdl.Color,
	comment sdl.Color,
	heading sdl.Color,
	link sdl.Color,
	punctuation sdl.Color,
) {
	runes := []rune(line)
	if len(runes) == 0 {
		return
	}
	curStyle := styleDefault
	curX := x
	var sb strings.Builder
	flush := func() {
		if sb.Len() == 0 {
			return
		}
		chunk := sb.String()
		drawText(r, font, curX, y, chunk, styleColor(curStyle, fg, keyword, typ, function, str, number, comment, heading, link, punctuation))
		curX += utf8.RuneCountInString(chunk) * cellW
		sb.Reset()
	}

	visualCol := 0
	for i, ru := range runes {
		style := styleDefault
		if i < len(lineStyle) {
			style = lineStyle[i]
		}
		if i == 0 {
			curStyle = style
		}
		if style != curStyle {
			flush()
			curStyle = style
		}
		if ru == '\t' {
			nextTab := ((visualCol / tabWidth) + 1) * tabWidth
			for visualCol < nextTab {
				sb.WriteByte(' ')
				visualCol++
			}
			continue
		}
		sb.WriteRune(ru)
		visualCol++
	}
	flush()
}

func styleColor(
	style tokenStyle,
	fg sdl.Color,
	keyword sdl.Color,
	typ sdl.Color,
	function sdl.Color,
	str sdl.Color,
	number sdl.Color,
	comment sdl.Color,
	heading sdl.Color,
	link sdl.Color,
	punctuation sdl.Color,
) sdl.Color {
	switch style {
	case styleKeyword:
		return keyword
	case styleType:
		return typ
	case styleFunction:
		return function
	case styleString:
		return str
	case styleNumber:
		return number
	case styleComment:
		return comment
	case styleHeading:
		return heading
	case styleLink:
		return link
	case stylePunctuation:
		return punctuation
	default:
		return fg
	}
}

// ======================
// Input helpers
// ======================

func textInputString(e *sdl.TextInputEvent) string {
	return C.GoString((*C.char)(unsafe.Pointer(&e.Text[0])))
}

// expandTabs renders tabs as fixed-width spaces for UI display.
func expandTabs(s string, width int) string {
	if width <= 0 {
		return s
	}
	var sb strings.Builder
	col := 0
	for _, r := range s {
		if r == '\t' {
			next := ((col / width) + 1) * width
			for col < next {
				sb.WriteByte(' ')
				col++
			}
			continue
		}
		sb.WriteRune(r)
		col++
	}
	return sb.String()
}

// visualColForRuneCol converts a rune column to a visual column accounting for tabs.
func visualColForRuneCol(line string, runeCol, width int) int {
	if width <= 0 {
		return runeCol
	}
	col := 0
	vis := 0
	for _, r := range line {
		if col >= runeCol {
			break
		}
		if r == '\t' {
			vis = ((vis / width) + 1) * width
		} else {
			vis++
		}
		col++
	}
	return vis
}

// visualLen returns the display width of a line with tabs expanded.
func visualLen(line string, width int) int {
	return visualColForRuneCol(line, len([]rune(line)), width)
}

func modsString(m modMask) string {
	parts := ""
	add := func(s string) {
		if parts != "" {
			parts += "|"
		}
		parts += s
	}
	if (m & modShift) != 0 {
		add("LSHIFT")
	}
	if (m & modCtrl) != 0 {
		add("LCTRL")
	}
	if (m & modLCmd) != 0 {
		add("LCMD")
	}
	if (m & modRCmd) != 0 {
		add("RCMD")
	}
	if (m & modLAlt) != 0 {
		add("LALT")
	}
	if (m & modRAlt) != 0 {
		add("RALT")
	}
	if parts == "" {
		return "none"
	}
	return parts
}

func sdlToMods(m sdl.Keymod) modMask {
	var out modMask
	if (m & sdl.KMOD_SHIFT) != 0 {
		out |= modShift
	}
	if (m & sdl.KMOD_CTRL) != 0 {
		out |= modCtrl
	}
	if (m & sdl.KMOD_LGUI) != 0 {
		out |= modLCmd
	}
	if (m & sdl.KMOD_RGUI) != 0 {
		out |= modRCmd
	}
	if (m & sdl.KMOD_LALT) != 0 {
		out |= modLAlt
	}
	if (m & sdl.KMOD_RALT) != 0 {
		out |= modRAlt
	}
	return out
}

func sdlKeyEvent(e *sdl.KeyboardEvent) keyEvent {
	if e == nil {
		return keyEvent{}
	}
	return keyEvent{
		down:   e.Type == sdl.KEYDOWN,
		repeat: int(e.Repeat),
		key:    sdlToKey(e.Keysym.Sym),
		mods:   sdlToMods(sdl.GetModState()),
	}
}

func sdlToKey(k sdl.Keycode) keyCode {
	switch k {
	case sdl.K_UP:
		return keyUp
	case sdl.K_DOWN:
		return keyDown
	case sdl.K_PAGEUP:
		return keyPageUp
	case sdl.K_PAGEDOWN:
		return keyPageDown
	case sdl.K_HOME:
		return keyHome
	case sdl.K_END:
		return keyEnd
	case sdl.K_ESCAPE:
		return keyEscape
	case sdl.K_TAB:
		return keyTab
	case sdl.K_BACKSPACE:
		return keyBackspace
	case sdl.K_DELETE:
		return keyDelete
	case sdl.K_RETURN:
		return keyReturn
	case sdl.K_KP_ENTER:
		return keyKpEnter
	case sdl.K_LEFT:
		return keyLeft
	case sdl.K_RIGHT:
		return keyRight
	case sdl.K_LGUI:
		return keyLcmd
	case sdl.K_RGUI:
		return keyRcmd
	case sdl.K_SPACE:
		return keySpace
	case sdl.K_PERIOD:
		return keyPeriod
	case sdl.K_COMMA:
		return keyComma
	case sdl.K_MINUS:
		return keyMinus
	case sdl.K_EQUALS:
		return keyEquals
	case sdl.K_SLASH:
		return keySlash
	case sdl.K_q:
		return keyQ
	case sdl.K_b:
		return keyB
	case sdl.K_w:
		return keyW
	case sdl.K_f:
		return keyF
	case sdl.K_s:
		return keyS
	case sdl.K_r:
		return keyR
	case sdl.K_a:
		return keyA
	case sdl.K_e:
		return keyE
	case sdl.K_k:
		return keyK
	case sdl.K_u:
		return keyU
	case sdl.K_i:
		return keyI
	case sdl.K_o:
		return keyO
	case sdl.K_l:
		return keyL
	case sdl.K_c:
		return keyC
	case sdl.K_x:
		return keyX
	case sdl.K_v:
		return keyV
	case sdl.K_0:
		return key0
	case sdl.K_1:
		return key1
	case sdl.K_2:
		return key2
	case sdl.K_3:
		return key3
	case sdl.K_4:
		return key4
	case sdl.K_5:
		return key5
	case sdl.K_6:
		return key6
	case sdl.K_7:
		return key7
	case sdl.K_8:
		return key8
	case sdl.K_9:
		return key9
	case sdl.K_d:
		return keyD
	case sdl.K_g:
		return keyG
	case sdl.K_h:
		return keyH
	case sdl.K_j:
		return keyJ
	case sdl.K_m:
		return keyM
	case sdl.K_n:
		return keyN
	case sdl.K_p:
		return keyP
	case sdl.K_t:
		return keyT
	case sdl.K_y:
		return keyY
	case sdl.K_z:
		return keyZ
	default:
		return keyUnknown
	}
}

// ======================
// Fonts + util
// ======================

func pickFont() string {
	local := filepath.Join("font", "JetBrainsMono-Regular.ttf")
	if _, err := os.Stat(local); err == nil {
		return local
	}
	panic("JetBrainsMono-Regular.ttf not found in ./font; ensure the font file is present")
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
func handleInputEvent(app *appState, ev sdl.Event) bool {
	switch e := ev.(type) {
	case *sdl.KeyboardEvent:
		return handleInputKey(app, sdlKeyEvent(e))
	case *sdl.TextInputEvent:
		text := textInputString(e)
		return handleInputText(app, text)
	default:
		return true
	}
}
