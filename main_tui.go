package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"gc/editor"

	"github.com/gdamore/tcell/v2"
)

type memoryClipboard struct {
	mu   sync.Mutex
	text string
}

func (m *memoryClipboard) GetText() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.text, nil
}

func (m *memoryClipboard) SetText(text string) error {
	m.mu.Lock()
	m.text = text
	m.mu.Unlock()
	return nil
}

func main() {
	if err := runTUI(); err != nil {
		panic(err)
	}
}

func runTUI() error {
	screen, err := tcell.NewScreen()
	if err != nil {
		return err
	}
	if err := screen.Init(); err != nil {
		return err
	}
	defer screen.Fini()

	root, _ := os.Getwd()
	clip := &memoryClipboard{}
	ed := editor.NewEditor("")
	ed.SetClipboard(clip)
	app := appState{
		blinkAt:      time.Now(),
		openRoot:     root,
		syntaxHL:     newGoHighlighter(),
		syntaxCheck:  newGoSyntaxChecker(),
		gopls:        newGoplsClient(),
		clipboard:    clip,
		startupFast:  true,
		escHelpDelay: 700 * time.Millisecond,
	}
	app.requestInterrupt = func(data any) {
		_ = screen.PostEvent(tcell.NewEventInterrupt(data))
	}
	app.initBuffers(ed)
	defer app.gopls.close()

	if len(os.Args) > 1 {
		loadStartupFiles(&app, filterArgsToFiles(os.Args[1:]))
	}

	for {
		fastStartupPass := app.startupFast
		drawTUI(screen, &app)
		if fastStartupPass {
			// Immediately draw again so startup can paint fast first, then enrich.
			continue
		}
		ev := screen.PollEvent()
		switch e := ev.(type) {
		case *tcell.EventResize:
			screen.Sync()
		case *tcell.EventKey:
			if !handleTUIKey(&app, e) {
				return nil
			}
		case *tcell.EventInterrupt:
			handleTUIInterrupt(&app, e)
		}
	}
}

func handleTUIInterrupt(app *appState, ev *tcell.EventInterrupt) {
	if app == nil || ev == nil {
		return
	}
	switch data := ev.Data().(type) {
	case int:
		if data != app.escHelpToken {
			return
		}
		delay := app.escHelpDelay
		if delay <= 0 {
			delay = 700 * time.Millisecond
		}
		if !app.cmdPrefixActive || app.escHelpVisible {
			return
		}
		// A newer prefix key may already have consumed Esc; only show help after the full delay.
		if time.Since(app.escPrefixAt) < delay {
			return
		}
		app.escHelpVisible = true
	case completionDetailInterrupt:
		if !app.completionPopup.active || data.Token != app.completionPopup.detailToken {
			return
		}
		delay := app.completionPopup.detailDelay
		if delay <= 0 {
			delay = 700 * time.Millisecond
		}
		if time.Since(app.completionPopup.detailArmedAt) < delay {
			return
		}
		if app.completionPopup.selected < 0 || app.completionPopup.selected >= len(app.completionPopup.items) {
			return
		}
		app.completionPopup.detailText = completionPopupDetailText(app.completionPopup.items[app.completionPopup.selected])
		app.completionPopup.detailVisible = strings.TrimSpace(app.completionPopup.detailText) != ""
	}
}

func handleTUIKey(app *appState, ev *tcell.EventKey) bool {
	if app == nil || ev == nil {
		return true
	}
	if app.escSeqActive {
		return handleEscSeqKey(app, ev)
	}
	mods := tcellToMods(ev.Modifiers())
	if app.cmdPrefixActive && ev.Key() == tcell.KeyRune && ev.Rune() == '[' {
		app.escSeqActive = true
		app.escSeq = "["
		return true
	}

	// Prefix mode: force next key through command dispatch (not text input).
	if app.cmdPrefixActive && ev.Key() == tcell.KeyRune {
		if k, ok := runeToKeyCode(ev.Rune()); ok {
			keyMods := mods
			if inferShiftFromRune(ev.Rune()) {
				keyMods |= modShift
			}
			keepRunning := dispatchTUIKeyEvent(app, keyEvent{down: true, repeat: 0, key: k, mods: keyMods})
			// TUI prefix dispatch does not have a separate text-input event for this key.
			app.suppressTextOnce = false
			return keepRunning
		}
		// Unknown key still consumes the prefix and does not insert text.
		keepRunning := dispatchTUIKeyEvent(app, keyEvent{down: true, repeat: 0, key: keyUnknown, mods: mods})
		app.suppressTextOnce = false
		return keepRunning
	}
	if app.lessMode && ev.Key() == tcell.KeyRune && ev.Rune() == ' ' {
		return dispatchTUIKeyEvent(app, keyEvent{down: true, repeat: 0, key: keySpace, mods: mods})
	}

	if ev.Key() == tcell.KeyRune && (ev.Modifiers()&tcell.ModCtrl) == 0 {
		return dispatchTUIText(app, string(ev.Rune()), mods)
	}
	if ev.Key() == tcell.KeyRune && (ev.Modifiers()&tcell.ModCtrl) != 0 {
		if k, ok := ctrlRuneToKey(ev.Rune()); ok {
			return dispatchTUIKeyEvent(app, keyEvent{down: true, repeat: 0, key: k, mods: mods | modCtrl})
		}
	}

	if k, ok := tcellKeyToKeyCode(ev); ok {
		keyMods := mods
		if ev.Key() >= tcell.KeyCtrlA && ev.Key() <= tcell.KeyCtrlZ {
			keyMods |= modCtrl
		}
		if ev.Key() == tcell.KeyBacktab {
			keyMods |= modShift
		}
		return dispatchTUIKeyEvent(app, keyEvent{down: true, repeat: 0, key: k, mods: keyMods})
	}
	return true
}

func handleEscSeqKey(app *appState, ev *tcell.EventKey) bool {
	if ev == nil || app == nil {
		return true
	}
	if ev.Key() != tcell.KeyRune {
		app.escSeqActive = false
		app.escSeq = ""
		app.cmdPrefixActive = false
		if k, ok := tcellKeyToKeyCode(ev); ok {
			return dispatchTUIKeyEvent(app, keyEvent{down: true, repeat: 0, key: k, mods: tcellToMods(ev.Modifiers())})
		}
		return true
	}
	app.escSeq += string(ev.Rune())
	k, mods, done, ok := decodeEscSeqArrow(app.escSeq)
	if !done {
		if len(app.escSeq) > 16 {
			app.escSeqActive = false
			app.escSeq = ""
			app.cmdPrefixActive = false
		}
		return true
	}
	app.escSeqActive = false
	app.escSeq = ""
	app.cmdPrefixActive = false
	if !ok {
		return true
	}
	return dispatchTUIKeyEvent(app, keyEvent{down: true, repeat: 0, key: k, mods: mods})
}

func decodeEscSeqArrow(seq string) (keyCode, modMask, bool, bool) {
	if !strings.HasPrefix(seq, "[") || len(seq) < 2 {
		return keyUnknown, 0, false, false
	}
	last := seq[len(seq)-1]
	if last != 'A' && last != 'B' && last != 'C' && last != 'D' {
		if isEscSeqPrefix(seq) {
			return keyUnknown, 0, false, false
		}
		return keyUnknown, 0, true, false
	}
	k := keyUnknown
	switch last {
	case 'A':
		k = keyUp
	case 'B':
		k = keyDown
	case 'C':
		k = keyRight
	case 'D':
		k = keyLeft
	}
	payload := seq[1 : len(seq)-1]
	if payload == "" {
		return k, 0, true, true
	}
	parts := strings.Split(payload, ";")
	if len(parts) == 1 {
		return k, 0, true, true
	}
	if len(parts) != 2 {
		return keyUnknown, 0, true, false
	}
	modParam, err := strconv.Atoi(parts[1])
	if err != nil {
		return keyUnknown, 0, true, false
	}
	return k, modMaskFromCSI(modParam), true, true
}

func modMaskFromCSI(modParam int) modMask {
	var mods modMask
	// XTerm modifier parameter uses 1 as base.
	// 2=Shift, 3=Alt, 4=Shift+Alt, 5=Ctrl, 6=Shift+Ctrl, 7=Alt+Ctrl, 8=Shift+Alt+Ctrl.
	switch modParam {
	case 2:
		mods |= modShift
	case 3:
		mods |= modLAlt
	case 4:
		mods |= modShift | modLAlt
	case 5:
		mods |= modCtrl
	case 6:
		mods |= modShift | modCtrl
	case 7:
		mods |= modLAlt | modCtrl
	case 8:
		mods |= modShift | modLAlt | modCtrl
	}
	return mods
}

func isEscSeqPrefix(seq string) bool {
	if !strings.HasPrefix(seq, "[") {
		return false
	}
	for _, r := range seq[1:] {
		if r == ';' {
			continue
		}
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func dispatchTUIKeyEvent(app *appState, e keyEvent) bool {
	if app.inputActive {
		return handleInputKey(app, e)
	}
	if app.open.Active {
		return handleOpenKeyEvent(app, e)
	}
	return handleKeyEvent(app, e)
}

func dispatchTUIText(app *appState, text string, mods modMask) bool {
	if app.inputActive {
		return handleInputText(app, text)
	}
	if app.open.Active {
		return handleOpenTextEvent(app, text)
	}
	return handleTextEvent(app, text, mods)
}

func tcellToMods(m tcell.ModMask) modMask {
	var out modMask
	if (m & tcell.ModShift) != 0 {
		out |= modShift
	}
	if (m & tcell.ModCtrl) != 0 {
		out |= modCtrl
	}
	if (m & tcell.ModAlt) != 0 {
		out |= modLAlt
	}
	return out
}

func tcellKeyToKeyCode(ev *tcell.EventKey) (keyCode, bool) {
	switch ev.Key() {
	case tcell.KeyUp:
		return keyUp, true
	case tcell.KeyDown:
		return keyDown, true
	case tcell.KeyPgUp:
		return keyPageUp, true
	case tcell.KeyPgDn:
		return keyPageDown, true
	case tcell.KeyHome:
		return keyHome, true
	case tcell.KeyEnd:
		return keyEnd, true
	case tcell.KeyEscape:
		return keyEscape, true
	case tcell.KeyTAB, tcell.KeyBacktab:
		return keyTab, true
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		return keyBackspace, true
	case tcell.KeyDelete:
		return keyDelete, true
	case tcell.KeyEnter:
		return keyReturn, true
	case tcell.KeyLeft:
		return keyLeft, true
	case tcell.KeyRight:
		return keyRight, true
	case tcell.KeyCtrlSpace:
		return keyLctrl, true
	case tcell.KeyCtrlA:
		return keyA, true
	case tcell.KeyCtrlB:
		return keyB, true
	case tcell.KeyCtrlC:
		return keyC, true
	case tcell.KeyCtrlE:
		return keyE, true
	case tcell.KeyCtrlF:
		return keyF, true
	case tcell.KeyCtrlI:
		// Many terminals encode Ctrl+Tab as Ctrl+I.
		return keyTab, true
	case tcell.KeyCtrlK:
		return keyK, true
	case tcell.KeyCtrlL:
		return keyL, true
	case tcell.KeyCtrlO:
		return keyO, true
	case tcell.KeyCtrlQ:
		return keyQ, true
	case tcell.KeyCtrlR:
		return keyR, true
	case tcell.KeyCtrlS:
		return keyS, true
	case tcell.KeyCtrlU:
		return keyU, true
	case tcell.KeyCtrlV:
		return keyV, true
	case tcell.KeyCtrlX:
		return keyX, true
	case tcell.KeyRune:
		switch strings.ToLower(string(ev.Rune())) {
		case "/":
			return keySlash, true
		case ",":
			return keyComma, true
		case ".":
			return keyPeriod, true
		}
	}
	return keyUnknown, false
}

func drawTUI(s tcell.Screen, app *appState) {
	if app == nil || app.ed == nil {
		s.Clear()
		s.Show()
		return
	}
	w, h := s.Size()
	if w < 10 || h < 4 {
		s.Clear()
		s.Show()
		return
	}

	lines, lineStyles, langMode, lineStarts := renderData(app)
	kind := syntaxNone
	switch langMode {
	case "go":
		kind = syntaxGo
	case "markdown":
		kind = syntaxMarkdown
	case "c":
		kind = syntaxC
	case "miranda":
		kind = syntaxMiranda
	}
	lineH := 1
	contentH := h - 2
	cLine := editor.CaretLineAt(lines, app.ed.Caret)
	cCol := editor.CaretColAt(lines, app.ed.Caret)
	ensureCaretVisible(app, cLine, len(lines), contentH)
	startLine := clamp(app.scrollLine, 0, max(0, len(lines)-contentH))
	caretY := cLine - startLine

	base := tcell.StyleDefault.Background(tcell.ColorBlack).Foreground(tcell.ColorWhite)
	gutter := tcell.StyleDefault.Background(tcell.ColorBlack).Foreground(tcell.ColorDarkCyan)
	gutterErr := tcell.StyleDefault.Background(tcell.ColorBlack).Foreground(tcell.ColorIndianRed)
	current := tcell.StyleDefault.Background(tcell.ColorBlack).Foreground(tcell.ColorWhite)
	if app.syntaxCheck == nil {
		app.syntaxCheck = newGoSyntaxChecker()
	}
	lineErrors, lineErrMsgs := activeBufferSyntaxErrors(app, kind, app.currentPath)
	var sel *selectionRange
	if app.ed.Sel.Active {
		selA, selB := app.ed.Sel.Normalised()
		sel = &selectionRange{a: selA, b: selB}
	}
	if lineStarts == nil {
		lineStarts = computeLineStarts(lines)
		if app.render.bufIdx == app.bufIdx && app.render.path == app.currentPath && len(app.render.lines) == len(lines) {
			app.render.lineStarts = lineStarts
		}
	}
	for row := 0; row < contentH; row += lineH {
		ln := startLine + row
		fillRow(s, row, w, base)
		if ln >= len(lines) {
			continue
		}
		lineStyle := base
		if ln == cLine {
			lineStyle = current
		}
		g := fmt.Sprintf("%4d ", ln+1)
		drawCellText(s, 0, row, g, gutter)
		if _, ok := lineErrors[ln]; ok {
			s.SetContent(0, row, '!', nil, gutterErr)
		}
		drawStyledTUICellLine(
			s, 5, row, lines[ln], lineStylesAt(lineStyles, ln), lineStyle,
			lineStarts[ln], sel,
		)
	}

	status := fmt.Sprintf("%s | lang=%s | root=%s", bufferLabel(app), langMode, app.openRoot)
	if len(app.buffers) > 0 && app.buffers[app.bufIdx].dirty {
		status += " | *unsaved*"
	}
	if app.lastEvent != "" {
		status += " | " + app.lastEvent
	}
	drawCellText(s, 0, h-2, padRight(status, w), tcell.StyleDefault.Background(tcell.ColorDarkSlateBlue).Foreground(tcell.ColorWhite))

	input := ""
	inputStyle := tcell.StyleDefault.Background(tcell.ColorBlack).Foreground(tcell.ColorGray)
	if app.inputActive {
		input = app.inputPrompt + app.inputValue
	} else if app.open.Active {
		input = "Open: " + app.open.Query
	} else if app.searchActive {
		input = "Search: " + string(app.searchQuery)
	} else if app.ed.Leap.Active {
		input = "Leap: " + string(app.ed.Leap.Query)
	} else if msg, ok := lineErrMsgs[cLine]; ok && strings.TrimSpace(msg) != "" {
		input = "Go syntax error: " + msg
		inputStyle = tcell.StyleDefault.Background(tcell.ColorBlack).Foreground(tcell.ColorIndianRed)
	} else {
		input = "Leap: unbound in TUI | Shift+Tab buffer cycle"
	}
	drawCellText(s, 0, h-1, padRight(input, w), inputStyle)

	if strings.TrimSpace(app.symbolInfoPopup) != "" {
		drawTUISymbolPopup(s, app, w, h)
	}
	if app.completionPopup.active {
		drawTUICompletionPopup(s, app, w, h)
		if app.completionPopup.detailVisible {
			drawTUICompletionDetailPopup(s, app, w, h)
		}
	}
	if app.escHelpVisible {
		drawTUIEscHelpPopup(s, w, h)
	}

	caretX := 5 + visualColForRuneCol(lines[cLine], cCol, tabWidth)
	if caretY >= 0 && caretY < contentH && caretX >= 0 && caretX < w {
		s.ShowCursor(caretX, caretY)
	} else {
		s.HideCursor()
	}
	s.Show()
}

type escShortcutCategory struct {
	title string
	items []string
}

var escHelpCategories = []escShortcutCategory{
	{
		title: "Files",
		items: []string{
			"b  new buffer",
			"w  write as...",
			"f  save + fmt/fix + reload",
			"S  save dirty buffers",
		},
	},
	{
		title: "Search & Modes",
		items: []string{
			"/  search mode",
			"x  line highlight mode",
			"m  cycle language mode",
			"i  symbol info popup",
		},
	},
	{
		title: "Navigation",
		items: []string{
			",  page up",
			".  page down",
			"Space  less mode",
			"Esc  close current buffer",
		},
	},
	{
		title: "Session",
		items: []string{
			"Q  quit all buffers",
			"Delete  clear buffer contents",
		},
	},
}

func drawTUIEscHelpPopup(s tcell.Screen, w, h int) {
	lines := escHelpPopupLines()
	if len(lines) == 0 || w < 20 || h < 8 {
		return
	}
	maxLine := 0
	for _, ln := range lines {
		if len(ln) > maxLine {
			maxLine = len(ln)
		}
	}
	boxW := min(max(34, maxLine+4), max(20, w-2))
	boxH := min(len(lines)+3, max(8, h-3))
	x0 := max(0, w-boxW-1)
	y0 := max(0, h-boxH-2)
	bg := tcell.StyleDefault.Background(tcell.ColorDarkSlateGray).Foreground(tcell.ColorWhite)
	border := tcell.StyleDefault.Background(tcell.ColorDarkSlateGray).Foreground(tcell.ColorLightCyan)
	title := tcell.StyleDefault.Background(tcell.ColorDarkSlateGray).Foreground(tcell.ColorLightYellow)
	dim := tcell.StyleDefault.Background(tcell.ColorDarkSlateGray).Foreground(tcell.ColorSilver)
	for y := range boxH {
		for x := range boxW {
			ch := ' '
			st := bg
			if y == 0 || y == boxH-1 || x == 0 || x == boxW-1 {
				ch = '│'
				if y == 0 || y == boxH-1 {
					ch = '─'
				}
				if y == 0 && x == 0 {
					ch = '┌'
				} else if y == 0 && x == boxW-1 {
					ch = '┐'
				} else if y == boxH-1 && x == 0 {
					ch = '└'
				} else if y == boxH-1 && x == boxW-1 {
					ch = '┘'
				}
				st = border
			}
			s.SetContent(x0+x, y0+y, ch, nil, st)
		}
	}
	drawCellText(s, x0+2, y0+1, padRight(lines[0], boxW-4), title)
	for i := 1; i < boxH-3 && i < len(lines); i++ {
		drawCellText(s, x0+2, y0+1+i, padRight(lines[i], boxW-4), bg)
	}
	drawCellText(s, x0+2, y0+boxH-2, padRight("Esc prefix active: press next key", boxW-4), dim)
}

func escHelpPopupLines() []string {
	out := []string{"Esc Commands"}
	for _, cat := range escHelpCategories {
		out = append(out, "")
		out = append(out, "["+cat.title+"]")
		for _, item := range cat.items {
			out = append(out, "  "+item)
		}
	}
	return out
}

func renderData(app *appState) ([]string, [][]tokenStyle, string, []int) {
	if app == nil || app.ed == nil {
		return []string{""}, nil, "text", nil
	}
	bufIdx := app.bufIdx
	textRev := 0
	if bufIdx >= 0 && bufIdx < len(app.buffers) {
		textRev = app.buffers[bufIdx].textRev
	}
	path := app.currentPath
	forcedMode := syntaxNone
	if bufIdx >= 0 && bufIdx < len(app.buffers) {
		forcedMode = app.buffers[bufIdx].mode
	}
	var slot *bufferSlot
	if bufIdx >= 0 && bufIdx < len(app.buffers) {
		slot = &app.buffers[bufIdx]
	}
	if app.render.bufIdx == bufIdx &&
		app.render.textRev == textRev &&
		app.render.mode == forcedMode &&
		app.render.path == path &&
		len(app.render.lines) > 0 {
		return app.render.lines, app.render.lineStyles, app.render.langMode, app.render.lineStarts
	}
	if slot != nil &&
		slot.cachedTextRev == textRev &&
		slot.cachedMode == forcedMode &&
		slot.cachedPath == path &&
		len(slot.cachedLines) > 0 {
		app.render = renderCache{
			bufIdx:     bufIdx,
			textRev:    textRev,
			mode:       forcedMode,
			path:       path,
			lines:      slot.cachedLines,
			lineStyles: slot.cachedLineStyles,
			langMode:   slot.cachedLangMode,
		}
		return app.render.lines, app.render.lineStyles, app.render.langMode, nil
	}

	lines := editor.SplitLines(app.ed.Runes())
	if len(lines) == 0 {
		lines = []string{""}
	}
	buf := app.ed.Runes()
	kind := bufferSyntaxKind(app, path, buf)
	if app.startupFast {
		app.startupFast = false
		langMode := syntaxKindLabel(kind)
		return lines, nil, langMode, nil
	}
	src := string(buf)
	lineStyles := app.syntaxHL.lineStyleForKind(path, src, lines, kind)
	langMode := syntaxKindLabel(kind)
	if slot != nil {
		slot.cachedTextRev = textRev
		slot.cachedMode = forcedMode
		slot.cachedPath = path
		slot.cachedLines = lines
		slot.cachedLineStyles = lineStyles
		slot.cachedLangMode = langMode
	}
	app.render = renderCache{
		bufIdx:     bufIdx,
		textRev:    textRev,
		mode:       forcedMode,
		path:       path,
		lines:      lines,
		lineStyles: lineStyles,
		langMode:   langMode,
	}
	return lines, lineStyles, langMode, nil
}

func drawCellText(s tcell.Screen, x, y int, text string, st tcell.Style) {
	for _, r := range text {
		w := runewidth(r)
		if w <= 0 {
			continue
		}
		s.SetContent(x, y, r, nil, st)
		x += w
	}
}

func runewidth(r rune) int {
	if r == 0 {
		return 0
	}
	return 1
}

func fillRow(s tcell.Screen, y, w int, st tcell.Style) {
	for x := range w {
		s.SetContent(x, y, ' ', nil, st)
	}
}

func lineStylesAt(all [][]tokenStyle, i int) []tokenStyle {
	if all == nil || i < 0 || i >= len(all) {
		return nil
	}
	return all[i]
}

func computeLineStarts(lines []string) []int {
	if len(lines) == 0 {
		return nil
	}
	out := make([]int, len(lines))
	pos := 0
	for i := range lines {
		out[i] = pos
		pos += utf8.RuneCountInString(lines[i]) + 1
	}
	return out
}

type selectionRange struct {
	a int
	b int
}

func drawStyledTUICellLine(
	s tcell.Screen,
	x, y int,
	line string,
	style []tokenStyle,
	base tcell.Style,
	lineStart int,
	sel *selectionRange,
) {
	visual := 0
	i := 0
	for _, r := range line {
		ts := styleDefault
		if i >= 0 && i < len(style) {
			ts = style[i]
		}
		st := tuiStyleForToken(base, ts)
		if sel != nil {
			abs := lineStart + i
			if abs >= sel.a && abs < sel.b {
				st = st.Background(tcell.ColorDarkSlateBlue).Foreground(tcell.ColorWhite)
			}
		}
		if r == '\t' {
			next := ((visual / tabWidth) + 1) * tabWidth
			for visual < next {
				s.SetContent(x+visual, y, ' ', nil, st)
				visual++
			}
			i++
			continue
		}
		s.SetContent(x+visual, y, r, nil, st)
		visual++
		i++
	}
}

func tuiStyleForToken(base tcell.Style, ts tokenStyle) tcell.Style {
	switch ts {
	case styleKeyword:
		return base.Foreground(tcell.ColorMediumPurple)
	case styleType:
		return base.Foreground(tcell.ColorLightSkyBlue)
	case styleFunction:
		return base.Foreground(tcell.ColorKhaki)
	case styleString:
		return base.Foreground(tcell.ColorLightGreen)
	case styleNumber:
		return base.Foreground(tcell.ColorLightSalmon)
	case styleComment:
		return base.Foreground(tcell.ColorDarkSeaGreen)
	case styleHeading:
		return base.Foreground(tcell.ColorWheat)
	case styleLink:
		return base.Foreground(tcell.ColorLightCyan)
	case stylePunctuation:
		return base.Foreground(tcell.ColorThistle)
	default:
		return base
	}
}

func ctrlRuneToKey(r rune) (keyCode, bool) {
	switch unicode.ToLower(r) {
	case 'q':
		return keyQ, true
	case 'e':
		return keyE, true
	case 'r':
		return keyR, true
	case 'a':
		return keyA, true
	case 's':
		return keyS, true
	case 'f':
		return keyF, true
	case 'o':
		return keyO, true
	case 'l':
		return keyL, true
	case 'k':
		return keyK, true
	case 'u':
		return keyU, true
	case 'c':
		return keyC, true
	case 'x':
		return keyX, true
	case 'v':
		return keyV, true
	case 'i':
		return keyTab, true
	case '/':
		return keySlash, true
	case ',':
		return keyComma, true
	case '.':
		return keyPeriod, true
	case '<':
		return keyComma, true
	case '>':
		return keyPeriod, true
	}
	return keyUnknown, false
}

func runeToKeyCode(r rune) (keyCode, bool) {
	switch unicode.ToLower(r) {
	case 'a':
		return keyA, true
	case 'b':
		return keyB, true
	case 'c':
		return keyC, true
	case 'd':
		return keyD, true
	case 'e':
		return keyE, true
	case 'f':
		return keyF, true
	case 'g':
		return keyG, true
	case 'h':
		return keyH, true
	case 'i':
		return keyI, true
	case 'j':
		return keyJ, true
	case 'k':
		return keyK, true
	case 'l':
		return keyL, true
	case 'm':
		return keyM, true
	case 'n':
		return keyN, true
	case 'o':
		return keyO, true
	case 'p':
		return keyP, true
	case 'q':
		return keyQ, true
	case 'r':
		return keyR, true
	case 's':
		return keyS, true
	case 't':
		return keyT, true
	case 'u':
		return keyU, true
	case 'v':
		return keyV, true
	case 'w':
		return keyW, true
	case 'x':
		return keyX, true
	case 'y':
		return keyY, true
	case 'z':
		return keyZ, true
	case '/':
		return keySlash, true
	case ',':
		return keyComma, true
	case '.':
		return keyPeriod, true
	case '<':
		return keyComma, true
	case '>':
		return keyPeriod, true
	case '-':
		return keyMinus, true
	case '=':
		return keyEquals, true
	case ' ':
		return keySpace, true
	}
	return keyUnknown, false
}

func inferShiftFromRune(r rune) bool {
	if unicode.IsUpper(r) {
		return true
	}
	switch r {
	case '<', '>', '?', '_', '+':
		return true
	default:
		return false
	}
}

func padRight(s string, w int) string {
	rs := []rune(s)
	if len(rs) >= w {
		return string(rs[:w])
	}
	return s + strings.Repeat(" ", w-len(rs))
}

func drawTUISymbolPopup(s tcell.Screen, app *appState, w, h int) {
	if app == nil || strings.TrimSpace(app.symbolInfoPopup) == "" {
		return
	}
	bg := tcell.StyleDefault.Background(tcell.ColorDarkSlateGray).Foreground(tcell.ColorWhite)
	border := tcell.StyleDefault.Background(tcell.ColorDarkSlateGray).Foreground(tcell.ColorLightCyan)
	title := tcell.StyleDefault.Background(tcell.ColorDarkSlateGray).Foreground(tcell.ColorLightYellow)
	dim := tcell.StyleDefault.Background(tcell.ColorDarkSlateGray).Foreground(tcell.ColorSilver)
	code := tcell.StyleDefault.
		Background(tcell.ColorDarkSlateGray).
		Foreground(tcell.ColorLightGreen).
		Attributes(tcell.AttrItalic)

	boxW := min(w-6, 88)
	if boxW < 32 {
		boxW = w - 2
	}
	boxH := max(min(h-4, 16), 6)
	x := max(1, (w-boxW)/2)
	y := max(1, (h-boxH)/2)

	for yy := range boxH {
		for xx := 0; xx < boxW; xx++ {
			ch := ' '
			st := bg
			if yy == 0 || yy == boxH-1 || xx == 0 || xx == boxW-1 {
				ch = '│'
				if yy == 0 || yy == boxH-1 {
					ch = '─'
				}
				if yy == 0 && xx == 0 {
					ch = '┌'
				} else if yy == 0 && xx == boxW-1 {
					ch = '┐'
				} else if yy == boxH-1 && xx == 0 {
					ch = '└'
				} else if yy == boxH-1 && xx == boxW-1 {
					ch = '┘'
				}
				st = border
			}
			s.SetContent(x+xx, y+yy, ch, nil, st)
		}
	}

	drawCellText(s, x+2, y+1, padRight("Symbol Info (Esc+i to toggle)", boxW-4), title)
	contentW := boxW - 4
	lines := wrapPopupText(app.symbolInfoPopup, max(10, contentW))
	maxLines := boxH - 4
	start := clamp(app.symbolInfoScroll, 0, max(0, len(lines)-1))
	visible := popupVisibleLines(lines, start, maxLines)
	for i := range visible {
		st := symbolPopupLineStyle(visible[i], bg, code)
		drawCellText(s, x+2, y+2+i, padRight(visible[i], contentW), st)
	}
	drawCellText(s, x+2, y+boxH-2, padRight("Esc close", contentW), dim)
}

func symbolPopupLineStyle(line string, base, code tcell.Style) tcell.Style {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return base
	}
	if strings.HasPrefix(line, "    ") || strings.HasPrefix(trimmed, "Code:") || strings.HasPrefix(trimmed, "Code (") {
		return code
	}
	return base
}

func popupVisibleLines(lines []string, start, maxLines int) []string {
	if len(lines) == 0 || maxLines <= 0 {
		return nil
	}
	start = clamp(start, 0, max(0, len(lines)-1))
	end := min(len(lines), start+maxLines)
	return lines[start:end]
}

func drawTUICompletionPopup(s tcell.Screen, app *appState, w, h int) {
	if app == nil || !app.completionPopup.active || len(app.completionPopup.items) == 0 {
		return
	}
	bg := tcell.StyleDefault.Background(tcell.ColorDarkSlateGray).Foreground(tcell.ColorWhite)
	border := tcell.StyleDefault.Background(tcell.ColorDarkSlateGray).Foreground(tcell.ColorLightCyan)
	title := tcell.StyleDefault.Background(tcell.ColorDarkSlateGray).Foreground(tcell.ColorLightYellow)
	sel := tcell.StyleDefault.Background(tcell.ColorMidnightBlue).Foreground(tcell.ColorWhite)
	dim := tcell.StyleDefault.Background(tcell.ColorDarkSlateGray).Foreground(tcell.ColorSilver)

	boxW := min(w-6, 96)
	if boxW < 44 {
		boxW = w - 2
	}
	maxRows := min(len(app.completionPopup.items), 10)
	boxH := max(6, maxRows+4)
	boxH = min(boxH, h-2)
	x := max(1, w-boxW-1)
	y := max(1, h-boxH-3)

	for yy := range boxH {
		for xx := 0; xx < boxW; xx++ {
			ch := ' '
			st := bg
			if yy == 0 || yy == boxH-1 || xx == 0 || xx == boxW-1 {
				ch = '│'
				if yy == 0 || yy == boxH-1 {
					ch = '─'
				}
				if yy == 0 && xx == 0 {
					ch = '┌'
				} else if yy == 0 && xx == boxW-1 {
					ch = '┐'
				} else if yy == boxH-1 && xx == 0 {
					ch = '└'
				} else if yy == boxH-1 && xx == boxW-1 {
					ch = '┘'
				}
				st = border
			}
			s.SetContent(x+xx, y+yy, ch, nil, st)
		}
	}
	header := app.completionPopup.title
	if strings.TrimSpace(header) == "" {
		header = "Completion"
	}
	drawCellText(s, x+2, y+1, padRight(header, boxW-4), title)

	rows := boxH - 3
	start := 0
	if app.completionPopup.selected >= rows {
		start = app.completionPopup.selected - rows + 1
	}
	for row := range rows {
		idx := start + row
		if idx >= len(app.completionPopup.items) {
			break
		}
		item := app.completionPopup.items[idx]
		line := completionPopupLine(item)
		st := bg
		if idx == app.completionPopup.selected {
			st = sel
		}
		drawCellText(s, x+2, y+2+row, padRight(line, boxW-4), st)
	}
	drawCellText(s, x+2, y+boxH-2, padRight("Tab/Shift+Tab choose, Enter apply, Esc cancel", boxW-4), dim)
}

func completionPopupLine(item completionItem) string {
	label := strings.TrimSpace(item.Label)
	if label == "" {
		label = strings.TrimSpace(item.Insert)
	}
	detail := strings.TrimSpace(item.Detail)
	detail = strings.ReplaceAll(detail, "\n", " ")
	if detail == "" {
		return label
	}
	return label + "  —  " + detail
}

func drawTUICompletionDetailPopup(s tcell.Screen, app *appState, w, h int) {
	if app == nil || !app.completionPopup.active || !app.completionPopup.detailVisible {
		return
	}
	text := strings.TrimSpace(app.completionPopup.detailText)
	if text == "" {
		return
	}
	bg := tcell.StyleDefault.Background(tcell.ColorBlack).Foreground(tcell.ColorWhite)
	border := tcell.StyleDefault.Background(tcell.ColorBlack).Foreground(tcell.ColorDarkCyan)
	title := tcell.StyleDefault.Background(tcell.ColorBlack).Foreground(tcell.ColorLightYellow)

	boxW := min(w-8, 88)
	if boxW < 36 {
		boxW = w - 2
	}
	boxH := min(h-4, 14)
	if boxH < 6 {
		boxH = h - 2
	}
	x := max(1, w-boxW-1)
	y := 1

	for yy := range boxH {
		for xx := 0; xx < boxW; xx++ {
			ch := ' '
			st := bg
			if yy == 0 || yy == boxH-1 || xx == 0 || xx == boxW-1 {
				ch = '│'
				if yy == 0 || yy == boxH-1 {
					ch = '─'
				}
				if yy == 0 && xx == 0 {
					ch = '┌'
				} else if yy == 0 && xx == boxW-1 {
					ch = '┐'
				} else if yy == boxH-1 && xx == 0 {
					ch = '└'
				} else if yy == boxH-1 && xx == boxW-1 {
					ch = '┘'
				}
				st = border
			}
			s.SetContent(x+xx, y+yy, ch, nil, st)
		}
	}
	drawCellText(s, x+2, y+1, padRight("Completion Details", boxW-4), title)
	contentW := boxW - 4
	lines := wrapPopupText(text, max(12, contentW))
	maxLines := boxH - 3
	for i := 0; i < maxLines && i < len(lines); i++ {
		st := symbolPopupLineStyle(lines[i], bg, tcell.StyleDefault.Background(tcell.ColorBlack).Foreground(tcell.ColorLightGreen).Attributes(tcell.AttrItalic))
		drawCellText(s, x+2, y+2+i, padRight(lines[i], contentW), st)
	}
}
