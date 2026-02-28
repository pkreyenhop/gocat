package main

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
	"unicode"

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
		blinkAt:     time.Now(),
		openRoot:    root,
		syntaxHL:    newGoHighlighter(),
		syntaxCheck: newGoSyntaxChecker(),
		gopls:       newGoplsClient(),
		clipboard:   clip,
		startupFast: true,
	}
	app.initBuffers(ed)
	defer app.gopls.close()

	if len(os.Args) > 1 {
		loadStartupFiles(&app, filterArgsToFiles(os.Args[1:]))
	}
	go func() {
		// Force a second frame quickly so startup can paint fast first, then enrich.
		screen.PostEventWait(tcell.NewEventInterrupt(nil))
	}()

	for {
		drawTUI(screen, &app)
		ev := screen.PollEvent()
		switch e := ev.(type) {
		case *tcell.EventResize:
			screen.Sync()
		case *tcell.EventKey:
			if !handleTUIKey(&app, e) {
				return nil
			}
		case *tcell.EventInterrupt:
			// Wake-up redraw.
		}
	}
}

func handleTUIKey(app *appState, ev *tcell.EventKey) bool {
	if app == nil || ev == nil {
		return true
	}
	mods := tcellToMods(ev.Modifiers())

	// Primary leap activation in the TUI: Alt+F / Alt+B.
	if ev.Key() == tcell.KeyRune && (ev.Modifiers()&tcell.ModAlt) != 0 {
		switch strings.ToLower(string(ev.Rune())) {
		case "f":
			app.ed.LeapStart(editor.DirFwd)
			return true
		case "b":
			app.ed.LeapStart(editor.DirBack)
			return true
		}
	}

	// Prefix mode: force next key through command dispatch (not text input).
	if app.cmdPrefixActive && ev.Key() == tcell.KeyRune {
		if k, ok := runeToKeyCode(ev.Rune()); ok {
			keyMods := mods
			if inferShiftFromRune(ev.Rune()) {
				keyMods |= modShift
			}
			return dispatchTUIKeyEvent(app, keyEvent{down: true, repeat: 0, key: k, mods: keyMods})
		}
		// Unknown key still consumes the prefix and does not insert text.
		return dispatchTUIKeyEvent(app, keyEvent{down: true, repeat: 0, key: keyUnknown, mods: mods})
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
	case tcell.KeyCtrlW:
		return keyW, true
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

	lines, lineStyles, langMode := renderData(app)
	lineH := 1
	contentH := h - 2
	cLine := editor.CaretLineAt(lines, app.ed.Caret)
	cCol := editor.CaretColAt(lines, app.ed.Caret)
	ensureCaretVisible(app, cLine, len(lines), contentH)
	startLine := clamp(app.scrollLine, 0, max(0, len(lines)-contentH))
	caretY := cLine - startLine

	base := tcell.StyleDefault.Background(tcell.ColorBlack).Foreground(tcell.ColorWhite)
	gutter := tcell.StyleDefault.Background(tcell.ColorBlack).Foreground(tcell.ColorDarkCyan)
	current := tcell.StyleDefault.Background(tcell.ColorBlack).Foreground(tcell.ColorWhite)

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
		drawStyledTUICellLine(s, 5, row, lines[ln], lineStylesAt(lineStyles, ln), lineStyle)
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
	if app.inputActive {
		input = app.inputPrompt + app.inputValue
	} else if app.open.Active {
		input = "Open: " + app.open.Query
	} else if app.ed.Leap.Active {
		input = "Leap: " + string(app.ed.Leap.Query)
	} else {
		input = "Alt+f / Alt+b leap | Shift+Tab buffer cycle"
	}
	drawCellText(s, 0, h-1, padRight(input, w), tcell.StyleDefault.Background(tcell.ColorBlack).Foreground(tcell.ColorGray))

	if strings.TrimSpace(app.symbolInfoPopup) != "" {
		drawTUISymbolPopup(s, app, w, h)
	}

	caretX := 5 + visualColForRuneCol(lines[cLine], cCol, tabWidth)
	if caretY >= 0 && caretY < contentH && caretX >= 0 && caretX < w {
		s.ShowCursor(caretX, caretY)
	} else {
		s.HideCursor()
	}
	s.Show()
}

func renderData(app *appState) ([]string, map[int][]tokenStyle, string) {
	if app == nil || app.ed == nil {
		return []string{""}, nil, "text"
	}
	bufIdx := app.bufIdx
	rev := 0
	if bufIdx >= 0 && bufIdx < len(app.buffers) {
		rev = app.buffers[bufIdx].rev
	}
	path := app.currentPath
	if app.render.bufIdx == bufIdx && app.render.rev == rev && app.render.path == path && len(app.render.lines) > 0 {
		return app.render.lines, app.render.lineStyles, app.render.langMode
	}

	lines := editor.SplitLines(app.ed.Buf)
	if len(lines) == 0 {
		lines = []string{""}
	}
	kind := bufferSyntaxKind(app, path, app.ed.Buf)
	if app.startupFast {
		app.startupFast = false
		langMode := syntaxKindLabel(kind)
		return lines, nil, langMode
	}
	src := string(app.ed.Buf)
	lineStyles := app.syntaxHL.lineStyleForKind(path, src, lines, kind)
	langMode := syntaxKindLabel(kind)
	app.render = renderCache{
		bufIdx:     bufIdx,
		rev:        rev,
		path:       path,
		lines:      lines,
		lineStyles: lineStyles,
		langMode:   langMode,
	}
	return lines, lineStyles, langMode
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

func lineStylesAt(all map[int][]tokenStyle, i int) []tokenStyle {
	if all == nil {
		return nil
	}
	return all[i]
}

func drawStyledTUICellLine(s tcell.Screen, x, y int, line string, style []tokenStyle, base tcell.Style) {
	runes := []rune(line)
	visual := 0
	for i, r := range runes {
		st := tuiStyleForToken(base, styleAt(style, i))
		if r == '\t' {
			next := ((visual / tabWidth) + 1) * tabWidth
			for visual < next {
				s.SetContent(x+visual, y, ' ', nil, st)
				visual++
			}
			continue
		}
		s.SetContent(x+visual, y, r, nil, st)
		visual++
	}
}

func styleAt(s []tokenStyle, i int) tokenStyle {
	if i < 0 || i >= len(s) {
		return styleDefault
	}
	return s[i]
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
	switch strings.ToLower(string(r)) {
	case "q":
		return keyQ, true
	case "w":
		return keyW, true
	case "e":
		return keyE, true
	case "r":
		return keyR, true
	case "a":
		return keyA, true
	case "s":
		return keyS, true
	case "f":
		return keyF, true
	case "o":
		return keyO, true
	case "l":
		return keyL, true
	case "k":
		return keyK, true
	case "u":
		return keyU, true
	case "c":
		return keyC, true
	case "x":
		return keyX, true
	case "v":
		return keyV, true
	case "i":
		return keyTab, true
	case "/":
		return keySlash, true
	case ",":
		return keyComma, true
	case ".":
		return keyPeriod, true
	case "<":
		return keyComma, true
	case ">":
		return keyPeriod, true
	}
	return keyUnknown, false
}

func runeToKeyCode(r rune) (keyCode, bool) {
	switch strings.ToLower(string(r)) {
	case "a":
		return keyA, true
	case "b":
		return keyB, true
	case "c":
		return keyC, true
	case "d":
		return keyD, true
	case "e":
		return keyE, true
	case "f":
		return keyF, true
	case "g":
		return keyG, true
	case "h":
		return keyH, true
	case "i":
		return keyI, true
	case "j":
		return keyJ, true
	case "k":
		return keyK, true
	case "l":
		return keyL, true
	case "m":
		return keyM, true
	case "n":
		return keyN, true
	case "o":
		return keyO, true
	case "p":
		return keyP, true
	case "q":
		return keyQ, true
	case "r":
		return keyR, true
	case "s":
		return keyS, true
	case "t":
		return keyT, true
	case "u":
		return keyU, true
	case "v":
		return keyV, true
	case "w":
		return keyW, true
	case "x":
		return keyX, true
	case "y":
		return keyY, true
	case "z":
		return keyZ, true
	case "/":
		return keySlash, true
	case ",":
		return keyComma, true
	case ".":
		return keyPeriod, true
	case "<":
		return keyComma, true
	case ">":
		return keyPeriod, true
	case "-":
		return keyMinus, true
	case "=":
		return keyEquals, true
	case " ":
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
		drawCellText(s, x+2, y+2+i, padRight(visible[i], contentW), bg)
	}
	drawCellText(s, x+2, y+boxH-2, padRight("Esc close", contentW), dim)
}

func popupVisibleLines(lines []string, start, maxLines int) []string {
	if len(lines) == 0 || maxLines <= 0 {
		return nil
	}
	start = clamp(start, 0, max(0, len(lines)-1))
	end := min(len(lines), start+maxLines)
	return lines[start:end]
}
