package main

import (
	"testing"

	"gc/editor"

	"github.com/veandco/go-sdl2/sdl"
)

func TestSDLToKeyMapping(t *testing.T) {
	tests := []struct {
		in   sdl.Keycode
		want keyCode
	}{
		{in: sdl.K_TAB, want: keyTab},
		{in: sdl.K_LGUI, want: keyLcmd},
		{in: sdl.K_RGUI, want: keyRcmd},
		{in: sdl.K_ESCAPE, want: keyEscape},
		{in: sdl.K_RETURN, want: keyReturn},
		{in: sdl.K_SLASH, want: keySlash},
	}
	for _, tc := range tests {
		if got := sdlToKey(tc.in); got != tc.want {
			t.Fatalf("sdlToKey(%v)=%v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestHandleKeyEventCtrlBAddsBufferWithoutSDLDispatch(t *testing.T) {
	app := appState{}
	app.initBuffers(editor.NewEditor("abc"))
	if !handleKeyEvent(&app, keyEvent{down: true, repeat: 0, key: keyB, mods: modCtrl}) {
		t.Fatalf("handleKeyEvent should continue running")
	}
	if len(app.buffers) != 2 {
		t.Fatalf("expected buffer count 2, got %d", len(app.buffers))
	}
}

func TestHandleTextEventInsertsTextWithoutSDLDispatch(t *testing.T) {
	app := appState{}
	app.initBuffers(editor.NewEditor("ab"))
	app.ed.Caret = len(app.ed.Buf)
	if !handleTextEvent(&app, "c", 0) {
		t.Fatalf("handleTextEvent should continue running")
	}
	if got := string(app.ed.Buf); got != "abc" {
		t.Fatalf("text insert mismatch: got %q", got)
	}
}

func BenchmarkHandleKeyEventMoveRight(b *testing.B) {
	app := appState{}
	app.initBuffers(editor.NewEditor("package main\nfunc main() {}\n"))
	ev := keyEvent{down: true, key: keyRight, mods: 0}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = handleKeyEvent(&app, ev)
	}
}
