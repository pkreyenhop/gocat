package main

import "strings"

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
	lineStyles [][]tokenStyle
}

func newGoHighlighter() *syntaxHighlighter {
	return &syntaxHighlighter{}
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
