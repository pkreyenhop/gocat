package editor

import "testing"

// Tests are written scenario-first using a small fixture DSL:
//   run(t, "buffer", caretPos, func(f *fixture) {
//     f.leap(DirFwd, "hello")
//     f.commit()
//     f.expectCaret(3)
//   })
// This keeps future behavioural specs concise while exercising only headless logic.

// Helper: build editor with buffer and caret.
func newEd(buf string, caret int) *Editor {
	ed := NewEditor(buf)
	ed.Caret = caret
	return ed
}

func TestFindInDir_Forward_NoWrap(t *testing.T) {
	// Searches forward from offset 1; should skip the match at 0 and land on the
	// next match at 4 without wrapping.
	hay := []rune("abc abc abc")
	needle := []rune("abc")

	// Start at 1 -> first match at 4
	pos, ok := FindInDir(hay, needle, 1, DirFwd, false)
	if !ok || pos != 4 {
		t.Fatalf("expected ok=true pos=4, got ok=%v pos=%d", ok, pos)
	}
}

func TestFindInDir_Forward_Wrap(t *testing.T) {
	// Forward search that starts at the end; with wrap=true it should circle
	// around and find the very first occurrence.
	hay := []rune("abc abc abc")
	needle := []rune("abc")

	// Start after last match; with wrap should return first match at 0
	pos, ok := FindInDir(hay, needle, len(hay), DirFwd, true)
	if !ok || pos != 0 {
		t.Fatalf("expected ok=true pos=0, got ok=%v pos=%d", ok, pos)
	}
}

func TestFindInDir_Backward_NoWrap(t *testing.T) {
	// Backward search starting in the middle; should return the previous match
	// without wrapping to the end.
	hay := []rune("abc abc abc")
	needle := []rune("abc")

	// Starting at 5, going back should find match at 4
	pos, ok := FindInDir(hay, needle, 5, DirBack, false)
	if !ok || pos != 4 {
		t.Fatalf("expected ok=true pos=4, got ok=%v pos=%d", ok, pos)
	}
}

func TestFindInDir_Backward_Wrap(t *testing.T) {
	// Backward search from the very start; wrap=true should allow it to find the
	// last occurrence at the tail of the buffer.
	hay := []rune("abc abc abc")
	needle := []rune("abc")

	// Start at 0 going back: without wrap would miss; with wrap should find last match at 8
	pos, ok := FindInDir(hay, needle, 0, DirBack, true)
	if !ok || pos != 8 {
		t.Fatalf("expected ok=true pos=8, got ok=%v pos=%d", ok, pos)
	}
}

func TestFindInDir_IgnoresCase(t *testing.T) {
	hay := []rune("One two ONE")
	needle := []rune("one")

	if pos, ok := FindInDir(hay, needle, 0, DirFwd, true); !ok || pos != 0 {
		t.Fatalf("forward case-insensitive: pos=%d ok=%v", pos, ok)
	}
	if pos, ok := FindInDir(hay, []rune("ONE"), len(hay), DirBack, true); !ok || pos != len([]rune("One two ")) {
		t.Fatalf("backward case-insensitive: pos=%d ok=%v", pos, ok)
	}
}

func TestDeleteWordAtCaretEdgeCases(t *testing.T) {
	run(t, "abc!", 4, func(f *fixture) {
		// Caret at end should delete word to the left.
		f.ed.Caret = len(f.ed.Buf)
		if !f.ed.DeleteWordAtCaret() {
			f.t.Fatal("expected delete at end to succeed")
		}
		f.expectBuffer("!")
		f.expectCaret(0)
	})

	run(t, "abc!", 3, func(f *fixture) {
		// Caret on punctuation should delete the punctuation only.
		if !f.ed.DeleteWordAtCaret() {
			f.t.Fatal("expected delete on punctuation")
		}
		f.expectBuffer("abc")
		f.expectCaret(3)
	})
}

func TestLeap_AnchoredAtOrigin_Forward(t *testing.T) {
	// Leap refinements are anchored at the origin caret; this confirms a forward
	// leap moves from position 0 to the first "hello" while committing the query.
	run(t, "xx hello xx hello xx", 0, func(f *fixture) {
		f.leap(DirFwd, "hello")
		f.expectCaret(3)
		f.commit()
		f.expectLastCommit("hello")
	})
}

func TestLeap_AnchoredAtOrigin_Backward(t *testing.T) {
	// Backward leap starts refining at the origin (end of buffer here) and lands
	// on the previous "hello" occurrence.
	run(t, "aa hello bb hello cc", 20, func(f *fixture) {
		f.leap(DirBack, "hello")
		f.expectCaret(12) // second hello
	})
}

func TestLeapCancel_RestoresOrigin_AndClearsSelectionFromThisLeap(t *testing.T) {
	// Cancel should return the caret to where the leap began and drop only the
	// selection that started during this leap, leaving no active selection.
	run(t, "one two three two", 0, func(f *fixture) {
		f.leap(DirFwd, "two")
		f.expectCaret(4)

		// simulate dual-leap selection during this leap
		f.ed.Leap.Selecting = true
		f.ed.Leap.SelAnchor = 0
		f.ed.Sel.Active = true
		f.ed.Sel.A, f.ed.Sel.B = 0, 4

		f.cancel()
		f.expectCaret(0)
		f.expectSelection(false, 0, 0)
		f.expectLeapActive(false)
	})
}

func TestSelection_Normalised(t *testing.T) {
	// Normalised should always return the ascending range regardless of the
	// order they were set, keeping assertions simple.
	s := Sel{Active: true, A: 10, B: 3}
	a, b := s.Normalised()
	if a != 3 || b != 10 {
		t.Fatalf("expected (3,10), got (%d,%d)", a, b)
	}
}

func TestInsert_ReplacesSelection(t *testing.T) {
	// Inserting text while a selection is active should replace that selection,
	// clear the selection flag, and place the caret after the inserted text.
	run(t, "hello world", 11, func(f *fixture) {
		f.selectRange(6, 11) // "world"
		f.ed.InsertText("cat")

		f.expectBuffer("hello cat")
		f.expectSelection(false, 0, 0)
		f.expectCaret(9) // "hello " (6) + "cat" (3)
	})
}

func TestLeapAgain_UsesLastCommit_NextMatch_Forward_WithWrap(t *testing.T) {
	// LeapAgain repeats the last committed query; forward direction should step
	// to the next match from just after the current caret and wrap to the start
	// when no further matches remain.
	run(t, "x aa x aa x aa", 0, func(f *fixture) {
		f.leap(DirFwd, "aa")
		f.expectCaret(2)
		f.commit()

		f.leapAgain(DirFwd)
		f.expectCaret(7)

		f.leapAgain(DirFwd)
		f.expectCaret(12)

		f.leapAgain(DirFwd)
		f.expectCaret(2) // wrap
	})
}

func TestLeapAgain_UsesLastCommit_PrevMatch_Backward_WithWrap(t *testing.T) {
	// Backward LeapAgain starts one rune before the caret, finds the previous
	// occurrence, and wraps to the end after the first match.
	run(t, "x aa x aa x aa", 12, func(f *fixture) {
		f.ed.Leap.LastCommit = []rune("aa")

		f.leapAgain(DirBack)
		f.expectCaret(7)

		f.leapAgain(DirBack)
		f.expectCaret(2)

		f.leapAgain(DirBack)
		f.expectCaret(12) // wrap
	})
}

func TestSelecting_UpdatesSelectionOnLeapSearch(t *testing.T) {
	// When selection is active during a leap, refining the query should move the
	// caret and extend the selection to that new caret position.
	run(t, "aa hello bb hello cc", 0, func(f *fixture) {
		f.leap(DirFwd, "")
		f.startSelection()
		f.ed.LeapAppend("hello")

		f.expectSelection(true, 0, 3)
		f.expectCaret(3)
	})
}

func TestMoveCaretLineUpDown(t *testing.T) {
	// Up/Down moves by whole lines while clamping column and respects selection extension.
	run(t, "abc\ndef\ng", 4, func(f *fixture) {
		// caret at line 1, col 0 (buffer position 4)
		lines := SplitLines(f.ed.Buf)
		f.ed.MoveCaretLine(lines, 1, false) // down
		f.expectCaret(8)                    // line 2 len=1 -> pos 8

		// extend selection upward
		lines = SplitLines(f.ed.Buf)
		f.ed.MoveCaretLine(lines, -1, true)
		f.expectCaret(4)
		f.expectSelection(true, 4, 8)
	})
}

func TestMoveCaretPage(t *testing.T) {
	buf := "l0\nl1\nl2\nl3\nl4\nl5\nl6\nl7\nl8\nl9\n"
	run(t, buf, 0, func(f *fixture) {
		lines := SplitLines(f.ed.Buf)
		f.ed.MoveCaretPage(lines, 5, DirFwd, false)
		// After 5 lines down, caret should be at start of line 5 (0-indexed)
		pos := 0
		for i := range 5 {
			pos += len([]rune(lines[i])) + 1
		}
		f.expectCaret(pos)

		lines = SplitLines(f.ed.Buf)
		f.ed.MoveCaretPage(lines, 3, DirBack, true) // extend selection up 3 lines
		lines = SplitLines(f.ed.Buf)
		posBack := 0
		for i := range 2 { // back to line 2
			posBack += len([]rune(lines[i])) + 1
		}
		f.expectCaret(posBack)
		f.expectSelection(true, posBack, pos)
	})
}

func TestCaretToLineEdgesAndKill(t *testing.T) {
	run(t, "abc\ndef\n", 4, func(f *fixture) {
		lines := SplitLines(f.ed.Buf)
		f.ed.CaretToLineEdge(lines, true, false)
		f.expectCaret(7) // end of "def"

		lines = SplitLines(f.ed.Buf)
		f.ed.CaretToLineEdge(lines, false, true)
		f.expectCaret(4)
		f.expectSelection(true, 4, 7)

		lines = SplitLines(f.ed.Buf)
		f.ed.KillToLineEnd(lines)
		f.expectBuffer("abc\n")
	})

	// Kill from end of last line should leave buffer unchanged
	run(t, "hi\nthere", len("hi\nthere"), func(f *fixture) {
		lines := SplitLines(f.ed.Buf)
		f.ed.KillToLineEnd(lines)
		f.expectBuffer("hi\nthere")
	})

	// Kill from middle of line should remove newline too
	run(t, "ab\ncd\nef", 3, func(f *fixture) {
		lines := SplitLines(f.ed.Buf)
		f.ed.KillToLineEnd(lines)
		f.expectBuffer("ab\nef")
	})
}

func TestCaretToBufferEdge(t *testing.T) {
	run(t, "ab\ncd\nef", 3, func(f *fixture) {
		lines := SplitLines(f.ed.Buf)
		f.ed.CaretToBufferEdge(lines, false, false)
		f.expectCaret(0)

		lines = SplitLines(f.ed.Buf)
		f.ed.CaretToBufferEdge(lines, true, true)
		f.expectCaret(len(f.ed.Buf))
		f.expectSelection(true, 0, len(f.ed.Buf))
	})
}

func TestUndoRestoresLastState(t *testing.T) {
	run(t, "abc", 3, func(f *fixture) {
		f.ed.InsertText("d")
		f.expectBuffer("abcd")
		f.ed.BackspaceOrDeleteSelection(true)
		f.expectBuffer("abc")

		f.ed.Undo() // undo backspace
		f.expectBuffer("abcd")

		f.ed.Undo() // undo insert
		f.expectBuffer("abc")
		f.expectCaret(3)
	})
}

func TestUndoNoHistoryIsSafe(t *testing.T) {
	run(t, "abc", 1, func(f *fixture) {
		f.ed.Undo() // nothing recorded
		f.expectBuffer("abc")
		f.expectCaret(1)
	})
}

func TestMoveCaretPageClampsWithinBuffer(t *testing.T) {
	buf := "short\nline\n"
	run(t, buf, 0, func(f *fixture) {
		lines := SplitLines(f.ed.Buf)
		f.ed.MoveCaretPage(lines, 50, DirBack, false)
		f.expectCaret(0)
		lines = SplitLines(f.ed.Buf)
		f.ed.MoveCaretPage(lines, 50, DirFwd, false)
		f.expectCaret(len(f.ed.Buf)) // clamp to end with large page
	})
}

func TestCaretToBufferEdgeSelectionExtends(t *testing.T) {
	run(t, "ab\ncd\nef", 2, func(f *fixture) {
		lines := SplitLines(f.ed.Buf)
		f.ed.CaretToBufferEdge(lines, true, true)
		f.expectCaret(len(f.ed.Buf))
		f.expectSelection(true, 2, len(f.ed.Buf))
	})
}

// ========
// Helpers
// ========

type fixture struct {
	t  *testing.T
	ed *Editor
}

// Fixture helpers keep tests declarative: call `leap`, `leapAgain`, `commit`, `cancel`,
// or selection helpers, then assert via `expectCaret`, `expectBuffer`, etc.
func run(t *testing.T, buf string, caret int, fn func(f *fixture)) {
	t.Helper()
	fn(&fixture{t: t, ed: newEd(buf, caret)})
}

func (f *fixture) leap(dir Dir, query string) {
	f.ed.LeapStart(dir)
	if query != "" {
		f.ed.LeapAppend(query)
	}
}

func (f *fixture) leapAgain(dir Dir) {
	f.ed.LeapAgain(dir)
}

func (f *fixture) commit() {
	f.ed.LeapEndCommit()
}

func (f *fixture) cancel() {
	f.ed.LeapCancel()
}

func (f *fixture) startSelection() {
	f.ed.Leap.Selecting = true
	f.ed.Leap.SelAnchor = f.ed.Caret
	f.ed.Sel.Active = true
	f.ed.Sel.A, f.ed.Sel.B = f.ed.Caret, f.ed.Caret
}

func (f *fixture) selectRange(a, b int) {
	f.ed.Sel.Active = true
	f.ed.Sel.A = a
	f.ed.Sel.B = b
}

func (f *fixture) expectCaret(want int) {
	f.t.Helper()
	if f.ed.Caret != want {
		f.t.Fatalf("caret: want %d, got %d", want, f.ed.Caret)
	}
}

func (f *fixture) expectLastCommit(want string) {
	f.t.Helper()
	if got := string(f.ed.Leap.LastCommit); got != want {
		f.t.Fatalf("lastCommit: want %q, got %q", want, got)
	}
}

func (f *fixture) expectBuffer(want string) {
	f.t.Helper()
	if got := string(f.ed.Buf); got != want {
		f.t.Fatalf("buffer: want %q, got %q", want, got)
	}
}

func (f *fixture) expectSelection(active bool, a, b int) {
	f.t.Helper()
	if f.ed.Sel.Active != active {
		f.t.Fatalf("selection active: want %v, got %v", active, f.ed.Sel.Active)
	}
	if !active {
		return
	}
	gotA, gotB := f.ed.Sel.Normalised()
	if gotA != a || gotB != b {
		f.t.Fatalf("selection range: want (%d,%d), got (%d,%d)", a, b, gotA, gotB)
	}
}

func (f *fixture) expectLeapActive(active bool) {
	f.t.Helper()
	if f.ed.Leap.Active != active {
		f.t.Fatalf("leap active: want %v, got %v", active, f.ed.Leap.Active)
	}
}
