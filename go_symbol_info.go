package main

import (
	"strings"

	"gc/editor"
)

var goKeywordInfo = map[string]string{
	"break":       "exit innermost for/switch/select",
	"case":        "branch label in switch/select",
	"chan":        "channel type operator",
	"const":       "declare immutable values",
	"continue":    "next loop iteration",
	"default":     "default branch in switch/select",
	"defer":       "schedule call for function return",
	"else":        "alternate branch for if",
	"fallthrough": "continue to next switch case",
	"for":         "loop construct",
	"func":        "declare function or method",
	"go":          "start goroutine",
	"goto":        "jump to label",
	"if":          "conditional branch",
	"import":      "import package",
	"interface":   "declare interface type",
	"map":         "map type constructor",
	"package":     "declare package name",
	"range":       "iterate over collection/channel",
	"return":      "return from function",
	"select":      "wait on channel operations",
	"struct":      "declare struct type",
	"switch":      "multi-branch conditional",
	"type":        "declare named type or alias",
	"var":         "declare mutable variables",
}

var goBuiltinInfo = map[string]string{
	"append":  "append elements to slice",
	"cap":     "capacity of array/slice/channel",
	"clear":   "clear map or zero slice elements",
	"close":   "close channel",
	"complex": "build complex number",
	"copy":    "copy slice elements",
	"delete":  "delete map key",
	"imag":    "imaginary part of complex",
	"len":     "length of value",
	"make":    "allocate slice/map/channel",
	"max":     "maximum of ordered values",
	"min":     "minimum of ordered values",
	"new":     "allocate zero value of type",
	"panic":   "raise runtime panic",
	"print":   "print to stderr (debug)",
	"println": "print line to stderr (debug)",
	"real":    "real part of complex",
	"recover": "recover from panic in deferred call",
}

func showSymbolInfo(app *appState) string {
	if app == nil || app.ed == nil {
		return "No symbol info"
	}
	if detectSyntax(app.currentPath, string(app.ed.Buf)) != syntaxGo {
		return "Symbol info: Go mode only"
	}
	sym := symbolUnderCaret(app.ed.Buf, app.ed.Caret)
	if sym == "" {
		return "No symbol under cursor"
	}
	if info, ok := goKeywordInfo[sym]; ok {
		return "Go keyword " + sym + ": " + info
	}
	if info, ok := goBuiltinInfo[sym]; ok {
		return "Go builtin " + sym + ": " + info
	}
	if !app.noGopls {
		lines := editor.SplitLines(app.ed.Buf)
		line := editor.CaretLineAt(lines, app.ed.Caret)
		col := editor.CaretColAt(lines, app.ed.Caret)
		if hover, err := app.gopls.hover(app.currentPath, string(app.ed.Buf), line, col); err == nil && strings.TrimSpace(hover) != "" {
			return "Go symbol " + sym + ": " + singleLine(hover)
		}
	}
	return "No info for symbol: " + sym
}

func singleLine(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if len(s) > 180 {
		return s[:180] + "..."
	}
	return s
}

func symbolUnderCaret(buf []rune, caret int) string {
	if len(buf) == 0 {
		return ""
	}
	if caret < 0 {
		caret = 0
	}
	if caret > len(buf) {
		caret = len(buf)
	}
	pos := caret
	if pos > 0 && (pos == len(buf) || !isIdentRune(buf[pos])) && isIdentRune(buf[pos-1]) {
		pos--
	}
	if pos < 0 || pos >= len(buf) || !isIdentRune(buf[pos]) {
		return ""
	}
	a := pos
	for a > 0 && isIdentRune(buf[a-1]) {
		a--
	}
	b := pos + 1
	for b < len(buf) && isIdentRune(buf[b]) {
		b++
	}
	return string(buf[a:b])
}

func isIdentRune(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_'
}
