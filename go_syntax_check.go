package main

import (
	"go/parser"
	"go/scanner"
	"go/token"
	"strconv"
	"strings"
)

type goSyntaxChecker struct {
	lastPath   string
	lastSource string
	lastLines  int
	lineErrors map[int]struct{}
}

func newGoSyntaxChecker() *goSyntaxChecker {
	return &goSyntaxChecker{}
}

func (c *goSyntaxChecker) lineErrorsFor(path string, buf []rune) map[int]struct{} {
	if c == nil {
		return nil
	}
	src := string(buf)
	if detectSyntax(path, src) != syntaxGo {
		c.lastPath = path
		c.lastSource = src
		c.lastLines = len(splitForSyntax(src))
		c.lineErrors = nil
		return nil
	}
	lines := splitForSyntax(src)
	if c.lastPath == path && c.lastSource == src && c.lastLines == len(lines) {
		return c.lineErrors
	}

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, pathForParse(path), src, parser.AllErrors)
	out := map[int]struct{}{}
	if err != nil {
		switch e := err.(type) {
		case scanner.ErrorList:
			for _, se := range e {
				ln := se.Pos.Line - 1
				if ln >= 0 {
					out[ln] = struct{}{}
				}
			}
		default:
			if ln, ok := parseLineFromErr(err.Error()); ok && ln >= 0 {
				out[ln] = struct{}{}
			}
		}
	}
	if len(out) == 0 {
		out = nil
	}
	c.lastPath = path
	c.lastSource = src
	c.lastLines = len(lines)
	c.lineErrors = out
	return out
}

func pathForParse(path string) string {
	if strings.TrimSpace(path) == "" {
		return "untitled.go"
	}
	return path
}

func splitForSyntax(src string) []string {
	return strings.Split(src, "\n")
}

func parseLineFromErr(msg string) (int, bool) {
	parts := strings.Split(msg, ":")
	if len(parts) < 3 {
		return 0, false
	}
	// Parse the second numeric field from the right to get "line"
	// in messages like "file:line:col: msg" or "line:col: msg".
	numSeen := 0
	for i := len(parts) - 1; i >= 0; i-- {
		n, err := strconv.Atoi(strings.TrimSpace(parts[i]))
		if err != nil {
			continue
		}
		numSeen++
		if numSeen == 2 {
			return n - 1, true
		}
	}
	return 0, false
}
