package main

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	treesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

type spanPriority struct {
	style    tokenStyle
	priority int
}

type tsLanguageSpec struct {
	kind         syntaxKind
	lang         *treesitter.Language
	query        string
	tokenFactory func([]byte, *treesitter.Language) treesitter.TokenSource

	once        sync.Once
	highlighter *treesitter.Highlighter
	initErr     error
}

var (
	tsSpecsOnce       sync.Once
	tsSpecs           map[syntaxKind]*tsLanguageSpec
	captureStyleCache sync.Map
)

func (h *syntaxHighlighter) lineStyleForKind(path, src string, lines []string, kind syntaxKind) [][]tokenStyle {
	if h == nil {
		return nil
	}
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

	tsSpecsOnce.Do(initTreeSitterSpecs)
	spec := tsSpecs[kind]
	lineStyles := buildTreeSitterLineStyles(spec, src, lines)

	h.lastPath = path
	h.lastSource = src
	h.lastLines = len(lines)
	h.lastKind = kind
	h.lineStyles = lineStyles
	return lineStyles
}

func initTreeSitterSpecs() {
	tsSpecs = map[syntaxKind]*tsLanguageSpec{}

	goEntry := grammars.DetectLanguage("x.go")
	cEntry := grammars.DetectLanguage("x.c")
	mdEntry := grammars.DetectLanguage("x.md")
	hsEntry := grammars.DetectLanguage("x.hs")

	if goEntry != nil {
		tsSpecs[syntaxGo] = &tsLanguageSpec{
			kind:         syntaxGo,
			lang:         goEntry.Language(),
			query:        goEntry.HighlightQuery,
			tokenFactory: goEntry.TokenSourceFactory,
		}
	}
	if cEntry != nil {
		tsSpecs[syntaxC] = &tsLanguageSpec{
			kind:         syntaxC,
			lang:         cEntry.Language(),
			query:        cEntry.HighlightQuery,
			tokenFactory: cEntry.TokenSourceFactory,
		}
	}
	if mdEntry != nil {
		query := strings.TrimSpace(mdEntry.HighlightQuery)
		if query == "" {
			query = markdownFallbackQuery
		}
		tsSpecs[syntaxMarkdown] = &tsLanguageSpec{
			kind:         syntaxMarkdown,
			lang:         mdEntry.Language(),
			query:        query,
			tokenFactory: mdEntry.TokenSourceFactory,
		}
	}
	if hsEntry != nil {
		tsSpecs[syntaxMiranda] = &tsLanguageSpec{
			kind:         syntaxMiranda,
			lang:         hsEntry.Language(),
			query:        hsEntry.HighlightQuery,
			tokenFactory: hsEntry.TokenSourceFactory,
		}
	}
}

func (s *tsLanguageSpec) highlighterForKind() (*treesitter.Highlighter, error) {
	if s == nil || s.lang == nil {
		return nil, fmt.Errorf("language unavailable")
	}
	s.once.Do(func() {
		query := strings.TrimSpace(s.query)
		if query == "" {
			s.initErr = fmt.Errorf("highlight query unavailable")
			return
		}

		opts := []treesitter.HighlighterOption{}
		if s.tokenFactory != nil {
			opts = append(opts, treesitter.WithTokenSourceFactory(func(source []byte) treesitter.TokenSource {
				return s.tokenFactory(source, s.lang)
			}))
		}

		hl, err := treesitter.NewHighlighter(s.lang, query, opts...)
		if err != nil && s.kind == syntaxMarkdown {
			hl, err = treesitter.NewHighlighter(s.lang, "(_) @punctuation", opts...)
		}
		s.highlighter = hl
		s.initErr = err
	})
	return s.highlighter, s.initErr
}

func buildTreeSitterLineStyles(spec *tsLanguageSpec, src string, lines []string) [][]tokenStyle {
	if spec == nil || len(lines) == 0 {
		return nil
	}
	hl, err := spec.highlighterForKind()
	if err != nil || hl == nil {
		return nil
	}

	ranges := hl.Highlight([]byte(src))
	if len(ranges) == 0 {
		return nil
	}

	styleGrid := make([][]spanPriority, len(lines))
	for i, line := range lines {
		styleGrid[i] = make([]spanPriority, utf8.RuneCountInString(line))
	}

	lineStartBytes := computeLineStartBytes(src, len(lines))
	for _, r := range ranges {
		style, pri := styleFromCapture(r.Capture)
		if style == styleDefault {
			continue
		}
		applyByteStyle(styleGrid, lines, lineStartBytes, int(r.StartByte), int(r.EndByte), style, pri)
	}

	out := make([][]tokenStyle, len(lines))
	hasAny := false
	for i, row := range styleGrid {
		hasStyle := false
		for _, cell := range row {
			if cell.style != styleDefault {
				hasStyle = true
				hasAny = true
				break
			}
		}
		if hasStyle {
			styles := make([]tokenStyle, len(row))
			for j, cell := range row {
				styles[j] = cell.style
			}
			out[i] = styles
		}
	}
	if !hasAny {
		return nil
	}
	return out
}

func styleFromCapture(capture string) (tokenStyle, int) {
	if v, ok := captureStyleCache.Load(capture); ok {
		sp := v.(spanPriority)
		return sp.style, sp.priority
	}
	name := strings.ToLower(capture)
	sp := spanPriority{}
	switch {
	case strings.Contains(name, "comment"):
		sp = spanPriority{style: styleComment, priority: 90}
	case strings.Contains(name, "string"), strings.Contains(name, "character"), strings.Contains(name, "escape"):
		sp = spanPriority{style: styleString, priority: 80}
	case strings.Contains(name, "number"), strings.Contains(name, "float"), strings.Contains(name, "integer"):
		sp = spanPriority{style: styleNumber, priority: 70}
	case strings.Contains(name, "function"), strings.Contains(name, "method"):
		sp = spanPriority{style: styleFunction, priority: 65}
	case strings.Contains(name, "type"), strings.Contains(name, "constructor"):
		sp = spanPriority{style: styleType, priority: 60}
	case strings.Contains(name, "heading"), strings.Contains(name, "title"):
		sp = spanPriority{style: styleHeading, priority: 70}
	case strings.Contains(name, "link"), strings.Contains(name, "url"), strings.Contains(name, "uri"):
		sp = spanPriority{style: styleLink, priority: 70}
	case strings.Contains(name, "keyword"), strings.Contains(name, "conditional"), strings.Contains(name, "repeat"), strings.Contains(name, "exception"):
		sp = spanPriority{style: styleKeyword, priority: 60}
	case strings.Contains(name, "operator"), strings.Contains(name, "punctuation"), strings.Contains(name, "delimiter"), strings.Contains(name, "bracket"):
		sp = spanPriority{style: stylePunctuation, priority: 55}
	}
	captureStyleCache.Store(capture, sp)
	return sp.style, sp.priority
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

func applyByteStyle(
	styleGrid [][]spanPriority,
	lines []string,
	lineStarts []int,
	startByte int,
	endByte int,
	style tokenStyle,
	priority int,
) {
	if len(styleGrid) == 0 || endByte <= startByte {
		return
	}
	startLine, startColByte := byteOffsetToLineCol(lineStarts, startByte)
	endLine, endColByte := byteOffsetToLineCol(lineStarts, endByte)
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
		if endColRune > len(row) {
			endColRune = len(row)
		}
		for i := max(startColRune, 0); i < endColRune; i++ {
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

const markdownFallbackQuery = `
[
  (atx_heading)
  (setext_heading)
  (atx_h1_marker)
  (atx_h2_marker)
  (atx_h3_marker)
  (atx_h4_marker)
  (atx_h5_marker)
  (atx_h6_marker)
] @heading

[
  (link_label)
  (link_destination)
  (link_title)
  (link_reference_definition)
] @link

[
  (fenced_code_block)
  (code_fence_content)
  (fenced_code_block_delimiter)
  (indented_code_block)
  (info_string)
] @string

[
  (thematic_break)
  (block_quote_marker)
  (list_marker_plus)
  (list_marker_minus)
  (list_marker_star)
  (list_marker_dot)
  (list_marker_parenthesis)
] @punctuation

(html_block) @comment
`
