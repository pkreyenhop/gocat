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
	hay := []rune("abc abc abc")
	needle := []rune("abc")

	// Start at 1 -> first match at 4
	pos, ok := FindInDir(hay, needle, 1, DirFwd, false)
	if !ok || pos != 4 {
		t.Fatalf("expected ok=true pos=4, got ok=%v pos=%d", ok, pos)
	}
}

func TestFindInDir_Forward_Wrap(t *testing.T) {
	hay := []rune("abc abc abc")
	needle := []rune("abc")

	// Start after last match; with wrap should return first match at 0
	pos, ok := FindInDir(hay, needle, len(hay), DirFwd, true)
	if !ok || pos != 0 {
		t.Fatalf("expected ok=true pos=0, got ok=%v pos=%d", ok, pos)
	}
}

func TestFindInDir_Backward_NoWrap(t *testing.T) {
	hay := []rune("abc abc abc")
	needle := []rune("abc")

	// Starting at 5, going back should find match at 4
	pos, ok := FindInDir(hay, needle, 5, DirBack, false)
	if !ok || pos != 4 {
		t.Fatalf("expected ok=true pos=4, got ok=%v pos=%d", ok, pos)
	}
}

func TestFindInDir_Backward_Wrap(t *testing.T) {
	hay := []rune("abc abc abc")
	needle := []rune("abc")

	// Start at 0 going back: without wrap would miss; with wrap should find last match at 8
	pos, ok := FindInDir(hay, needle, 0, DirBack, true)
	if !ok || pos != 8 {
		t.Fatalf("expected ok=true pos=8, got ok=%v pos=%d", ok, pos)
	}
}

func TestLeap_AnchoredAtOrigin_Forward(t *testing.T) {
	run(t, "xx hello xx hello xx", 0, func(f *fixture) {
		f.leap(DirFwd, "hello")
		f.expectCaret(3)
		f.commit()
		f.expectLastCommit("hello")
	})
}

func TestLeap_AnchoredAtOrigin_Backward(t *testing.T) {
	run(t, "aa hello bb hello cc", 20, func(f *fixture) {
		f.leap(DirBack, "hello")
		f.expectCaret(12) // second hello
	})
}

func TestLeapCancel_RestoresOrigin_AndClearsSelectionFromThisLeap(t *testing.T) {
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
	s := Sel{Active: true, A: 10, B: 3}
	a, b := s.Normalised()
	if a != 3 || b != 10 {
		t.Fatalf("expected (3,10), got (%d,%d)", a, b)
	}
}

func TestInsert_ReplacesSelection(t *testing.T) {
	run(t, "hello world", 11, func(f *fixture) {
		f.selectRange(6, 11) // "world"
		f.ed.InsertText("cat")

		f.expectBuffer("hello cat")
		f.expectSelection(false, 0, 0)
		f.expectCaret(9) // "hello " (6) + "cat" (3)
	})
}

func TestLeapAgain_UsesLastCommit_NextMatch_Forward_WithWrap(t *testing.T) {
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
	run(t, "aa hello bb hello cc", 0, func(f *fixture) {
		f.leap(DirFwd, "")
		f.startSelection()
		f.ed.LeapAppend("hello")

		f.expectSelection(true, 0, 3)
		f.expectCaret(3)
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
