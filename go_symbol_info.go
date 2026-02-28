package main

import (
	"fmt"
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

var goKeywordUsage = map[string]string{
	"break":     "for { if done { break } }",
	"case":      "switch x { case 1: }",
	"chan":      "var ch chan int",
	"const":     "const Pi = 3.14",
	"continue":  "for i := range xs { if skip { continue } }",
	"default":   "switch x { default: }",
	"defer":     "defer f.Close()",
	"else":      "if ok { ... } else { ... }",
	"for":       "for i := 0; i < n; i++ { }",
	"func":      "func add(a, b int) int { return a + b }",
	"go":        "go worker(ch)",
	"if":        "if err != nil { return err }",
	"import":    "import \"fmt\"",
	"interface": "type R interface { Read([]byte) (int, error) }",
	"map":       "m := map[string]int{}",
	"package":   "package main",
	"range":     "for _, v := range xs { }",
	"return":    "return value, nil",
	"select":    "select { case v := <-ch: _ = v }",
	"struct":    "type S struct { Name string }",
	"switch":    "switch x { case 1: }",
	"type":      "type ID string",
	"var":       "var n int",
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

var goBuiltinUsage = map[string]string{
	"append":  "s = append(s, v)",
	"cap":     "n := cap(s)",
	"clear":   "clear(m)",
	"close":   "close(ch)",
	"complex": "z := complex(1, 2)",
	"copy":    "n := copy(dst, src)",
	"delete":  "delete(m, k)",
	"imag":    "y := imag(z)",
	"len":     "n := len(v)",
	"make":    "ch := make(chan int, 1)",
	"max":     "m := max(a, b)",
	"min":     "m := min(a, b)",
	"new":     "p := new(T)",
	"panic":   "panic(\"bad state\")",
	"print":   "print(v)",
	"println": "println(v)",
	"real":    "x := real(z)",
	"recover": "if r := recover(); r != nil { }",
}

func showSymbolInfo(app *appState) string {
	if app == nil || app.ed == nil {
		return "No symbol info"
	}
	if bufferSyntaxKind(app, app.currentPath, app.ed.Buf) != syntaxGo {
		return "Symbol info: Go mode only"
	}
	sym := symbolUnderCaret(app.ed.Buf, app.ed.Caret)
	if sym == "" {
		return "No symbol under cursor"
	}
	local, hasLocal := findLocalDefinition(app.ed.Buf, sym)
	if info, ok := goKeywordInfo[sym]; ok {
		out := "Go keyword " + sym + ": " + info
		if usage, ok := goKeywordUsage[sym]; ok {
			out += "\nUsage: " + usage
		}
		if hasLocal {
			out += "\n" + local
		}
		return out
	}
	if info, ok := goBuiltinInfo[sym]; ok {
		out := "Go builtin " + sym + ": " + info
		if usage, ok := goBuiltinUsage[sym]; ok {
			out += "\nUsage: " + usage
		}
		if hasLocal {
			out += "\n" + local
		}
		return out
	}
	hover := ""
	if !app.noGopls {
		lines := editor.SplitLines(app.ed.Buf)
		line := editor.CaretLineAt(lines, app.ed.Caret)
		col := editor.CaretColAt(lines, app.ed.Caret)
		if h, err := app.gopls.hover(app.currentPath, string(app.ed.Buf), line, col); err == nil && strings.TrimSpace(h) != "" {
			hover = "Go symbol " + sym + ": " + singleLine(h)
		}
	}
	if hasLocal && hover != "" {
		return local + "\n" + hover
	}
	if hasLocal {
		return local
	}
	if hover != "" {
		return hover
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

func findLocalDefinition(buf []rune, sym string) (string, bool) {
	if strings.TrimSpace(sym) == "" {
		return "", false
	}
	lines := editor.SplitLines(buf)
	for i, line := range lines {
		if kind, ok := lineDefinesSymbol(line, sym); ok {
			return fmt.Sprintf("Local definition (line %d, %s): %s", i+1, kind, singleLine(strings.TrimSpace(line))), true
		}
	}
	return "", false
}

func lineDefinesSymbol(line, sym string) (string, bool) {
	t := strings.TrimSpace(line)
	if t == "" || strings.HasPrefix(t, "//") {
		return "", false
	}
	if after, ok := strings.CutPrefix(t, "func "); ok {
		after := strings.TrimSpace(after)
		if strings.HasPrefix(after, sym+"(") {
			return "function", true
		}
		if strings.HasPrefix(after, "(") {
			if _, after0, ok := strings.Cut(after, ")"); ok {
				rest := strings.TrimSpace(after0)
				if strings.HasPrefix(rest, sym+"(") {
					return "method", true
				}
			}
		}
	}
	if after, ok := strings.CutPrefix(t, "type "); ok {
		rest := strings.TrimSpace(after)
		if strings.HasPrefix(rest, sym+" ") || strings.HasPrefix(rest, sym+"=") {
			return "type", true
		}
	}
	if after, ok := strings.CutPrefix(t, "var "); ok {
		rest := strings.TrimSpace(after)
		if strings.HasPrefix(rest, sym+" ") || strings.HasPrefix(rest, sym+"=") || strings.HasPrefix(rest, sym+":=") {
			return "var", true
		}
	}
	if after, ok := strings.CutPrefix(t, "const "); ok {
		rest := strings.TrimSpace(after)
		if strings.HasPrefix(rest, sym+" ") || strings.HasPrefix(rest, sym+"=") {
			return "const", true
		}
	}
	return "", false
}
