package main

import (
	"context"
	"sort"
	"strings"
	"unicode/utf8"

	sitter "github.com/smacker/go-tree-sitter"
	sitterc "github.com/smacker/go-tree-sitter/c"
	sittergo "github.com/smacker/go-tree-sitter/golang"
	sittermd "github.com/smacker/go-tree-sitter/markdown/tree-sitter-markdown"
	sitterhs "github.com/tree-sitter/tree-sitter-haskell/bindings/go"
)

type tokenStyle int

const (
	styleDefault tokenStyle = iota
	styleKeyword
	styleType
	styleFunction
	styleString
	styleNumber
	styleComment
	styleHeading
	styleLink
	stylePunctuation
)

type syntaxKind int

const (
	syntaxNone syntaxKind = iota
	syntaxGo
	syntaxMarkdown
	syntaxC
	syntaxMiranda
)

type syntaxHighlighter struct {
	lastPath   string
	lastSource string
	lastLines  int
	lastKind   syntaxKind
	lineStyles map[int][]tokenStyle
}

func newGoHighlighter() *syntaxHighlighter {
	return &syntaxHighlighter{}
}

func (h *syntaxHighlighter) lineStyleFor(path string, buf []rune, lines []string) map[int][]tokenStyle {
	if h == nil {
		return nil
	}
	src := string(buf)
	kind := detectSyntax(path, src)
	if kind == syntaxNone {
		h.lastPath = path
		h.lastSource = src
		h.lastLines = len(lines)
		h.lastKind = kind
		h.lineStyles = nil
		return nil
	}
	if h.lastPath == path && h.lastSource == src && h.lastLines == len(lines) && h.lastKind == kind {
		return h.lineStyles
	}

	var lineStyles map[int][]tokenStyle
	switch kind {
	case syntaxGo:
		lineStyles = buildLineStyles(src, lines, sittergo.GetLanguage(), classifyGoNode)
	case syntaxMarkdown:
		lineStyles = buildLineStyles(src, lines, sittermd.GetLanguage(), classifyMarkdownNode)
	case syntaxC:
		lineStyles = buildLineStyles(src, lines, sitterc.GetLanguage(), classifyCNode)
	case syntaxMiranda:
		lineStyles = buildLineStyles(src, lines, sitter.NewLanguage(sitterhs.Language()), classifyMirandaNode)
	}

	h.lastPath = path
	h.lastSource = src
	h.lastLines = len(lines)
	h.lastKind = kind
	h.lineStyles = lineStyles
	return lineStyles
}

func detectSyntax(path, src string) syntaxKind {
	pathLower := strings.ToLower(path)
	switch {
	case strings.HasSuffix(pathLower, ".go"):
		return syntaxGo
	case strings.HasSuffix(pathLower, ".md"), strings.HasSuffix(pathLower, ".markdown"):
		return syntaxMarkdown
	case strings.HasSuffix(pathLower, ".c"), strings.HasSuffix(pathLower, ".h"):
		return syntaxC
	case strings.HasSuffix(pathLower, ".m"):
		return syntaxMiranda
	}

	for line := range strings.SplitSeq(src, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "package ") {
			return syntaxGo
		}
		if strings.HasPrefix(trimmed, "# ") || strings.HasPrefix(trimmed, "## ") {
			return syntaxMarkdown
		}
		return syntaxNone
	}
	return syntaxNone
}

type spanPriority struct {
	style    tokenStyle
	priority int
}

type styleClassifier func(*sitter.Node, string) (tokenStyle, int)

func buildLineStyles(src string, lines []string, lang *sitter.Language, classify styleClassifier) map[int][]tokenStyle {
	root, err := sitter.ParseCtx(context.Background(), []byte(src), lang)
	if err != nil || root == nil {
		return nil
	}

	styleGrid := make([][]spanPriority, len(lines))
	for i, line := range lines {
		styleGrid[i] = make([]spanPriority, utf8.RuneCountInString(line))
	}

	lineStartBytes := computeLineStartBytes(src, len(lines))
	walkTreeLeaves(root, func(n *sitter.Node) {
		style, pri := classify(n, src)
		if style == styleDefault {
			return
		}
		applyNodeStyle(styleGrid, lines, lineStartBytes, n.StartByte(), n.EndByte(), style, pri)
	})

	out := make(map[int][]tokenStyle, len(lines))
	for i, row := range styleGrid {
		hasStyle := false
		styles := make([]tokenStyle, len(row))
		for j, cell := range row {
			styles[j] = cell.style
			if cell.style != styleDefault {
				hasStyle = true
			}
		}
		if hasStyle {
			out[i] = styles
		}
	}
	return out
}

func walkTreeLeaves(node *sitter.Node, visit func(*sitter.Node)) {
	if node == nil {
		return
	}
	if node.ChildCount() == 0 {
		visit(node)
		return
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		walkTreeLeaves(node.Child(i), visit)
	}
}

var goKeywordTokens = map[string]struct{}{
	"break":       {},
	"case":        {},
	"chan":        {},
	"const":       {},
	"continue":    {},
	"default":     {},
	"defer":       {},
	"else":        {},
	"fallthrough": {},
	"for":         {},
	"func":        {},
	"go":          {},
	"goto":        {},
	"if":          {},
	"import":      {},
	"interface":   {},
	"map":         {},
	"package":     {},
	"range":       {},
	"return":      {},
	"select":      {},
	"struct":      {},
	"switch":      {},
	"type":        {},
	"var":         {},
}

var goPseudoKeywords = map[string]struct{}{
	"nil":   {},
	"true":  {},
	"false": {},
	"iota":  {},
}

func classifyGoNode(node *sitter.Node, src string) (tokenStyle, int) {
	if node == nil {
		return styleDefault, 0
	}
	typ := node.Type()
	switch typ {
	case "comment":
		return styleComment, 90
	case "interpreted_string_literal", "raw_string_literal", "rune_literal":
		return styleString, 80
	case "int_literal", "float_literal", "imaginary_literal":
		return styleNumber, 70
	case "type_identifier":
		return styleType, 60
	case "field_identifier":
		if p := node.Parent(); p != nil && p.Type() == "method_declaration" {
			return styleFunction, 60
		}
		return styleDefault, 0
	case "identifier":
		text := nodeText(src, node)
		if _, ok := goPseudoKeywords[text]; ok {
			return styleKeyword, 60
		}
		if p := node.Parent(); p != nil {
			switch p.Type() {
			case "function_declaration", "call_expression":
				return styleFunction, 50
			case "type_spec":
				return styleType, 55
			}
		}
		return styleDefault, 0
	}
	if !node.IsNamed() {
		if _, ok := goKeywordTokens[typ]; ok {
			return styleKeyword, 60
		}
	}
	return styleDefault, 0
}

func classifyMarkdownNode(node *sitter.Node, _ string) (tokenStyle, int) {
	if node == nil {
		return styleDefault, 0
	}
	switch node.Type() {
	case "atx_heading", "setext_heading", "atx_h1_marker", "atx_h2_marker", "atx_h3_marker", "atx_h4_marker", "atx_h5_marker", "atx_h6_marker", "setext_h1_underline", "setext_h2_underline":
		return styleHeading, 70
	case "fenced_code_block", "code_fence_content", "fenced_code_block_delimiter", "indented_code_block", "info_string", "language":
		return styleString, 80
	case "link_label", "link_destination", "link_title", "link_reference_definition":
		return styleLink, 70
	case "thematic_break", "block_quote_marker", "list_marker_plus", "list_marker_minus", "list_marker_star", "list_marker_dot", "list_marker_parenthesis", "task_list_marker_checked", "task_list_marker_unchecked", "pipe_table_delimiter_row", "pipe_table_delimiter_cell":
		return stylePunctuation, 60
	case "html_block":
		return styleComment, 50
	default:
		return styleDefault, 0
	}
}

var cKeywordTokens = map[string]struct{}{
	"break":          {},
	"case":           {},
	"const":          {},
	"continue":       {},
	"default":        {},
	"do":             {},
	"else":           {},
	"enum":           {},
	"extern":         {},
	"for":            {},
	"goto":           {},
	"if":             {},
	"inline":         {},
	"register":       {},
	"restrict":       {},
	"return":         {},
	"sizeof":         {},
	"static":         {},
	"struct":         {},
	"switch":         {},
	"typedef":        {},
	"union":          {},
	"volatile":       {},
	"while":          {},
	"_Alignas":       {},
	"_Alignof":       {},
	"_Atomic":        {},
	"_Bool":          {},
	"_Complex":       {},
	"_Generic":       {},
	"_Imaginary":     {},
	"_Noreturn":      {},
	"_Static_assert": {},
	"_Thread_local":  {},
}

func classifyCNode(node *sitter.Node, _ string) (tokenStyle, int) {
	if node == nil {
		return styleDefault, 0
	}
	switch node.Type() {
	case "comment":
		return styleComment, 90
	case "string_literal", "char_literal":
		return styleString, 80
	case "number_literal":
		return styleNumber, 70
	case "type_identifier", "primitive_type", "sized_type_specifier", "macro_type_specifier":
		return styleType, 65
	case "preproc_include", "preproc_def", "preproc_function_def", "preproc_if", "preproc_ifdef", "preproc_else", "preproc_elif", "preproc_elifdef", "preproc_directive", "#include", "#define", "#if", "#ifdef", "#ifndef", "#else", "#elif", "#elifdef", "#elifndef":
		return styleKeyword, 75
	}
	if !node.IsNamed() {
		if _, ok := cKeywordTokens[node.Type()]; ok {
			return styleKeyword, 60
		}
	}
	return styleDefault, 0
}

var mirandaKeywordTokens = map[string]struct{}{
	"let":      {},
	"in":       {},
	"if":       {},
	"then":     {},
	"else":     {},
	"case":     {},
	"of":       {},
	"where":    {},
	"module":   {},
	"import":   {},
	"type":     {},
	"data":     {},
	"newtype":  {},
	"class":    {},
	"instance": {},
}

func classifyMirandaNode(node *sitter.Node, _ string) (tokenStyle, int) {
	if node == nil {
		return styleDefault, 0
	}
	switch node.Type() {
	case "comment":
		return styleComment, 90
	case "string", "char":
		return styleString, 80
	case "integer", "float":
		return styleNumber, 70
	case "type":
		return styleType, 65
	case "module", "import", "newtype", "class", "instance":
		return styleKeyword, 70
	}
	if !node.IsNamed() {
		if _, ok := mirandaKeywordTokens[node.Type()]; ok {
			return styleKeyword, 60
		}
	}
	return styleDefault, 0
}

func nodeText(src string, node *sitter.Node) string {
	if node == nil {
		return ""
	}
	a := int(node.StartByte())
	b := int(node.EndByte())
	if a < 0 {
		a = 0
	}
	if b > len(src) {
		b = len(src)
	}
	if a >= b {
		return ""
	}
	return src[a:b]
}

func computeLineStartBytes(src string, lineCount int) []int {
	starts := make([]int, 0, lineCount)
	starts = append(starts, 0)
	for i := 0; i < len(src); i++ {
		if src[i] == '\n' {
			starts = append(starts, i+1)
		}
	}
	if len(starts) > lineCount {
		return starts[:lineCount]
	}
	for len(starts) < lineCount {
		starts = append(starts, len(src))
	}
	return starts
}

func applyNodeStyle(
	styleGrid [][]spanPriority,
	lines []string,
	lineStarts []int,
	startByte uint32,
	endByte uint32,
	style tokenStyle,
	priority int,
) {
	if len(styleGrid) == 0 || int(endByte) <= int(startByte) {
		return
	}
	startLine, startColByte := byteOffsetToLineCol(lineStarts, int(startByte))
	endLine, endColByte := byteOffsetToLineCol(lineStarts, int(endByte))
	if startLine >= len(styleGrid) || endLine < 0 {
		return
	}
	if startLine < 0 {
		startLine = 0
	}
	if endLine >= len(styleGrid) {
		endLine = len(styleGrid) - 1
	}
	for ln := startLine; ln <= endLine; ln++ {
		if ln < 0 || ln >= len(lines) {
			continue
		}
		line := lines[ln]
		lineBytes := len(line)
		segStartByte := 0
		segEndByte := lineBytes
		if ln == startLine {
			segStartByte = min(startColByte, lineBytes)
		}
		if ln == endLine {
			segEndByte = min(endColByte, lineBytes)
		}
		if segEndByte <= segStartByte {
			continue
		}
		startColRune := utf8.RuneCountInString(line[:segStartByte])
		endColRune := utf8.RuneCountInString(line[:segEndByte])
		row := styleGrid[ln]
		if startColRune < 0 {
			startColRune = 0
		}
		if endColRune > len(row) {
			endColRune = len(row)
		}
		for i := startColRune; i < endColRune; i++ {
			if priority >= row[i].priority {
				row[i] = spanPriority{style: style, priority: priority}
			}
		}
	}
}

func byteOffsetToLineCol(lineStarts []int, off int) (line int, col int) {
	if len(lineStarts) == 0 {
		return 0, 0
	}
	if off <= 0 {
		return 0, 0
	}
	lastStart := lineStarts[len(lineStarts)-1]
	if off >= lastStart {
		return len(lineStarts) - 1, off - lastStart
	}
	i := sort.Search(len(lineStarts), func(i int) bool {
		return lineStarts[i] > off
	})
	line = max(i-1, 0)
	col = max(off-lineStarts[line], 0)
	return line, col
}
