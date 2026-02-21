package editor

// Core editing and Leap logic. This package is UI-agnostic to keep logic testable.

type Dir int

const (
	DirBack Dir = -1
	DirFwd  Dir = 1
)

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

type Editor struct {
	Buf   []rune
	Caret int
	Sel   Sel
	Leap  LeapState

	clip Clipboard
}

func NewEditor(initial string) *Editor {
	return &Editor{
		Buf:  []rune(initial),
		Leap: LeapState{LastFoundPos: -1},
	}
}

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

// Convert a buffer position to (line, col) assuming lines from splitLines.
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

func FindInDir(hay []rune, needle []rune, start int, dir Dir, wrap bool) (int, bool) {
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
