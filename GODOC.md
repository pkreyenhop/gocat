# GoDoc

Generated on 2026-02-28 with:

```sh
go doc -cmd .
go doc -all ./editor
```

## `gc` (command package)

```text
package main // import "."

```

### Runtime Modes (TUI)

- `Esc` command-prefix mode: if delayed, a lower-right grouped popup shows valid next-letter commands.
- `Esc+/` search mode: type pattern, press `/` to lock, then `Tab`/`Shift+Tab` navigate matches.
  - Locking an empty pattern (`/`) repeats the last non-empty search and jumps to the next match.
- `Esc+X` line-highlight mode: `x` extends highlighted lines; `Esc` exits.
- `Esc+Space` less mode: `Space` pages forward; `Esc` exits.

## `gc/editor`

```text
package editor // import "."

Package editor provides headless editing and Canon-Cat-inspired Leap logic.

FUNCTIONS

func CaretColAt(lines []string, caret int) int
func CaretLineAt(lines []string, caret int) int
func FindInDir(hay []rune, needle []rune, start int, dir Dir, wrap bool) (int, bool)
    FindInDir searches for needle starting near start in the given direction,
    optionally wrapping. The search is case-insensitive.

func LineColForPos(lines []string, pos int) (int, int)
    LineColForPos converts a buffer position to (line, col) assuming lines from
    SplitLines.

func SplitLines(buf []rune) []string
    SplitLines splits a rune buffer into lines separated by '\n'.


TYPES

type Clipboard interface {
	GetText() (string, error)
	SetText(string) error
}
    Clipboard abstracts clipboard operations for testability.

type Dir int

const (
	DirBack Dir = -1
	DirFwd  Dir = 1
)
type Editor struct {
	Buf   []rune
	Caret int
	Sel   Sel
	Leap  LeapState

	// Has unexported fields.
}
    Editor holds buffer state, caret/selection, Leap state, and clipboard.

func NewEditor(initial string) *Editor

func (e *Editor) BackspaceOrDeleteSelection(isBackspace bool)

func (e *Editor) CaretToBufferEdge(lines []string, toEnd bool, extendSelection bool)
    CaretToBufferEdge moves caret to start or end of buffer.

func (e *Editor) CaretToLineEdge(lines []string, toEnd bool, extendSelection bool)
    CaretToLineEdge moves caret to start or end of the current line.

func (e *Editor) CopySelection()

func (e *Editor) CutSelection()

func (e *Editor) DeleteLineAtCaret() bool
    DeleteLineAtCaret removes the entire line containing the caret.

func (e *Editor) DeleteWordAtCaret() bool
    DeleteWordAtCaret removes the word under the caret
    (letters/digits/underscore). If the caret is on a non-word rune, deletes
    that single rune instead.

func (e *Editor) InsertText(text string)

func (e *Editor) KillToLineEnd(lines []string)
    KillToLineEnd deletes from caret to end-of-line (including newline if at
    EOL).

func (e *Editor) LeapAgain(dir Dir)

func (e *Editor) LeapAppend(text string)

func (e *Editor) LeapBackspace()

func (e *Editor) LeapCancel()

func (e *Editor) LeapEndCommit()

func (e *Editor) LeapStart(dir Dir)

func (e *Editor) MoveCaret(delta int, extendSelection bool)

func (e *Editor) MoveCaretLine(lines []string, deltaLines int, extendSelection bool)
    MoveCaretLine moves caret by whole lines using a line/col mapping.

func (e *Editor) MoveCaretPage(lines []string, pageLines int, dir Dir, extendSelection bool)
    MoveCaretPage moves by a page worth of lines (positive for down, negative
    for up).

func (e *Editor) PasteClipboard()

func (e *Editor) SetClipboard(c Clipboard)
    SetClipboard injects a clipboard implementation.

func (e *Editor) Undo()
    Undo restores the most recent recorded state (single-step).

type LeapState struct {
	Active       bool
	Dir          Dir
	Query        []rune
	OriginCaret  int
	LastFoundPos int

	// Selection state while leap-driven selection is active.
	Selecting  bool
	SelAnchor  int
	LastSrc    string // "textinput" or "keydown"
	LastCommit []rune // last committed query for Leap Again
}

type Sel struct {
	Active bool
	A      int // inclusive
	B      int // selection endpoint; Normalised handles ordering
}
    Sel represents a selection range.

func (s Sel) Normalised() (int, int)

```
