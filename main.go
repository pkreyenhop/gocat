package main

import "testing"

// Helper: build editor with buffer and caret.
func newEd(buf string, caret int) *Editor {
	return &Editor{
		buf:   []rune(buf),
		caret: caret,
		leap:  LeapState{lastFoundPos: -1},
	}
}

func TestFindInDir_Forward_NoWrap(t *testing.T) {
	hay := []rune("abc abc abc")
	needle := []rune("abc")

	// Start at 1 -> first match at 4
	pos, ok := findInDir(hay, needle, 1, DirFwd, false)
	if !ok || pos != 4 {
		t.Fatalf("expected ok=true pos=4, got ok=%v pos=%d", ok, pos)
	}
}

func TestFindInDir_Forward_Wrap(t *testing.T) {
	hay := []rune("abc abc abc")
	needle := []rune("abc")

	// Start after last match; with wrap should return first match at 0
	pos, ok := findInDir(hay, needle, len(hay), DirFwd, true)
	if !ok || pos != 0 {
		t.Fatalf("expected ok=true pos=0, got ok=%v pos=%d", ok, pos)
	}
}

func TestFindInDir_Backward_NoWrap(t *testing.T) {
	hay := []rune("abc abc abc")
	needle := []rune("abc")

	// Starting at 5, going back should find match at 4
	pos, ok := findInDir(hay, needle, 5, DirBack, false)
	if !ok || pos != 4 {
		t.Fatalf("expected ok=true pos=4, got ok=%v pos=%d", ok, pos)
	}
}

func TestFindInDir_Backward_Wrap(t *testing.T) {
	hay := []rune("abc abc abc")
	needle := []rune("abc")

	// Start at 0 going back: without wrap would miss; with wrap should find last match at 8
	pos, ok := findInDir(hay, needle, 0, DirBack, true)
	if !ok || pos != 8 {
		t.Fatalf("expected ok=true pos=8, got ok=%v pos=%d", ok, pos)
	}
}

func TestLeap_AnchoredAtOrigin_Forward(t *testing.T) {
	ed := newEd("xx hello xx hello xx", 0)

	ed.leapStart(DirFwd)
	ed.leapAppend("hello")

	if ed.caret != 3 {
		t.Fatalf("expected caret=3 (first 'hello'), got %d", ed.caret)
	}

	ed.leapEndCommit()
	if got := string(ed.leap.lastCommit); got != "hello" {
		t.Fatalf("expected lastCommit='hello', got %q", got)
	}
}

func TestLeap_AnchoredAtOrigin_Backward(t *testing.T) {
	ed := newEd("aa hello bb hello cc", 20) // near end
	ed.leapStart(DirBack)
	ed.leapAppend("hello")

	// Anchor at originCaret=20 and search backward should land on the second hello at index 12
	if ed.caret != 12 {
		t.Fatalf("expected caret=12, got %d", ed.caret)
	}
}

func TestLeapCancel_RestoresOrigin_AndClearsSelectionFromThisLeap(t *testing.T) {
	ed := newEd("one two three two", 0)

	ed.leapStart(DirFwd)
	ed.leapAppend("two")
	if ed.caret != 4 {
		t.Fatalf("expected caret=4, got %d", ed.caret)
	}

	// Simulate dual-leap selection started during this leap
	ed.leap.selecting = true
	ed.leap.selAnchor = 0
	ed.sel.active = true
	ed.sel.a, ed.sel.b = 0, 4

	ed.leapCancel()
	if ed.caret != 0 {
		t.Fatalf("expected caret restored to origin=0, got %d", ed.caret)
	}
	if ed.sel.active {
		t.Fatalf("expected selection cleared on leapCancel when selecting")
	}
	if ed.leap.active {
		t.Fatalf("expected leap inactive after cancel")
	}
}

func TestSelection_Normalised(t *testing.T) {
	s := Sel{active: true, a: 10, b: 3}
	a, b := s.normalised()
	if a != 3 || b != 10 {
		t.Fatalf("expected (3,10), got (%d,%d)", a, b)
	}
}

func TestInsert_ReplacesSelection(t *testing.T) {
	ed := newEd("hello world", 11)

	// Select "world"
	ed.sel.active = true
	ed.sel.a = 6
	ed.sel.b = 11

	ed.insertText("cat")

	if got := string(ed.buf); got != "hello cat" {
		t.Fatalf("expected 'hello cat', got %q", got)
	}
	if ed.sel.active {
		t.Fatalf("expected selection cleared after replacement insert")
	}
	if ed.caret != 9 { // "hello " (6) + "cat" (3)
		t.Fatalf("expected caret=9, got %d", ed.caret)
	}
}

func TestLeapAgain_UsesLastCommit_NextMatch_Forward_WithWrap(t *testing.T) {
	ed := newEd("x aa x aa x aa", 0)

	// Commit last pattern "aa" and land on first match at 2
	ed.leapStart(DirFwd)
	ed.leapAppend("aa")
	if ed.caret != 2 {
		t.Fatalf("expected caret=2 after leap, got %d", ed.caret)
	}
	ed.leapEndCommit()

	// Leap again forward should go to next match (7)
	ed.leapAgain(DirFwd)
	if ed.caret != 7 {
		t.Fatalf("expected caret=7 after leapAgain forward, got %d", ed.caret)
	}

	// Leap again forward should go to next match (12)
	ed.leapAgain(DirFwd)
	if ed.caret != 12 {
		t.Fatalf("expected caret=12 after second leapAgain forward, got %d", ed.caret)
	}

	// Leap again forward should wrap to first match (2)
	ed.leapAgain(DirFwd)
	if ed.caret != 2 {
		t.Fatalf("expected caret=2 after wrap, got %d", ed.caret)
	}
}

func TestLeapAgain_UsesLastCommit_PrevMatch_Backward_WithWrap(t *testing.T) {
	ed := newEd("x aa x aa x aa", 12)

	// Set lastCommit manually to avoid relying on previous leap
	ed.leap.lastCommit = []rune("aa")

	// From caret=12, leapAgain backward should go to previous match at 7
	ed.leapAgain(DirBack)
	if ed.caret != 7 {
		t.Fatalf("expected caret=7 after leapAgain backward, got %d", ed.caret)
	}

	// Next prev should go to 2
	ed.leapAgain(DirBack)
	if ed.caret != 2 {
		t.Fatalf("expected caret=2 after second leapAgain backward, got %d", ed.caret)
	}

	// Next prev should wrap to last (12)
	ed.leapAgain(DirBack)
	if ed.caret != 12 {
		t.Fatalf("expected caret=12 after wrap backward, got %d", ed.caret)
	}
}

func TestSelecting_UpdatesSelectionOnLeapSearch(t *testing.T) {
	ed := newEd("aa hello bb hello cc", 0)

	ed.leapStart(DirFwd)
	// simulate both cmds pressed causing selection mode
	ed.leap.selecting = true
	ed.leap.selAnchor = 0

	ed.leapAppend("hello")

	if !ed.sel.active {
		t.Fatalf("expected selection active")
	}
	a, b := ed.sel.normalised()
	if a != 0 || b != ed.caret {
		t.Fatalf("expected selection from anchor(0) to caret(%d), got (%d,%d)", ed.caret, a, b)
	}
	if ed.caret != 3 {
		t.Fatalf("expected caret=3, got %d", ed.caret)
	}
}
