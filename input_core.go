package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"gc/editor"
)

type modMask uint16

const (
	modShift modMask = 1 << iota
	modCtrl
	modLCmd
	modRCmd
	modLAlt
	modRAlt
)

type keyCode int

const (
	keyUnknown keyCode = iota
	keyUp
	keyDown
	keyPageUp
	keyPageDown
	keyHome
	keyEnd
	keyEscape
	keyTab
	keyBackspace
	keyDelete
	keyReturn
	keyKpEnter
	keyLeft
	keyRight
	keyLcmd
	keyRcmd
	keySpace
	keyPeriod
	keyComma
	keyMinus
	keyEquals
	keySlash
	keyQ
	keyB
	keyW
	keyF
	keyS
	keyR
	keyA
	keyE
	keyK
	keyU
	keyI
	keyO
	keyL
	keyC
	keyX
	keyV
	key0
	key1
	key2
	key3
	key4
	key5
	key6
	key7
	key8
	key9
	keyD
	keyG
	keyH
	keyJ
	keyM
	keyN
	keyP
	keyT
	keyY
	keyZ
)

type keyEvent struct {
	down   bool
	repeat int
	key    keyCode
	mods   modMask
}

func handleKeyEvent(app *appState, e keyEvent) bool {
	ed := app.ed
	app.blinkAt = time.Now()
	app.lastMods = e.mods

	if e.down {
		app.lastEvent = fmt.Sprintf("KEYDOWN key=%s repeat=%d mods=%s", keyName(e.key), e.repeat, modsString(e.mods))
	} else {
		app.lastEvent = fmt.Sprintf("KEYUP   key=%s mods=%s", keyName(e.key), modsString(e.mods))
	}
	if Debug {
		fmt.Println(app.lastEvent)
	}

	if e.down && app.symbolInfoPopup != "" {
		switch e.key {
		case keyUp:
			app.symbolInfoScroll = max(0, app.symbolInfoScroll-1)
			return true
		case keyDown:
			app.symbolInfoScroll++
			return true
		case keyPageUp:
			app.symbolInfoScroll = max(0, app.symbolInfoScroll-6)
			return true
		case keyPageDown:
			app.symbolInfoScroll += 6
			return true
		case keyHome:
			app.symbolInfoScroll = 0
			return true
		case keyEnd:
			app.symbolInfoScroll = 1 << 20
			return true
		}
	}

	if e.down && e.repeat == 0 && e.key == keyEscape && !ed.Leap.Active {
		if app.symbolInfoPopup != "" {
			app.symbolInfoPopup = ""
			app.symbolInfoScroll = 0
			return true
		}
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
		if len(app.buffers) > 0 && !app.buffers[app.bufIdx].dirty {
			remaining := app.closeBuffer()
			app.lastEvent = "Closed clean buffer"
			if remaining == 0 {
				return false
			}
			return true
		}
		app.lastEvent = "Unsaved changes — press Ctrl+W to save or Ctrl+Q to close"
		return true
	}

	if e.down && e.repeat == 0 {
		ctrlHeld := (e.mods & modCtrl) != 0
		if ctrlHeld {
			switch e.key {
			case keyTab:
				delta := 1
				if (e.mods & modShift) != 0 {
					delta = -1
				}
				app.switchBuffer(delta)
				app.lastEvent = fmt.Sprintf("Switched to buffer %d/%d", app.bufIdx+1, len(app.buffers))
				return true
			case keyQ:
				if (e.mods & modShift) != 0 {
					app.lastEvent = "Quit (discard all buffers)"
					return false
				}
				remaining := app.closeBuffer()
				if remaining == 0 {
					app.lastEvent = "Closed last buffer, quitting"
					return false
				}
				app.lastEvent = fmt.Sprintf("Closed buffer, now %d/%d", app.bufIdx+1, remaining)
				return true
			case keyB:
				app.addBuffer()
				app.lastEvent = fmt.Sprintf("New buffer %d/%d", app.bufIdx+1, len(app.buffers))
				return true
			case keyW:
				if err := saveCurrent(app); err != nil {
					app.lastEvent = fmt.Sprintf("SAVE ERR: %v", err)
				} else {
					app.lastEvent = fmt.Sprintf("Saved %s", app.currentPath)
				}
				return true
			case keyF:
				if err := formatFixReloadCurrent(app); err != nil {
					app.lastEvent = fmt.Sprintf("FMT/FIX ERR: %v", err)
				} else {
					app.lastEvent = fmt.Sprintf("Saved, fmt/fix, reloaded %s", app.currentPath)
				}
				return true
			case keyS:
				if (e.mods & modShift) != 0 {
					if err := saveAll(app); err != nil {
						app.lastEvent = fmt.Sprintf("SAVE ALL ERR: %v", err)
					} else {
						app.lastEvent = "Saved dirty buffers"
					}
					return true
				}
				if err := saveCurrent(app); err != nil {
					app.lastEvent = fmt.Sprintf("SAVE ERR: %v", err)
				} else {
					app.lastEvent = fmt.Sprintf("Saved %s", app.currentPath)
				}
				return true
			case keyR:
				if err := runCurrentPackage(app); err != nil {
					app.lastEvent = fmt.Sprintf("RUN ERR: %v", err)
				} else {
					app.lastEvent = "Running: go run ."
				}
				return true
			case keyA:
				lines := editor.SplitLines(ed.Buf)
				if (e.mods & modShift) != 0 {
					ed.CaretToBufferEdge(lines, false, true)
				} else {
					ed.CaretToLineEdge(lines, false, false)
				}
				return true
			case keyE:
				lines := editor.SplitLines(ed.Buf)
				if (e.mods & modShift) != 0 {
					ed.CaretToBufferEdge(lines, true, true)
				} else {
					ed.CaretToLineEdge(lines, true, false)
				}
				return true
			case keyK:
				ed.KillToLineEnd(editor.SplitLines(ed.Buf))
				app.markDirty()
				return true
			case keyU:
				ed.Undo()
				app.lastEvent = "Undo"
				app.markDirty()
				return true
			case keyI:
				app.symbolInfoPopup = showSymbolInfo(app)
				app.symbolInfoScroll = 0
				return true
			case keySlash:
				if (e.mods & modShift) != 0 {
					app.addBuffer()
					app.ed.Buf = []rune(helpText())
					app.currentPath = ""
					app.buffers[app.bufIdx].path = ""
					app.lastEvent = "Opened shortcuts buffer"
					return true
				}
				toggleComment(ed)
				app.lastEvent = "Toggled comment"
				app.markDirty()
				return true
			case keyO:
				listRoot := app.openRoot
				if listRoot == "" {
					if cwd, err := os.Getwd(); err == nil {
						listRoot = cwd
					}
				}
				if len(app.buffers) > 0 && app.buffers[app.bufIdx].picker {
					listRoot = filepath.Dir(listRoot)
				}
				list, err := pickerLines(listRoot, 500)
				if err != nil {
					app.lastEvent = fmt.Sprintf("OPEN ERR: %v", err)
					return true
				}
				if len(list) == 0 {
					app.lastEvent = "OPEN: no files under root"
					return true
				}
				app.openRoot = listRoot
				if len(app.buffers) > 0 && app.buffers[app.bufIdx].picker {
					app.buffers[app.bufIdx].pickerRoot = listRoot
					app.buffers[app.bufIdx].ed.Buf = []rune(strings.Join(list, "\n"))
					app.ed = app.buffers[app.bufIdx].ed
					app.currentPath = ""
				} else {
					app.addPickerBuffer(list)
				}
				app.lastEvent = fmt.Sprintf("OPEN: file picker (%d files). Leap to a line, Ctrl+L to load", len(list))
				return true
			case keyL:
				if err := loadFileAtCaret(app); err != nil {
					app.lastEvent = fmt.Sprintf("LOAD ERR: %v", err)
				} else {
					app.lastEvent = fmt.Sprintf("Opened %s", app.currentPath)
				}
				return true
			case keyComma:
				lines := editor.SplitLines(ed.Buf)
				ed.MoveCaretPage(lines, 20, editor.DirBack, (e.mods&modShift) != 0)
				return true
			case keyPeriod:
				lines := editor.SplitLines(ed.Buf)
				ed.MoveCaretPage(lines, 20, editor.DirFwd, (e.mods&modShift) != 0)
				return true
			case keyC:
				ed.CopySelection()
				return true
			case keyX:
				ed.CutSelection()
				app.markDirty()
				return true
			case keyV:
				ed.PasteClipboard()
				app.markDirty()
				return true
			}
		}
		if e.key == keyTab && !ed.Leap.Active {
			if tryManualCompletion(app) {
				app.lastEvent = "Completed"
			}
			return true
		}
	}

	if e.down && e.repeat == 0 && !ed.Leap.Active {
		ctrlHeld := (e.mods & modCtrl) != 0
		if ctrlHeld {
			if e.key == keyRcmd {
				ed.LeapAgain(editor.DirFwd)
				return true
			}
			if e.key == keyLcmd {
				ed.LeapAgain(editor.DirBack)
				return true
			}
		}
	}

	if e.down && e.repeat == 0 {
		if e.key == keyLcmd {
			ed.Leap.HeldL = true
		}
		if e.key == keyRcmd {
			ed.Leap.HeldR = true
		}
	}
	if !e.down {
		if e.key == keyLcmd {
			ed.Leap.HeldL = false
		}
		if e.key == keyRcmd {
			ed.Leap.HeldR = false
		}
	}

	if e.down && e.repeat == 0 && !ed.Leap.Active {
		ctrlHeld := (e.mods & modCtrl) != 0
		if !ctrlHeld {
			if e.key == keyRcmd {
				ed.LeapStart(editor.DirFwd)
				beginLeapGrab(app)
				return true
			}
			if e.key == keyLcmd {
				ed.LeapStart(editor.DirBack)
				beginLeapGrab(app)
				return true
			}
		}
	}

	if ed.Leap.Active && e.down && e.repeat == 0 {
		if (e.key == keyLcmd && ed.Leap.HeldR) || (e.key == keyRcmd && ed.Leap.HeldL) {
			ed.BeginLeapSelection()
		}
	}

	if !e.down && ed.Leap.Active {
		if !ed.Leap.HeldL && !ed.Leap.HeldR {
			ed.LeapEndCommit()
			endLeapGrab(app)
			return true
		}
	}

	if ed.Leap.Active && e.down && e.repeat == 0 {
		switch e.key {
		case keyEscape:
			ed.LeapCancel()
			endLeapGrab(app)
			return true
		case keyBackspace:
			ed.LeapBackspace()
			return true
		case keyReturn, keyKpEnter:
			ed.LeapEndCommit()
			endLeapGrab(app)
			return true
		}

		if r, ok := keyToRune(e.key, e.mods); ok {
			ed.Leap.LastSrc = "keydown"
			ed.LeapAppend(string(r))
			return true
		}
	}

	if !ed.Leap.Active && e.down {
		lines := editor.SplitLines(ed.Buf)
		switch e.key {
		case keyBackspace:
			ed.BackspaceOrDeleteSelection(true)
			app.markDirty()
		case keyDelete:
			if (e.mods & modShift) != 0 {
				if ed.DeleteLineAtCaret() {
					app.markDirty()
				}
			} else if ed.DeleteWordAtCaret() {
				app.markDirty()
			}
		case keyLeft:
			ed.MoveCaret(-1, (e.mods&modShift) != 0)
		case keyRight:
			ed.MoveCaret(1, (e.mods&modShift) != 0)
		case keyUp:
			ed.MoveCaretLine(lines, -1, (e.mods&modShift) != 0)
		case keyDown:
			ed.MoveCaretLine(lines, 1, (e.mods&modShift) != 0)
		case keyPageDown:
			ed.MoveCaretPage(lines, 20, editor.DirFwd, (e.mods&modShift) != 0)
		case keyPageUp:
			ed.MoveCaretPage(lines, 20, editor.DirBack, (e.mods&modShift) != 0)
		case keyReturn, keyKpEnter:
			if e.repeat == 0 {
				ed.InsertText("\n")
				app.markDirty()
			}
		}
	}
	return true
}

func handleTextEvent(app *appState, text string, mods modMask) bool {
	app.blinkAt = time.Now()
	app.lastEvent = fmt.Sprintf("TEXTINPUT %q mods=%s", text, modsString(mods))
	if Debug {
		fmt.Println(app.lastEvent)
	}

	if text == "" || !utf8.ValidString(text) {
		return true
	}
	if text == "\t" {
		return true
	}
	ed := app.ed
	if ed.Leap.Active {
		ed.Leap.LastSrc = "textinput"
		ed.LeapAppend(text)
		return true
	}
	if text == " " {
		lines := editor.SplitLines(ed.Buf)
		lineIdx := editor.CaretLineAt(lines, ed.Caret)
		col := editor.CaretColAt(lines, ed.Caret)
		double := app.lastSpaceLn == lineIdx && time.Since(app.lastSpaceAt) < 2*time.Second
		app.lastSpaceLn = lineIdx
		app.lastSpaceAt = time.Now()
		if double && ed.Caret > 0 && ed.Buf[ed.Caret-1] == ' ' {
			ed.BackspaceOrDeleteSelection(true)
			lines = editor.SplitLines(ed.Buf)
			col = editor.CaretColAt(lines, ed.Caret)
			lineStart := max(ed.Caret-col, 0)
			indentEnd := lineStart
			for indentEnd < len(ed.Buf) && (ed.Buf[indentEnd] == '\t' || ed.Buf[indentEnd] == ' ') {
				indentEnd++
			}
			ed.Caret = indentEnd
			ed.InsertText("\t")
			app.lastSpaceLn = lineIdx
			return true
		}
	} else {
		app.lastSpaceLn = -1
	}
	ed.InsertText(text)
	app.markDirty()
	return true
}

func handleOpenKeyEvent(app *appState, e keyEvent) bool {
	if !e.down || e.repeat != 0 {
		return true
	}
	switch e.key {
	case keyEscape:
		app.open.Active = false
		app.lastEvent = "Open cancelled"
		return true
	case keyBackspace:
		if len(app.open.Query) > 0 {
			rs := []rune(app.open.Query)
			app.open.Query = string(rs[:len(rs)-1])
			app.open.Matches = findMatches(app.openRoot, app.open.Query, 50)
		}
		return true
	case keyReturn, keyKpEnter:
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
		if r, ok := keyToRune(e.key, e.mods); ok {
			app.open.Query += string(r)
			app.open.Matches = findMatches(app.openRoot, app.open.Query, 50)
		}
		return true
	}
}

func handleOpenTextEvent(app *appState, text string) bool {
	if text != "" && utf8.ValidString(text) {
		app.open.Query += text
		app.open.Matches = findMatches(app.openRoot, app.open.Query, 50)
	}
	return true
}

func handleInputKey(app *appState, e keyEvent) bool {
	if !e.down || e.repeat != 0 {
		return true
	}
	switch e.key {
	case keyEscape:
		app.inputActive = false
		app.inputValue = ""
		app.inputPrompt = ""
		app.inputKind = ""
		app.lastEvent = "Input cancelled"
		return true
	case keyBackspace:
		if len(app.inputValue) > 0 {
			rs := []rune(app.inputValue)
			app.inputValue = string(rs[:len(rs)-1])
		}
		return true
	case keyReturn, keyKpEnter:
		switch app.inputKind {
		case "save":
			name := strings.TrimSpace(app.inputValue)
			if name == "" {
				app.lastEvent = "SAVE ERR: filename required"
				return true
			}
			path := name
			if !filepath.IsAbs(path) {
				root := app.openRoot
				if root == "" {
					if cwd, err := os.Getwd(); err == nil {
						root = cwd
					}
				}
				path = filepath.Join(root, name)
			}
			app.currentPath = path
			if app.bufIdx >= 0 && app.bufIdx < len(app.buffers) {
				app.buffers[app.bufIdx].path = path
			}
			app.inputActive = false
			app.inputValue = ""
			app.inputPrompt = ""
			app.inputKind = ""
			if err := saveCurrent(app); err != nil {
				app.lastEvent = fmt.Sprintf("SAVE ERR: %v", err)
			} else {
				app.lastEvent = fmt.Sprintf("Saved %s", app.currentPath)
			}
		default:
			app.inputActive = false
		}
		return true
	}
	return true
}

func handleInputText(app *appState, text string) bool {
	if text != "" && utf8.ValidString(text) {
		app.inputValue += text
	}
	return true
}

func keyToRune(k keyCode, mods modMask) (rune, bool) {
	shift := (mods & modShift) != 0
	switch k {
	case keyA:
		if shift {
			return 'A', true
		}
		return 'a', true
	case keyB:
		if shift {
			return 'B', true
		}
		return 'b', true
	case keyC:
		if shift {
			return 'C', true
		}
		return 'c', true
	case keyD:
		if shift {
			return 'D', true
		}
		return 'd', true
	case keyE:
		if shift {
			return 'E', true
		}
		return 'e', true
	case keyF:
		if shift {
			return 'F', true
		}
		return 'f', true
	case keyG:
		if shift {
			return 'G', true
		}
		return 'g', true
	case keyH:
		if shift {
			return 'H', true
		}
		return 'h', true
	case keyI:
		if shift {
			return 'I', true
		}
		return 'i', true
	case keyJ:
		if shift {
			return 'J', true
		}
		return 'j', true
	case keyK:
		if shift {
			return 'K', true
		}
		return 'k', true
	case keyL:
		if shift {
			return 'L', true
		}
		return 'l', true
	case keyM:
		if shift {
			return 'M', true
		}
		return 'm', true
	case keyN:
		if shift {
			return 'N', true
		}
		return 'n', true
	case keyO:
		if shift {
			return 'O', true
		}
		return 'o', true
	case keyP:
		if shift {
			return 'P', true
		}
		return 'p', true
	case keyQ:
		if shift {
			return 'Q', true
		}
		return 'q', true
	case keyR:
		if shift {
			return 'R', true
		}
		return 'r', true
	case keyS:
		if shift {
			return 'S', true
		}
		return 's', true
	case keyT:
		if shift {
			return 'T', true
		}
		return 't', true
	case keyU:
		if shift {
			return 'U', true
		}
		return 'u', true
	case keyV:
		if shift {
			return 'V', true
		}
		return 'v', true
	case keyW:
		if shift {
			return 'W', true
		}
		return 'w', true
	case keyX:
		if shift {
			return 'X', true
		}
		return 'x', true
	case keyY:
		if shift {
			return 'Y', true
		}
		return 'y', true
	case keyZ:
		if shift {
			return 'Z', true
		}
		return 'z', true
	case key0:
		return '0', true
	case key1:
		return '1', true
	case key2:
		return '2', true
	case key3:
		return '3', true
	case key4:
		return '4', true
	case key5:
		return '5', true
	case key6:
		return '6', true
	case key7:
		return '7', true
	case key8:
		return '8', true
	case key9:
		return '9', true
	case keySpace:
		return ' ', true
	case keyPeriod:
		if shift {
			return '>', true
		}
		return '.', true
	case keyComma:
		if shift {
			return '<', true
		}
		return ',', true
	case keyMinus:
		if shift {
			return '_', true
		}
		return '-', true
	case keyEquals:
		if shift {
			return '+', true
		}
		return '=', true
	case keySlash:
		if shift {
			return '?', true
		}
		return '/', true
	}
	return 0, false
}

func keyName(k keyCode) string {
	switch k {
	case keyUp:
		return "Up"
	case keyDown:
		return "Down"
	case keyPageUp:
		return "PageUp"
	case keyPageDown:
		return "PageDown"
	case keyHome:
		return "Home"
	case keyEnd:
		return "End"
	case keyEscape:
		return "Escape"
	case keyTab:
		return "Tab"
	case keyBackspace:
		return "Backspace"
	case keyDelete:
		return "Delete"
	case keyReturn:
		return "Return"
	case keyKpEnter:
		return "KpEnter"
	case keyLeft:
		return "Left"
	case keyRight:
		return "Right"
	case keyLcmd:
		return "LCmd"
	case keyRcmd:
		return "RCmd"
	case keySlash:
		return "Slash"
	case keyComma:
		return "Comma"
	case keyPeriod:
		return "Period"
	case keySpace:
		return "Space"
	default:
		return "Key"
	}
}
