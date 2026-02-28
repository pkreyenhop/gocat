package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gc/editor"
)

func BenchmarkEditorInsertAtCaret(b *testing.B) {
	e := editor.NewEditor(strings.Repeat("x", 8192))
	e.Caret = e.RuneLen() / 2
	ins := "package"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.InsertText(ins)
		e.Caret -= len([]rune(ins))
		e.BackspaceOrDeleteSelection(false)
		e.BackspaceOrDeleteSelection(false)
		e.BackspaceOrDeleteSelection(false)
		e.BackspaceOrDeleteSelection(false)
		e.BackspaceOrDeleteSelection(false)
		e.BackspaceOrDeleteSelection(false)
		e.BackspaceOrDeleteSelection(false)
	}
}

func BenchmarkEditorDeleteWord(b *testing.B) {
	e := editor.NewEditor(strings.Repeat("identifier ", 4096))
	e.Caret = e.RuneLen() / 2
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !e.DeleteWordAtCaret() {
			e.Caret = e.RuneLen() / 2
		}
		if e.RuneLen() < 2048 {
			e = editor.NewEditor(strings.Repeat("identifier ", 4096))
			e.Caret = e.RuneLen() / 2
		}
	}
}

func BenchmarkOpenPathLargeFile(b *testing.B) {
	dir := b.TempDir()
	path := filepath.Join(dir, "large.go")
	var src strings.Builder
	src.Grow(2 << 20)
	for range 120000 {
		src.WriteString("var x = 12345\n")
	}
	if err := os.WriteFile(path, []byte(src.String()), 0o644); err != nil {
		b.Fatalf("write fixture: %v", err)
	}

	app := &appState{openRoot: dir}
	app.initBuffers(editor.NewEditor(""))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := openPath(app, path); err != nil {
			b.Fatalf("openPath: %v", err)
		}
	}
}
