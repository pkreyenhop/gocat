// Package editor provides headless editing and Canon-Cat-inspired Leap logic.
package editor

import "unicode"

// Core editing and Leap logic. This package is UI-agnostic to keep logic testable.

type Dir int

const (
	DirBack Dir = -1
	DirFwd  Dir = 1
)

// Sel represents a selection range.
type Sel struct {
	Active bool
	A      int // inclusive
	B      int // exclusive-ish in rendering; we normalise anyway
}

func (s Sel) Normalised() (int, int) {
	if !s.Active {
		return 0, 0
	}
	if s.A <= s.B {
		return s.A, s.B
	}
	return s.B, s.A
}

type LeapState struct {
	Active       bool
	Dir          Dir
	Query        []rune
	OriginCaret  int
	LastFoundPos int

	HeldL bool
	HeldR bool

	// Selection while both Leap keys are involved
	Selecting  bool
	SelAnchor  int
	LastSrc    string // "textinput" or "keydown"
	LastCommit []rune // last committed query for Leap Again
}

// Clipboard abstracts clipboard operations for testability.
type Clipboard interface {
	GetText() (string, error)
	SetText(string) error
}

// Editor holds buffer state, caret/selection, Leap state, and clipboard.
type Editor struct {
	Buf   []rune
	Caret int
	Sel   Sel
	Leap  LeapState

	clip Clipboard
	undo []undoState
}

type undoState struct {
	buf   []rune
	caret int
	sel   Sel
}

func NewEditor(initial string) *Editor {
	return &Editor{
		Buf:  []rune(initial),
		Leap: LeapState{LastFoundPos: -1},
	}
}

// SetClipboard injects a clipboard implementation.
func (e *Editor) SetClipboard(c Clipboard) {
	e.clip = c
}

// ======================
// Leap + selection logic
// ======================

func (e *Editor) LeapStart(dir Dir) {
	e.Leap.Active = true
	e.Leap.Dir = dir
	e.Leap.OriginCaret = e.Caret
	e.Leap.Query = e.Leap.Query[:0]
	e.Leap.LastFoundPos = -1
	e.Leap.Selecting = false
	e.Leap.LastSrc = ""
	// Starting a leap does not clear an existing selection (Cat keeps it until you do something),
	// but editing will replace it.
}

func (e *Editor) LeapEndCommit() {
	// Commit: keep caret where it is.
	// Store query for Leap Again.
	if len(e.Leap.Query) > 0 {
		e.Leap.LastCommit = append(e.Leap.LastCommit[:0], e.Leap.Query...)
	}

	// If selecting, selection remains active (already tracked).
	// If not selecting, we leave selection as-is.

	e.Leap.Active = false
	e.Leap.Query = e.Leap.Query[:0]
	e.Leap.LastFoundPos = -1
	e.Leap.Selecting = false
	e.Leap.LastSrc = ""
}

func (e *Editor) LeapCancel() {
	// Cancel leap: return to origin; also cancel selection that started during this leap.
	e.Caret = e.Leap.OriginCaret
	if e.Leap.Selecting {
		e.Sel.Active = false
	}
	e.Leap.Active = false
	e.Leap.Query = e.Leap.Query[:0]
	e.Leap.LastFoundPos = -1
	e.Leap.Selecting = false
	e.Leap.LastSrc = ""
}

func (e *Editor) LeapAppend(text string) {
	e.Leap.Query = append(e.Leap.Query, []rune(text)...)
	e.leapSearch()
}

func (e *Editor) LeapBackspace() {
	if len(e.Leap.Query) == 0 {
		return
	}
	e.Leap.Query = e.Leap.Query[:len(e.Leap.Query)-1]
	e.leapSearch()
}

func (e *Editor) BeginLeapSelection() {
	if e.Leap.Selecting {
		return
	}
	e.Leap.Selecting = true
	e.Leap.SelAnchor = e.Caret
	e.Sel.Active = true
	e.Sel.A = e.Leap.SelAnchor
	e.Sel.B = e.Caret
}

func (e *Editor) leapSearch() {
	if len(e.Leap.Query) == 0 {
		e.Caret = e.Leap.OriginCaret
		e.Leap.LastFoundPos = -1
		if e.Leap.Selecting {
			e.updateSelectionWithCaret()
		}
		return
	}

	// Canon Cat feel: refine anchored at origin
	start := e.Leap.OriginCaret

	if pos, ok := FindInDir(e.Buf, e.Leap.Query, start, e.Leap.Dir, true /*wrap*/); ok {
		e.Caret = pos
		e.Leap.LastFoundPos = pos
	} else {
		e.Leap.LastFoundPos = -1
	}
	if e.Leap.Selecting {
		e.updateSelectionWithCaret()
	}
}

func (e *Editor) updateSelectionWithCaret() {
	e.Sel.Active = true
	e.Sel.A = e.Leap.SelAnchor
	e.Sel.B = e.Caret
}

func (e *Editor) LeapAgain(dir Dir) {
	if len(e.Leap.LastCommit) == 0 {
		return
	}
	q := e.Leap.LastCommit

	// Start after/before caret to get "next" behaviour.
	start := e.Caret
	if dir == DirFwd {
		start = min(len(e.Buf), e.Caret+1)
	} else {
		start = max(0, e.Caret-1)
	}

	if pos, ok := FindInDir(e.Buf, q, start, dir, true /*wrap*/); ok {
		e.Caret = pos
	}
}

// ======================
// Editing + selection
// ======================

func (e *Editor) InsertText(text string) {
	// Replace selection if active
	e.recordUndo()
	if e.Sel.Active {
		e.deleteSelection()
	}
	rs := []rune(text)
	if len(rs) == 0 {
		return
	}
	e.Caret = clamp(e.Caret, 0, len(e.Buf))
	e.Buf = append(e.Buf[:e.Caret], append(rs, e.Buf[e.Caret:]...)...)
	e.Caret += len(rs)
}

func (e *Editor) BackspaceOrDeleteSelection(isBackspace bool) {
	e.recordUndo()
	if e.Sel.Active {
		e.deleteSelection()
		return
	}
	if len(e.Buf) == 0 {
		return
	}
	if isBackspace {
		if e.Caret <= 0 {
			return
		}
		e.Buf = append(e.Buf[:e.Caret-1], e.Buf[e.Caret:]...)
		e.Caret--
		return
	}
	// delete forward
	if e.Caret >= len(e.Buf) {
		return
	}
	e.Buf = append(e.Buf[:e.Caret], e.Buf[e.Caret+1:]...)
}

// DeleteWordAtCaret removes the word under the caret (letters/digits/underscore).
// If the caret is on a non-word rune, deletes that single rune instead.
func (e *Editor) DeleteWordAtCaret() bool {
	if e == nil {
		return false
	}
	isWord := func(r rune) bool {
		return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
	}
	e.recordUndo()
	if e.Sel.Active {
		e.deleteSelection()
		return true
	}
	if len(e.Buf) == 0 || e.Caret >= len(e.Buf) {
		if e.Caret == 0 {
			return false
		}
		// At or past the end: skip trailing non-word run, then delete the word to the left.
		idx := e.Caret - 1
		for idx >= 0 && !isWord(e.Buf[idx]) {
			idx--
		}
		if idx < 0 {
			return false
		}
		end := idx + 1
		start := idx
		for start > 0 && isWord(e.Buf[start-1]) {
			start--
		}
		e.Buf = append(e.Buf[:start], e.Buf[end:]...)
		e.Caret = start
		return true
	}
	r := e.Buf[e.Caret]
	if !isWord(r) {
		// If caret is on whitespace right after a word, delete that word instead.
		if unicode.IsSpace(r) && e.Caret > 0 && isWord(e.Buf[e.Caret-1]) {
			start := e.Caret - 1
			for start > 0 && isWord(e.Buf[start-1]) {
				start--
			}
			end := e.Caret
			e.Buf = append(e.Buf[:start], e.Buf[end:]...)
			e.Caret = start
			return true
		}
		e.Buf = append(e.Buf[:e.Caret], e.Buf[e.Caret+1:]...)
		return true
	}
	start := e.Caret
	for start > 0 && isWord(e.Buf[start-1]) {
		start--
	}
	end := e.Caret
	for end < len(e.Buf) && isWord(e.Buf[end]) {
		end++
	}
	e.Buf = append(e.Buf[:start], e.Buf[end:]...)
	e.Caret = start
	return true
}

// DeleteLineAtCaret removes the entire line containing the caret.
func (e *Editor) DeleteLineAtCaret() bool {
	if e == nil {
		return false
	}
	e.recordUndo()
	lines := SplitLines(e.Buf)
	if len(lines) == 0 {
		return false
	}
	lineIdx, _ := LineColForPos(lines, e.Caret)
	if lineIdx < 0 || lineIdx >= len(lines) {
		return false
	}
	start := 0
	for i := 0; i < lineIdx; i++ {
		start += len([]rune(lines[i])) + 1
	}
	end := start + len([]rune(lines[lineIdx]))
	// remove newline if not last line
	if lineIdx < len(lines)-1 {
		end++
	}
	if end < start {
		return false
	}
	if start > len(e.Buf) {
		start = len(e.Buf)
	}
	if end > len(e.Buf) {
		end = len(e.Buf)
	}
	e.Buf = append(e.Buf[:start], e.Buf[end:]...)
	e.Caret = clamp(start, 0, len(e.Buf))
	e.Sel.Active = false
	return true
}

func (e *Editor) deleteSelection() {
	a, b := e.Sel.Normalised()
	a = clamp(a, 0, len(e.Buf))
	b = clamp(b, 0, len(e.Buf))
	if a == b {
		e.Sel.Active = false
		return
	}
	e.Buf = append(e.Buf[:a], e.Buf[b:]...)
	e.Caret = a
	e.Sel.Active = false
}

// Undo restores the most recent recorded state (single-step).
func (e *Editor) Undo() {
	if len(e.undo) == 0 {
		return
	}
	last := e.undo[len(e.undo)-1]
	e.undo = e.undo[:len(e.undo)-1]
	e.Buf = append([]rune(nil), last.buf...)
	e.Caret = last.caret
	e.Sel = last.sel
	e.Leap = LeapState{LastFoundPos: -1}
}

func (e *Editor) recordUndo() {
	snap := undoState{
		buf:   append([]rune(nil), e.Buf...),
		caret: e.Caret,
		sel:   e.Sel,
	}
	e.undo = append(e.undo, snap)
	if len(e.undo) > 256 {
		e.undo = e.undo[len(e.undo)-256:]
	}
}

func (e *Editor) MoveCaret(delta int, extendSelection bool) {
	newPos := clamp(e.Caret+delta, 0, len(e.Buf))
	if extendSelection {
		if !e.Sel.Active {
			e.Sel.Active = true
			e.Sel.A = e.Caret
			e.Sel.B = newPos
		} else {
			e.Sel.B = newPos
		}
	} else {
		e.Sel.Active = false
	}
	e.Caret = newPos
}

// MoveCaretLine moves caret by whole lines using a line/col mapping.
func (e *Editor) MoveCaretLine(lines []string, deltaLines int, extendSelection bool) {
	if deltaLines == 0 {
		return
	}
	curLine, curCol := LineColForPos(lines, e.Caret)
	targetLine := clamp(curLine+deltaLines, 0, len(lines)-1)
	// Clamp col to target line length
	targetCol := min(curCol, len([]rune(lines[targetLine])))

	// Compute new caret absolute position
	pos := 0
	for i := range targetLine {
		pos += len([]rune(lines[i])) + 1 // include newline
	}
	pos += targetCol

	if extendSelection {
		if !e.Sel.Active {
			e.Sel.Active = true
			e.Sel.A = e.Caret
			e.Sel.B = pos
		} else {
			e.Sel.B = pos
		}
	} else {
		e.Sel.Active = false
	}
	e.Caret = pos
}

// MoveCaretPage moves by a page worth of lines (positive for down, negative for up).
func (e *Editor) MoveCaretPage(lines []string, pageLines int, dir Dir, extendSelection bool) {
	if pageLines <= 0 {
		return
	}
	delta := pageLines
	if dir == DirBack {
		delta = -pageLines
	}
	e.MoveCaretLine(lines, delta, extendSelection)
}

// CaretToLineEdge moves caret to start or end of the current line.
func (e *Editor) CaretToLineEdge(lines []string, toEnd bool, extendSelection bool) {
	lineIdx, _ := LineColForPos(lines, e.Caret)
	if lineIdx < 0 || lineIdx >= len(lines) {
		return
	}
	targetCol := 0
	if toEnd {
		targetCol = len([]rune(lines[lineIdx]))
	}
	e.moveCaretTo(lineIdx, targetCol, lines, extendSelection)
}

// CaretToBufferEdge moves caret to start or end of buffer.
func (e *Editor) CaretToBufferEdge(lines []string, toEnd bool, extendSelection bool) {
	if len(lines) == 0 {
		return
	}
	targetLine := 0
	targetCol := 0
	if toEnd {
		targetLine = len(lines) - 1
		targetCol = len([]rune(lines[targetLine]))
	}
	e.moveCaretTo(targetLine, targetCol, lines, extendSelection)
}

func (e *Editor) moveCaretTo(lineIdx int, col int, lines []string, extendSelection bool) {
	if lineIdx < 0 {
		lineIdx = 0
	}
	if lineIdx >= len(lines) {
		lineIdx = len(lines) - 1
	}
	pos := 0
	for i := 0; i < lineIdx; i++ {
		pos += len([]rune(lines[i])) + 1
	}
	pos += col

	if extendSelection {
		if !e.Sel.Active {
			e.Sel.Active = true
			e.Sel.A = e.Caret
			e.Sel.B = pos
		} else {
			e.Sel.B = pos
		}
	} else {
		e.Sel.Active = false
	}
	e.Caret = pos
}

// KillToLineEnd deletes from caret to end-of-line (including newline if at EOL).
func (e *Editor) KillToLineEnd(lines []string) {
	e.recordUndo()
	lineIdx, col := LineColForPos(lines, e.Caret)
	if lineIdx < 0 || lineIdx >= len(lines) {
		return
	}
	lineLen := len([]rune(lines[lineIdx]))
	// At end of last line: nothing to kill.
	if lineIdx == len(lines)-1 && col == lineLen {
		e.Sel.Active = false
		return
	}
	pos := e.Caret
	target := pos + (lineLen - col)
	// Remove newline too if we're not on the last line.
	if lineIdx < len(lines)-1 {
		target++
	}
	if target > pos && target <= len(e.Buf) {
		e.Buf = append(e.Buf[:pos], e.Buf[target:]...)
	}
	e.Sel.Active = false
}

func (e *Editor) CopySelection() {
	if !e.Sel.Active || e.clip == nil {
		return
	}
	a, b := e.Sel.Normalised()
	a = clamp(a, 0, len(e.Buf))
	b = clamp(b, 0, len(e.Buf))
	if a == b {
		return
	}
	_ = e.clip.SetText(string(e.Buf[a:b]))
}

func (e *Editor) CutSelection() {
	e.recordUndo()
	if !e.Sel.Active || e.clip == nil {
		return
	}
	e.CopySelection()
	e.deleteSelection()
}

func (e *Editor) PasteClipboard() {
	if e.clip == nil {
		return
	}
	txt, err := e.clip.GetText()
	if err != nil || txt == "" {
		return
	}
	e.InsertText(txt)
}

// ======================
// Line/col mapping
// ======================

// SplitLines splits a rune buffer into lines separated by '\n'.
func SplitLines(buf []rune) []string {
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

// LineColForPos converts a buffer position to (line, col) assuming lines from SplitLines.
func LineColForPos(lines []string, pos int) (int, int) {
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

func CaretLineAt(lines []string, caret int) int {
	ln, _ := LineColForPos(lines, caret)
	return ln
}

func CaretColAt(lines []string, caret int) int {
	_, col := LineColForPos(lines, caret)
	return col
}

// ======================
// Search
// ======================

// FindInDir searches for needle starting near start in the given direction, optionally wrapping.
// The search is case-insensitive.
func FindInDir(hay []rune, needle []rune, start int, dir Dir, wrap bool) (int, bool) {
	if len(needle) == 0 {
		return start, true
	}
	if len(hay) == 0 || len(needle) > len(hay) {
		return -1, false
	}
	hayFold := unicode.ToLower
	needleFold := unicode.ToLower
	start = clamp(start, 0, len(hay))

	if dir == DirFwd {
		if pos, ok := scanFwdFold(hay, needle, start, hayFold, needleFold); ok {
			return pos, true
		}
		if wrap {
			return scanFwdFold(hay, needle, 0, hayFold, needleFold)
		}
		return -1, false
	}

	// backward
	searchStart := start - 1 // search strictly before start to get the previous match
	if pos, ok := scanBackFold(hay, needle, searchStart, hayFold, needleFold); ok {
		return pos, true
	}
	if wrap {
		return scanBackFold(hay, needle, len(hay), hayFold, needleFold)
	}
	return -1, false
}

func scanFwdFold(hay, needle []rune, start int, hf, nf func(rune) rune) (int, bool) {
	for i := start; i+len(needle) <= len(hay); i++ {
		if matchAtFold(hay, needle, i, hf, nf) {
			return i, true
		}
	}
	return -1, false
}

func scanBackFold(hay, needle []rune, start int, hf, nf func(rune) rune) (int, bool) {
	if start < 0 {
		return -1, false
	}
	lastStart := min(start, len(hay)-len(needle))
	for i := lastStart; i >= 0; i-- {
		if matchAtFold(hay, needle, i, hf, nf) {
			return i, true
		}
	}
	return -1, false
}

func matchAtFold(hay, needle []rune, i int, hf, nf func(rune) rune) bool {
	for j := range needle {
		if hf(hay[i+j]) != nf(needle[j]) {
			return false
		}
	}
	return true
}

// ======================
// Util
// ======================

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
