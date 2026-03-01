package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path"
	"regexp"
	"strconv"
	"strings"

	"gc/editor"
)

var (
	mdLinkRE       = regexp.MustCompile(`\[(.*?)\]\((.*?)\)`)
	mdInlineCodeRE = regexp.MustCompile("`([^`]+)`")
	mdBoldRE       = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	mdItalicRE     = regexp.MustCompile(`\*([^*]+)\*`)
	mdStrongRE     = regexp.MustCompile(`__([^_]+)__`)
	mdEmRE         = regexp.MustCompile(`_([^_]+)_`)
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
	buf := app.ed.Runes()
	if bufferSyntaxKind(app, app.currentPath, buf) != syntaxGo {
		return "Symbol info: Go mode only"
	}
	sym := symbolUnderCaret(buf, app.ed.Caret)
	src := string(buf)
	ctx := analyzeGoCaretContext(src, app.ed.Caret)
	if sym == "" && strings.TrimSpace(ctx.message) != "" {
		return ctx.message
	}
	if sym == "" {
		return "No symbol under cursor"
	}
	local, hasLocal := findLocalDefinitionFromSource(src, sym, app.ed.Caret)
	if !hasLocal {
		local, hasLocal = findLocalDefinition(buf, sym)
	}
	if info, ok := goKeywordInfo[sym]; ok {
		out := "Go keyword: " + sym + "\n" + info
		if usage, ok := goKeywordUsage[sym]; ok {
			out += "\n\nUsage:\n" + usage
		}
		if hasLocal {
			out += "\n\n" + local
		}
		return out
	}
	if info, ok := goBuiltinInfo[sym]; ok {
		out := "Go builtin: " + sym + "\n" + info
		if usage, ok := goBuiltinUsage[sym]; ok {
			out += "\n\nUsage:\n" + usage
		}
		if hasLocal {
			out += "\n\n" + local
		}
		return out
	}
	hover := ""
	if !app.noGopls {
		lines := editor.SplitLines(buf)
		line := editor.CaretLineAt(lines, app.ed.Caret)
		col := editor.CaretColAt(lines, app.ed.Caret)
		if h, err := app.gopls.hover(app.currentPath, string(buf), line, col); err == nil && strings.TrimSpace(h) != "" {
			hover = "Go symbol: " + sym + "\n\nHover:\n" + formatHoverMarkdown(h)
		}
	}
	if strings.TrimSpace(ctx.message) != "" {
		out := ctx.message
		if hasLocal {
			out += "\n\n" + local
		}
		if hover != "" {
			out += "\n\n" + hover
		}
		return out
	}
	if hasLocal && hover != "" {
		return local + "\n\n" + hover
	}
	if hasLocal {
		return local
	}
	if hover != "" {
		return hover
	}
	return "No info for symbol: " + sym
}

func normalizeInfoText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t")
	}
	return strings.Join(lines, "\n")
}

func formatHoverMarkdown(s string) string {
	s = normalizeInfoText(s)
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	inCode := false
	for _, raw := range lines {
		trimmed := strings.TrimSpace(raw)
		if strings.HasPrefix(trimmed, "```") {
			if inCode {
				inCode = false
				if len(out) == 0 || out[len(out)-1] != "" {
					out = append(out, "")
				}
				continue
			}
			inCode = true
			lang := strings.TrimSpace(strings.TrimPrefix(trimmed, "```"))
			if lang != "" {
				out = append(out, "Code ("+lang+"):")
			} else {
				out = append(out, "Code:")
			}
			continue
		}
		if inCode {
			out = append(out, "    "+strings.TrimRight(raw, " \t"))
			continue
		}
		if trimmed == "" {
			if len(out) == 0 || out[len(out)-1] == "" {
				continue
			}
			out = append(out, "")
			continue
		}
		if title, ok := parseMarkdownHeading(trimmed); ok {
			title = renderInlineMarkdown(title)
			out = append(out, strings.ToUpper(title))
			out = append(out, strings.Repeat("─", min(len(title), 48)))
			continue
		}
		if body, ok := strings.CutPrefix(trimmed, ">"); ok {
			out = append(out, "│ "+renderInlineMarkdown(strings.TrimSpace(body)))
			continue
		}
		if body, ok := parseMarkdownBullet(trimmed); ok {
			out = append(out, "• "+renderInlineMarkdown(body))
			continue
		}
		out = append(out, renderInlineMarkdown(trimmed))
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func parseMarkdownHeading(line string) (string, bool) {
	i := 0
	for i < len(line) && line[i] == '#' {
		i++
	}
	if i == 0 || i >= len(line) || line[i] != ' ' {
		return "", false
	}
	return strings.TrimSpace(line[i+1:]), true
}

func parseMarkdownBullet(line string) (string, bool) {
	if after, ok := strings.CutPrefix(line, "- "); ok {
		return strings.TrimSpace(after), true
	}
	if after, ok := strings.CutPrefix(line, "* "); ok {
		return strings.TrimSpace(after), true
	}
	if after, ok := strings.CutPrefix(line, "+ "); ok {
		return strings.TrimSpace(after), true
	}
	for i := 0; i < len(line); i++ {
		if line[i] < '0' || line[i] > '9' {
			if i > 0 && i+1 < len(line) && line[i] == '.' && line[i+1] == ' ' {
				return strings.TrimSpace(line[i+2:]), true
			}
			break
		}
	}
	return "", false
}

func renderInlineMarkdown(s string) string {
	s = mdLinkRE.ReplaceAllString(s, `$1 ($2)`)
	s = mdInlineCodeRE.ReplaceAllString(s, `"$1"`)
	s = mdBoldRE.ReplaceAllString(s, `$1`)
	s = mdItalicRE.ReplaceAllString(s, `$1`)
	s = mdStrongRE.ReplaceAllString(s, `$1`)
	s = mdEmRE.ReplaceAllString(s, `$1`)
	return strings.TrimSpace(s)
}

type goCaretContext struct {
	message string
}

func analyzeGoCaretContext(src string, caret int) goCaretContext {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", src, parser.ParseComments)
	if err != nil || file == nil {
		return goCaretContext{}
	}
	tf := fset.File(file.Pos())
	if tf == nil {
		return goCaretContext{}
	}
	off := runeOffsetToByteOffset(src, caret)
	if off < 0 {
		off = 0
	}
	if off > tf.Size() {
		off = tf.Size()
	}
	pos := tf.Pos(off)

	if file.Name != nil && containsPos(file.Name, pos) {
		name := strings.TrimSpace(file.Name.Name)
		if name != "" {
			return goCaretContext{message: fmt.Sprintf("Package declaration: %s\nUsage: package %s", name, name)}
		}
	}

	importNames := goImportNames(file)
	for _, imp := range file.Imports {
		pkgPath := goImportPath(imp.Path)
		pkgName := goImportName(imp, pkgPath)
		if imp.Name != nil && containsPos(imp.Name, pos) {
			if pkgName == "." {
				return goCaretContext{message: fmt.Sprintf("Dot import: %q\nUsage: import . %q", pkgPath, pkgPath)}
			}
			if pkgName == "_" {
				return goCaretContext{message: fmt.Sprintf("Blank import: %q\nUsage: import _ %q", pkgPath, pkgPath)}
			}
			return goCaretContext{message: fmt.Sprintf("Import alias %s for package %s (%q)\nUsage: %s.<symbol>", pkgName, path.Base(pkgPath), pkgPath, pkgName)}
		}
		if imp.Path != nil && containsPos(imp.Path, pos) {
			return goCaretContext{message: fmt.Sprintf("Imported package %s (%q)\nUsage: import %q", pkgName, pkgPath, pkgPath)}
		}
	}

	if se := selectorExprAtPos(file, pos); se != nil {
		pkgIdent, _ := se.X.(*ast.Ident)
		if pkgIdent != nil {
			pkgPath, ok := importNames[pkgIdent.Name]
			if ok {
				member := se.Sel.Name
				if member == "" {
					member = "<member>"
				}
				if containsPos(se.Sel, pos) {
					return goCaretContext{
						message: fmt.Sprintf(
							"Package member: %s.%s\nImported package: %s (%q)\nUsage: %s.%s(...)",
							pkgIdent.Name,
							member,
							path.Base(pkgPath),
							pkgPath,
							pkgIdent.Name,
							member,
						),
					}
				}
				return goCaretContext{
					message: fmt.Sprintf(
						"Imported package alias: %s (%q)\nCurrent selector: %s.%s",
						pkgIdent.Name,
						pkgPath,
						pkgIdent.Name,
						member,
					),
				}
			}
		}
	}

	return goCaretContext{}
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

func findLocalDefinitionFromSource(src, sym string, caret int) (string, bool) {
	if strings.TrimSpace(sym) == "" {
		return "", false
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", src, parser.ParseComments)
	if err != nil || file == nil {
		return "", false
	}
	tf := fset.File(file.Pos())
	if tf == nil {
		return "", false
	}
	off := runeOffsetToByteOffset(src, caret)
	if off < 0 {
		off = 0
	}
	if off > tf.Size() {
		off = tf.Size()
	}
	caretPos := tf.Pos(off)
	lines := strings.Split(src, "\n")
	type candidate struct {
		pos  token.Pos
		kind string
		line int
	}
	best := candidate{}
	add := func(pos token.Pos, kind string) {
		if pos == token.NoPos {
			return
		}
		line := fset.Position(pos).Line
		c := candidate{pos: pos, kind: kind, line: line}
		if best.pos == token.NoPos {
			best = c
			return
		}
		if pos <= caretPos && (best.pos > caretPos || pos > best.pos) {
			best = c
			return
		}
		if best.pos > caretPos && pos > caretPos && pos < best.pos {
			best = c
		}
	}
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncDecl:
			if node.Name != nil && node.Name.Name == sym {
				kind := "function"
				if node.Recv != nil {
					kind = "method"
				}
				add(node.Name.Pos(), kind)
			}
			if node.Type != nil && node.Type.Params != nil {
				for _, f := range node.Type.Params.List {
					for _, name := range f.Names {
						if name.Name == sym {
							add(name.Pos(), "param")
						}
					}
				}
			}
		case *ast.TypeSpec:
			if node.Name != nil && node.Name.Name == sym {
				add(node.Name.Pos(), "type")
			}
		case *ast.ValueSpec:
			for _, name := range node.Names {
				if name.Name == sym {
					add(name.Pos(), "var")
				}
			}
		case *ast.AssignStmt:
			if node.Tok == token.DEFINE {
				for _, lhs := range node.Lhs {
					if id, ok := lhs.(*ast.Ident); ok && id.Name == sym {
						add(id.Pos(), "var")
					}
				}
			}
		case *ast.RangeStmt:
			if node.Tok == token.DEFINE {
				if id, ok := node.Key.(*ast.Ident); ok && id.Name == sym {
					add(id.Pos(), "var")
				}
				if id, ok := node.Value.(*ast.Ident); ok && id.Name == sym {
					add(id.Pos(), "var")
				}
			}
		}
		return true
	})
	if best.pos == token.NoPos {
		return "", false
	}
	if best.line <= 0 || best.line > len(lines) {
		return "", false
	}
	snippet := strings.TrimSpace(lines[best.line-1])
	return fmt.Sprintf("Local definition (line %d, %s): %s", best.line, best.kind, singleLine(snippet)), true
}

func runeOffsetToByteOffset(src string, runeOffset int) int {
	if runeOffset <= 0 {
		return 0
	}
	ri := 0
	for bi := range src {
		if ri == runeOffset {
			return bi
		}
		ri++
	}
	return len(src)
}

func goImportPath(pathLit *ast.BasicLit) string {
	if pathLit == nil {
		return ""
	}
	v, err := strconv.Unquote(pathLit.Value)
	if err != nil {
		return strings.Trim(pathLit.Value, "\"")
	}
	return v
}

func goImportName(imp *ast.ImportSpec, pkgPath string) string {
	if imp != nil && imp.Name != nil && strings.TrimSpace(imp.Name.Name) != "" {
		return imp.Name.Name
	}
	if pkgPath == "" {
		return ""
	}
	return path.Base(pkgPath)
}

func goImportNames(file *ast.File) map[string]string {
	out := make(map[string]string, len(file.Imports))
	for _, imp := range file.Imports {
		pkgPath := goImportPath(imp.Path)
		name := goImportName(imp, pkgPath)
		if name == "" || name == "_" || name == "." {
			continue
		}
		out[name] = pkgPath
	}
	return out
}

func containsPos(n ast.Node, pos token.Pos) bool {
	if n == nil || pos == token.NoPos {
		return false
	}
	return n.Pos() <= pos && pos <= n.End()
}

func selectorExprAtPos(file *ast.File, pos token.Pos) *ast.SelectorExpr {
	var best *ast.SelectorExpr
	ast.Inspect(file, func(n ast.Node) bool {
		se, ok := n.(*ast.SelectorExpr)
		if !ok || !containsPos(se, pos) {
			return true
		}
		if best == nil || (se.End()-se.Pos()) < (best.End()-best.Pos()) {
			best = se
		}
		return true
	})
	return best
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
