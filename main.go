package main

import (
	"bufio"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	pathpkg "path"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unsafe"

	"gc/editor"
)

const debug = false
const tabWidth = 4

type bufferSlot struct {
	ed   *editor.Editor
	path string
	// picker buffers are temporary file-list views
	picker     bool
	pickerRoot string
	dirty      bool
	rev        int
	textRev    int
	mode       syntaxKind
	// Per-buffer cached render data keyed by textRev/mode/path.
	cachedTextRev    int
	cachedMode       syntaxKind
	cachedPath       string
	cachedLines      []string
	cachedLineStyles [][]tokenStyle
	cachedLangMode   string
	// Per-buffer cached syntax-check data keyed by textRev/mode/path.
	syntaxErrTextRev int
	syntaxErrPath    string
	syntaxErrMode    syntaxKind
	syntaxErrLines   map[int]struct{}
	syntaxErrMsgs    map[int]string
}

type renderCache struct {
	bufIdx     int
	textRev    int
	mode       syntaxKind
	path       string
	lines      []string
	lineStarts []int
	lineStyles [][]tokenStyle
	langMode   string
}

type appState struct {
	ed               *editor.Editor
	lastEvent        string
	lastMods         modMask
	blinkAt          time.Time
	lastSpaceAt      time.Time
	lastSpaceLn      int
	inputActive      bool
	inputPrompt      string
	inputValue       string
	inputKind        string
	openRoot         string
	open             openPrompt
	buffers          []bufferSlot
	bufIdx           int
	currentPath      string
	scrollLine       int
	symbolInfoPopup  string
	symbolInfoScroll int
	syntaxHL         *syntaxHighlighter
	syntaxCheck      *goSyntaxChecker
	gopls            *goplsClient
	noGopls          bool
	clipboard        editor.Clipboard
	cmdPrefixActive  bool
	suppressTextOnce bool
	lessMode         bool
	escSeqActive     bool
	escSeq           string
	// Esc-prefix delayed helper popup state.
	escHelpVisible   bool
	escPrefixAt      time.Time
	escHelpToken     int
	escHelpDelay     time.Duration
	requestInterrupt func(any)
	// Line-highlight mode state.
	lineHighlightMode       bool
	lineHighlightAnchorLine int
	lineHighlightToLine     int
	// Incremental search state.
	searchActive      bool
	searchQuery       []rune
	lastSearchQuery   []rune
	searchPatternDone bool
	searchOrigin      int
	searchLastMatch   int
	completionPopup   completionPopupState
	render            renderCache
	startupFast       bool
}

type completionPopupState struct {
	active        bool
	title         string
	items         []completionItem
	selected      int
	replaceStart  int
	replaceEnd    int
	detailText    string
	detailVisible bool
	detailArmedAt time.Time
	detailToken   int
	detailDelay   time.Duration
}

type completionDetailInterrupt struct {
	Token int
}

type helpEntry struct {
	action string
	keys   string
}

var helpEntries = []helpEntry{
	{"Leap forward / backward", "Unbound in TUI mode"},
	{"Leap Again", "N/A in TUI mode"},
	{"New buffer / cycle buffers", "Ctrl+B / Shift+Tab"},
	{"File picker / load line path", "Ctrl+O / Ctrl+L"},
	{"Write as / save all", "Esc+W / Esc+Shift+S"},
	{"Save + fmt/fix + reload", "Esc+F"},
	{"Run package (go run .)", "Ctrl+R"},
	{"Close buffer / quit", "Ctrl+Q / Esc+Shift+Q"},
	{"Undo", "Ctrl+U"},
	{"Comment / uncomment", "Ctrl+/ (selection or current line)"},
	{"Line start / end", "Ctrl+A / Ctrl+E (Shift = select)"},
	{"Buffer start / end", "Ctrl+Shift+A / Ctrl+Shift+E"},
	{"Kill to EOL", "Ctrl+K"},
	{"Copy / Cut / Paste", "Ctrl+C / Ctrl+X / Ctrl+V"},
	{"Symbol info under cursor (Go)", "Esc+I"},
	{"Cycle language mode", "Esc+M"},
	{"Search mode", "Esc+/ then type pattern; / locks; Tab/Shift+Tab navigate; x enters line highlight mode"},
	{"Line highlight mode", "Esc+X (or x from locked search), then x to extend by line; Esc exits"},
	{"Autocomplete (Go mode)", "Tab"},
	{"Less mode", "Esc+Space (Space page, Esc exit)"},
	{"Navigation", "Arrows, PageUp/Down, Ctrl+, Ctrl+. (Shift = select)"},
	{"Delete buffer contents", "Esc+Shift+Delete"},
	{"Escape", "Closes symbol info popup or exits less mode; otherwise command prefix (Esc then Esc closes current buffer)"},
	{"Help buffer", "Ctrl+Shift+/ (Ctrl+?)"},
}

type openPrompt struct {
	Active  bool
	Query   string
	Matches []string
}

func (app *appState) initBuffers(ed *editor.Editor) {
	app.buffers = []bufferSlot{{ed: ed, rev: 1, textRev: 1}}
	app.bufIdx = 0
	app.ed = ed
	app.currentPath = ""
	app.lastSpaceLn = -1
	app.render = renderCache{}
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
	nb := bufferSlot{ed: editor.NewEditor(""), rev: 1, textRev: 1}
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
		rev:        1,
		textRev:    1,
		mode:       syntaxNone,
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
	app.buffers[app.bufIdx].rev++
	app.buffers[app.bufIdx].textRev++
	app.buffers[app.bufIdx].dirty = true
	app.buffers[app.bufIdx].syntaxErrTextRev = 0
	app.buffers[app.bufIdx].syntaxErrPath = ""
	app.buffers[app.bufIdx].syntaxErrMode = syntaxNone
	app.buffers[app.bufIdx].syntaxErrLines = nil
	app.buffers[app.bufIdx].syntaxErrMsgs = nil
}

func (app *appState) touchBuffer(idx int) {
	if app == nil || idx < 0 || idx >= len(app.buffers) {
		return
	}
	app.buffers[idx].rev++
}

func (app *appState) touchBufferText(idx int) {
	if app == nil || idx < 0 || idx >= len(app.buffers) {
		return
	}
	app.buffers[idx].rev++
	app.buffers[idx].textRev++
	app.buffers[idx].syntaxErrTextRev = 0
	app.buffers[idx].syntaxErrPath = ""
	app.buffers[idx].syntaxErrMode = syntaxNone
	app.buffers[idx].syntaxErrLines = nil
	app.buffers[idx].syntaxErrMsgs = nil
}

func (app *appState) touchActiveBuffer() {
	app.touchBuffer(app.bufIdx)
}

func (app *appState) touchActiveBufferText() {
	app.touchBufferText(app.bufIdx)
}

func (app *appState) switchBuffer(delta int) {
	if len(app.buffers) == 0 {
		return
	}
	n := len(app.buffers)
	app.bufIdx = (app.bufIdx + delta + n) % n
	app.syncActiveBuffer()
}

func (app *appState) closeBuffer() int {
	if app == nil || len(app.buffers) == 0 {
		return 0
	}
	app.buffers = append(app.buffers[:app.bufIdx], app.buffers[app.bufIdx+1:]...)
	if app.bufIdx >= len(app.buffers) {
		app.bufIdx = len(app.buffers) - 1
	}
	app.syncActiveBuffer()
	app.open = openPrompt{}
	return len(app.buffers)
}

func saveCurrent(app *appState) error {
	if app == nil || app.ed == nil || len(app.buffers) == 0 {
		return fmt.Errorf("no editor to save")
	}
	path := app.currentPath
	if path == "" {
		promptSaveAs(app)
		return fmt.Errorf("no path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(app.ed.String()), 0644); err != nil {
		return err
	}
	app.buffers[app.bufIdx].path = path
	app.buffers[app.bufIdx].dirty = false
	app.touchActiveBuffer()
	return nil
}

func promptSaveAs(app *appState) {
	if app == nil {
		return
	}
	app.inputActive = true
	app.inputPrompt = "Save as: "
	app.inputValue = ""
	app.inputKind = "save"
	app.lastEvent = "Save: enter filename in input line, Enter to confirm, Esc to cancel"
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
var completeGoCompletions = func(app *appState, path string, content string, line int, col int) ([]completionItem, error) {
	if app == nil || app.gopls == nil {
		return nil, fmt.Errorf("gopls unavailable")
	}
	return app.gopls.complete(path, content, line, col)
}

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
	runIdx := app.bufIdx
	app.buffers[app.bufIdx].path = title
	app.buffers[app.bufIdx].dirty = false
	app.currentPath = title
	runEd := app.ed
	runEd.SetRunes([]rune(fmt.Sprintf("$ (cd %s && go run .)\n\n", dir)))
	runEd.Caret = runEd.RuneLen()
	runEd.Sel = editor.Sel{}
	app.touchBufferText(runIdx)

	appendOut := func(s string) {
		appendRunOutput(runEd, s)
		app.touchBufferText(runIdx)
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
	ed.Caret = ed.RuneLen()
	ed.InsertText(s)
	ed.Caret = ed.RuneLen()
}

func goFmtAndFix(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("no file path")
	}
	errList := make([]string, 0, 2)

	fmtCmd := exec.Command("gofmt", "-w", path)
	if out, err := fmtCmd.CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		errList = append(errList, "gofmt: "+msg)
	}

	fixCmd := exec.Command("go", "fix", path)
	fixCmd.Dir = filepath.Dir(path)
	if out, err := fixCmd.CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		errList = append(errList, "go fix: "+msg)
	}

	if len(errList) > 0 {
		return errors.New(strings.Join(errList, "; "))
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
	buf, err := readFileRunes(path)
	if err != nil {
		return err
	}
	app.ed.SetRunes(buf)
	app.ed.Caret = clamp(app.ed.Caret, 0, app.ed.RuneLen())
	app.ed.Sel = editor.Sel{}
	app.ed.Leap = editor.LeapState{LastFoundPos: -1}
	app.buffers[app.bufIdx].dirty = false
	app.buffers[app.bufIdx].path = path
	app.touchActiveBufferText()
	return nil
}

func openPath(app *appState, path string) error {
	if app == nil || app.ed == nil || len(app.buffers) == 0 {
		return fmt.Errorf("no active buffer")
	}
	buf, err := readFileRunes(path)
	if err != nil {
		return err
	}
	if app.openRoot != "" {
		if rel, err := filepath.Rel(app.openRoot, path); err != nil || strings.HasPrefix(rel, "..") {
			return fmt.Errorf("refusing to open outside %s", app.openRoot)
		}
	}
	app.currentPath = path
	app.buffers[app.bufIdx].path = path
	app.buffers[app.bufIdx].dirty = false
	app.ed.SetRunes(buf)
	app.ed.Caret = 0
	app.ed.Sel = editor.Sel{}
	app.ed.Leap = editor.LeapState{LastFoundPos: -1}
	app.touchActiveBufferText()
	return nil
}

func readFileRunes(path string) ([]rune, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return bytesToRunes(data), nil
}

func bytesToRunes(data []byte) []rune {
	if len(data) == 0 {
		return nil
	}
	// Avoid an extra byte-to-string copy when decoding file content into runes.
	s := unsafe.String(unsafe.SliceData(data), len(data))
	return []rune(s)
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

func loadFileAtCaret(app *appState) error {
	if app == nil || app.ed == nil || len(app.buffers) == 0 {
		return fmt.Errorf("no active buffer")
	}
	slot := &app.buffers[app.bufIdx]
	lines := editor.SplitLines(app.ed.Runes())
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
		slot.ed.SetRunes([]rune(strings.Join(list, "\n")))
		app.touchActiveBufferText()
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
		slot.ed.SetRunes([]rune(strings.Join(list, "\n")))
		app.touchActiveBufferText()
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

	for i, b := range app.buffers {
		if filepath.Clean(b.path) == filepath.Clean(full) {
			app.bufIdx = i
			app.syncActiveBuffer()
			return nil
		}
	}

	app.addBuffer()
	app.openRoot = filepath.Dir(full)
	return openPath(app, full)
}

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
		if strings.Contains(strings.ToLower(d.Name()), lq) {
			matches = append(matches, path)
		}
		return nil
	})
	return matches
}

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
	sort.Strings(entries[1:])
	return entries, nil
}

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
			app.currentPath = abs
			app.buffers[app.bufIdx].path = abs
			app.ed.SetRunes(nil)
			app.buffers[app.bufIdx].dirty = false
			app.touchActiveBufferText()
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
	oldLines := editor.SplitLines(ed.Runes())
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
	for i := range deltas {
		cum[i+1] = cum[i] + deltas[i]
	}
	adjustPos := func(oldPos int) int {
		ln, _ := editor.LineColForPos(oldLines, oldPos)
		if ln < 0 || ln >= len(oldLines) {
			return oldPos
		}
		return oldPos + cum[ln] + deltas[ln]
	}

	ed.SetRunes([]rune(strings.Join(lines, "\n")))
	if origSel.Active {
		ed.Sel.Active = true
		ed.Sel.A = adjustPos(selA)
		ed.Sel.B = adjustPos(selB)
	} else {
		ed.Sel.Active = false
	}
	ed.Caret = adjustPos(ed.Caret)
	ed.Caret = clamp(ed.Caret, 0, ed.RuneLen())
}

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

func wrapPopupText(text string, maxChars int) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	if maxChars <= 1 {
		return []string{strings.TrimSpace(text)}
	}
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	for _, raw := range lines {
		line := strings.TrimRight(raw, " \t\r")
		if line == "" {
			out = append(out, "")
			continue
		}
		rs := []rune(line)
		for len(rs) > 0 {
			if len(rs) <= maxChars {
				out = append(out, string(rs))
				break
			}
			cut := maxChars
			for i := maxChars; i > 0; i-- {
				if rs[i-1] == ' ' || rs[i-1] == '\t' {
					cut = i
					break
				}
			}
			part := strings.TrimSpace(string(rs[:cut]))
			if part == "" {
				part = string(rs[:maxChars])
				rs = rs[maxChars:]
			} else {
				rs = rs[cut:]
			}
			out = append(out, part)
			rs = []rune(strings.TrimLeft(string(rs), " \t"))
		}
	}
	return out
}

func syntaxKindLabel(kind syntaxKind) string {
	switch kind {
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

func bufferSyntaxKind(app *appState, path string, buf []rune) syntaxKind {
	if app != nil && app.bufIdx >= 0 && app.bufIdx < len(app.buffers) {
		if forced := app.buffers[app.bufIdx].mode; forced != syntaxNone {
			return forced
		}
	}
	return detectSyntax(path, string(buf))
}

func activeBufferSyntaxErrors(app *appState, kind syntaxKind, path string) (map[int]struct{}, map[int]string) {
	if app == nil || app.ed == nil || app.syntaxCheck == nil || kind != syntaxGo {
		return nil, nil
	}
	if app.bufIdx < 0 || app.bufIdx >= len(app.buffers) {
		return nil, nil
	}
	slot := &app.buffers[app.bufIdx]
	if slot.syntaxErrTextRev == slot.textRev &&
		slot.syntaxErrMode == kind &&
		slot.syntaxErrPath == path {
		return slot.syntaxErrLines, slot.syntaxErrMsgs
	}
	lines := app.syntaxCheck.lineErrorsFor(path, app.ed.Runes())
	msgs := app.syntaxCheck.lineMsgs
	slot.syntaxErrTextRev = slot.textRev
	slot.syntaxErrMode = kind
	slot.syntaxErrPath = path
	slot.syntaxErrLines = lines
	slot.syntaxErrMsgs = msgs
	return lines, msgs
}

func cycleBufferMode(app *appState) string {
	if app == nil || app.bufIdx < 0 || app.bufIdx >= len(app.buffers) {
		return "text"
	}
	order := []syntaxKind{syntaxNone, syntaxGo, syntaxMarkdown, syntaxC, syntaxMiranda}
	cur := app.buffers[app.bufIdx].mode
	next := order[0]
	for i, k := range order {
		if k == cur {
			next = order[(i+1)%len(order)]
			break
		}
	}
	app.buffers[app.bufIdx].mode = next
	app.touchActiveBuffer()
	return syntaxKindLabel(next)
}

func tryManualCompletion(app *appState) bool {
	if app == nil || app.ed == nil || app.inputActive || app.open.Active || app.ed.Leap.Active {
		return false
	}
	buf := app.ed.Runes()
	if bufferSyntaxKind(app, app.currentPath, buf) != syntaxGo {
		return false
	}
	if app.completionPopup.active {
		return completionPopupApplySelection(app)
	}
	if tryImportedPackageNameExpansion(app, buf) {
		return true
	}
	if selectorPrefix, selectorStart, selectorEnd, ok := selectorCompletionPrefix(buf, app.ed.Caret); ok {
		return trySelectorCompletionPopup(app, buf, selectorPrefix, selectorStart, selectorEnd)
	}
	prefixStart := identPrefixStart(buf, app.ed.Caret)
	prefix := string(buf[prefixStart:app.ed.Caret])
	if len(prefix) < 1 {
		return false
	}
	// Fast path: deterministic keyword expansion avoids the initial gopls round trip.
	if kw, ok := goKeywordFallback(prefix); ok {
		applyCompletionText(app, prefixStart, kw)
		return true
	}
	lines := editor.SplitLines(buf)
	line := editor.CaretLineAt(lines, app.ed.Caret)
	col := editor.CaretColAt(lines, app.ed.Caret)
	if line < 0 || col < 0 {
		return false
	}
	items := []completionItem(nil)
	if !app.noGopls {
		got, err := completeGoCompletions(app, app.currentPath, string(buf), line, col)
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
	return false
}

func tryImportedPackageNameExpansion(app *appState, buf []rune) bool {
	prefixStart := identPrefixStart(buf, app.ed.Caret)
	prefix := string(buf[prefixStart:app.ed.Caret])
	if len(prefix) < 1 {
		return false
	}
	names := importedPackageNames(string(buf))
	match := ""
	for _, name := range names {
		if strings.HasPrefix(name, prefix) {
			if match != "" {
				return false
			}
			match = name
		}
	}
	if match == "" || match == prefix {
		return false
	}
	applyCompletionText(app, prefixStart, match)
	return true
}

func selectorCompletionPrefix(buf []rune, caret int) (prefix string, start int, end int, ok bool) {
	if caret < 0 || caret > len(buf) {
		return "", 0, 0, false
	}
	end = caret
	start = caret
	for start > 0 && isSimpleIdentRune(buf[start-1]) {
		start--
	}
	if start == 0 || buf[start-1] != '.' {
		return "", 0, 0, false
	}
	pkgEnd := start - 1
	pkgStart := pkgEnd
	for pkgStart > 0 && isSimpleIdentRune(buf[pkgStart-1]) {
		pkgStart--
	}
	if pkgStart == pkgEnd {
		return "", 0, 0, false
	}
	return string(buf[pkgStart:caret]), start, end, true
}

func trySelectorCompletionPopup(app *appState, buf []rune, prefix string, start int, end int) bool {
	lines := editor.SplitLines(buf)
	line := editor.CaretLineAt(lines, app.ed.Caret)
	col := editor.CaretColAt(lines, app.ed.Caret)
	if line < 0 || col < 0 {
		return false
	}
	items := []completionItem(nil)
	if !app.noGopls {
		got, err := completeGoCompletions(app, app.currentPath, string(buf), line, col)
		if err != nil {
			app.noGopls = true
			app.lastEvent = "Autocomplete disabled (gopls unavailable)"
			return false
		}
		items = got
	}
	if len(items) == 0 {
		return false
	}
	openCompletionPopup(app, "Completions for "+prefix, items, start, end)
	return true
}

func openCompletionPopup(app *appState, title string, items []completionItem, replaceStart, replaceEnd int) {
	if app == nil {
		return
	}
	app.completionPopup.active = true
	app.completionPopup.title = title
	app.completionPopup.items = append(app.completionPopup.items[:0], items...)
	app.completionPopup.selected = 0
	app.completionPopup.replaceStart = replaceStart
	app.completionPopup.replaceEnd = replaceEnd
	if app.completionPopup.detailDelay <= 0 {
		app.completionPopup.detailDelay = 700 * time.Millisecond
	}
	armCompletionPopupDetails(app)
	app.lastEvent = fmt.Sprintf("Completion: %d candidates (Tab/Shift+Tab, Enter)", len(items))
}

func closeCompletionPopup(app *appState) {
	if app == nil {
		return
	}
	app.completionPopup = completionPopupState{}
}

func completionPopupMove(app *appState, delta int) {
	if app == nil || !app.completionPopup.active || len(app.completionPopup.items) == 0 {
		return
	}
	n := len(app.completionPopup.items)
	app.completionPopup.selected = (app.completionPopup.selected + delta + n) % n
	armCompletionPopupDetails(app)
}

func completionPopupApplySelection(app *appState) bool {
	if app == nil || !app.completionPopup.active || len(app.completionPopup.items) == 0 {
		return false
	}
	sel := app.completionPopup.selected
	if sel < 0 || sel >= len(app.completionPopup.items) {
		sel = 0
	}
	item := app.completionPopup.items[sel]
	insert := item.Insert
	if insert == "" {
		insert = item.Label
	}
	cur := app.ed.Runes()
	start := clamp(app.completionPopup.replaceStart, 0, len(cur))
	end := clamp(app.completionPopup.replaceEnd, start, len(cur))
	ins := []rune(insert)
	next := make([]rune, 0, len(cur)-(end-start)+len(ins))
	next = append(next, cur[:start]...)
	next = append(next, ins...)
	next = append(next, cur[end:]...)
	app.ed.SetRunes(next)
	app.ed.Caret = start + len(ins)
	closeCompletionPopup(app)
	app.markDirty()
	app.lastEvent = "Completed"
	return true
}

func armCompletionPopupDetails(app *appState) {
	if app == nil || !app.completionPopup.active || len(app.completionPopup.items) == 0 {
		return
	}
	if app.completionPopup.detailDelay <= 0 {
		app.completionPopup.detailDelay = 700 * time.Millisecond
	}
	app.completionPopup.detailVisible = false
	app.completionPopup.detailText = ""
	app.completionPopup.detailArmedAt = time.Now()
	app.completionPopup.detailToken++
	if app.requestInterrupt == nil {
		return
	}
	token := app.completionPopup.detailToken
	delay := app.completionPopup.detailDelay
	post := app.requestInterrupt
	time.AfterFunc(delay, func() {
		post(completionDetailInterrupt{Token: token})
	})
}

func completionPopupDetailText(item completionItem) string {
	label := strings.TrimSpace(item.Label)
	if label == "" {
		label = strings.TrimSpace(item.Insert)
	}
	out := "Completion: " + label
	if detail := strings.TrimSpace(item.Detail); detail != "" {
		out += "\n\nSignature:\n" + strings.TrimSpace(strings.ReplaceAll(detail, "\n", " "))
	}
	if doc := strings.TrimSpace(item.Doc); doc != "" {
		out += "\n\nDescription:\n" + formatHoverMarkdown(doc)
	}
	return out
}

func importedPackageNames(src string) []string {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", src, parser.ImportsOnly)
	if err != nil || file == nil {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(file.Imports))
	for _, imp := range file.Imports {
		if imp == nil || imp.Path == nil {
			continue
		}
		name := ""
		if imp.Name != nil {
			name = strings.TrimSpace(imp.Name.Name)
			if name == "_" || name == "." {
				continue
			}
		}
		if name == "" {
			p := strings.Trim(imp.Path.Value, "\"")
			name = pathpkg.Base(p)
		}
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func isSimpleIdentRune(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_'
}

func applyCompletionText(app *appState, prefixStart int, insertText string) {
	insert := []rune(insertText)
	cur := app.ed.Runes()
	next := make([]rune, 0, len(cur)-(app.ed.Caret-prefixStart)+len(insert))
	next = append(next, cur[:prefixStart]...)
	next = append(next, insert...)
	next = append(next, cur[app.ed.Caret:]...)
	app.ed.SetRunes(next)
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
	for kw := range goCompletionKeywords {
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
