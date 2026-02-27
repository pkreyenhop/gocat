package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type completionItem struct {
	Label  string
	Insert string
	Detail string
}

type completionState struct {
	Active      bool
	Items       []completionItem
	Selected    int
	PrefixStart int
}

type lspCompletionItem struct {
	Label            string `json:"label"`
	InsertText       string `json:"insertText"`
	InsertTextFormat int    `json:"insertTextFormat"`
	Detail           string `json:"detail"`
	TextEdit         struct {
		NewText string `json:"newText"`
	} `json:"textEdit"`
}

type goplsClient struct {
	cmd     *exec.Cmd
	in      io.WriteCloser
	out     *bufio.Reader
	nextID  int
	inited  bool
	opened  map[string]int
	rootURI string
}

func newGoplsClient() *goplsClient {
	return &goplsClient{
		opened: make(map[string]int),
	}
}

func (c *goplsClient) ensureStarted() error {
	if c == nil {
		return fmt.Errorf("nil gopls client")
	}
	if c.cmd != nil {
		return nil
	}
	cmd := exec.Command("gopls")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return err
	}
	c.cmd = cmd
	c.in = stdin
	c.out = bufio.NewReader(stdout)
	c.nextID = 1
	if cwd, err := os.Getwd(); err == nil {
		c.rootURI = pathToURI(cwd)
	}
	return nil
}

func (c *goplsClient) ensureInitialized() error {
	if c.inited {
		return nil
	}
	params := map[string]any{
		"processId": os.Getpid(),
		"rootUri":   c.rootURI,
		"capabilities": map[string]any{
			"textDocument": map[string]any{
				"completion": map[string]any{
					"completionItem": map[string]any{
						"snippetSupport": false,
					},
				},
			},
		},
	}
	if _, err := c.request("initialize", params); err != nil {
		return err
	}
	if err := c.notify("initialized", map[string]any{}); err != nil {
		return err
	}
	c.inited = true
	return nil
}

func (c *goplsClient) complete(path string, content string, line int, col int) ([]completionItem, error) {
	if err := c.ensureStarted(); err != nil {
		return nil, err
	}
	if err := c.ensureInitialized(); err != nil {
		return nil, err
	}
	uri := completionURI(path)
	if err := c.syncDocument(uri, content); err != nil {
		return nil, err
	}
	params := map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"position": map[string]any{
			"line":      line,
			"character": col,
		},
	}
	raw, err := c.request("textDocument/completion", params)
	if err != nil {
		return nil, err
	}
	return parseCompletionItems(raw), nil
}

func (c *goplsClient) hover(path string, content string, line int, col int) (string, error) {
	if err := c.ensureStarted(); err != nil {
		return "", err
	}
	if err := c.ensureInitialized(); err != nil {
		return "", err
	}
	uri := completionURI(path)
	if err := c.syncDocument(uri, content); err != nil {
		return "", err
	}
	params := map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"position": map[string]any{
			"line":      line,
			"character": col,
		},
	}
	raw, err := c.request("textDocument/hover", params)
	if err != nil {
		return "", err
	}
	return parseHoverText(raw), nil
}

func (c *goplsClient) syncDocument(uri, content string) error {
	ver := c.opened[uri]
	if ver == 0 {
		params := map[string]any{
			"textDocument": map[string]any{
				"uri":        uri,
				"languageId": "go",
				"version":    1,
				"text":       content,
			},
		}
		if err := c.notify("textDocument/didOpen", params); err != nil {
			return err
		}
		c.opened[uri] = 1
		return nil
	}
	ver++
	params := map[string]any{
		"textDocument": map[string]any{
			"uri":     uri,
			"version": ver,
		},
		"contentChanges": []map[string]any{
			{"text": content},
		},
	}
	if err := c.notify("textDocument/didChange", params); err != nil {
		return err
	}
	c.opened[uri] = ver
	return nil
}

func (c *goplsClient) close() {
	if c == nil || c.cmd == nil {
		return
	}
	_, _ = c.request("shutdown", nil)
	_ = c.notify("exit", nil)
	_ = c.in.Close()
	_ = c.cmd.Wait()
	c.cmd = nil
}

func (c *goplsClient) request(method string, params any) (json.RawMessage, error) {
	id := c.nextID
	c.nextID++
	msg := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	}
	if err := c.writeMessage(msg); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(600 * time.Millisecond)
	for {
		raw, err := c.readMessage(deadline)
		if err != nil {
			return nil, err
		}
		var envelope struct {
			ID     json.RawMessage `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			continue
		}
		if len(envelope.ID) == 0 {
			continue
		}
		var gotID int
		if err := json.Unmarshal(envelope.ID, &gotID); err != nil || gotID != id {
			continue
		}
		if envelope.Error != nil {
			return nil, fmt.Errorf("%s", envelope.Error.Message)
		}
		return envelope.Result, nil
	}
}

func (c *goplsClient) notify(method string, params any) error {
	msg := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	}
	return c.writeMessage(msg)
}

func (c *goplsClient) writeMessage(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(b))
	if _, err := io.WriteString(c.in, header); err != nil {
		return err
	}
	_, err = c.in.Write(b)
	return err
}

func (c *goplsClient) readMessage(deadline time.Time) ([]byte, error) {
	for {
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("gopls timeout")
		}
		var contentLength int
		for {
			line, err := c.out.ReadString('\n')
			if err != nil {
				return nil, err
			}
			line = strings.TrimRight(line, "\r\n")
			if line == "" {
				break
			}
			if after, ok := strings.CutPrefix(strings.ToLower(line), "content-length:"); ok {
				n, _ := strconv.Atoi(strings.TrimSpace(after))
				contentLength = n
			}
		}
		if contentLength <= 0 {
			continue
		}
		buf := make([]byte, contentLength)
		if _, err := io.ReadFull(c.out, buf); err != nil {
			return nil, err
		}
		return buf, nil
	}
}

func parseCompletionItems(raw json.RawMessage) []completionItem {
	var direct []lspCompletionItem
	if err := json.Unmarshal(raw, &direct); err == nil && len(direct) > 0 {
		return mapCompletionItems(direct)
	}
	var list struct {
		Items []lspCompletionItem `json:"items"`
	}
	if err := json.Unmarshal(raw, &list); err == nil {
		return mapCompletionItems(list.Items)
	}
	return nil
}

func mapCompletionItems(items []lspCompletionItem) []completionItem {
	out := make([]completionItem, 0, len(items))
	seen := map[string]struct{}{}
	for _, it := range items {
		text := it.TextEdit.NewText
		if text == "" {
			text = it.InsertText
		}
		if text == "" {
			text = it.Label
		}
		if it.InsertTextFormat == 2 {
			text = stripSnippet(text)
		}
		if text == "" {
			continue
		}
		key := it.Label + "\x00" + text
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, completionItem{
			Label:  it.Label,
			Insert: text,
			Detail: it.Detail,
		})
		if len(out) >= 20 {
			break
		}
	}
	return out
}

func parseHoverText(raw json.RawMessage) string {
	var payload struct {
		Contents json.RawMessage `json:"contents"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil || len(payload.Contents) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(payload.Contents, &s); err == nil {
		return s
	}
	var markup struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(payload.Contents, &markup); err == nil && markup.Value != "" {
		return markup.Value
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(payload.Contents, &arr); err == nil {
		var out strings.Builder
		for _, it := range arr {
			var ss string
			if err := json.Unmarshal(it, &ss); err == nil {
				if out.Len() > 0 {
					out.WriteByte('\n')
				}
				out.WriteString(ss)
				continue
			}
			var m struct {
				Value string `json:"value"`
			}
			if err := json.Unmarshal(it, &m); err == nil && m.Value != "" {
				if out.Len() > 0 {
					out.WriteByte('\n')
				}
				out.WriteString(m.Value)
			}
		}
		return out.String()
	}
	return ""
}

func stripSnippet(s string) string {
	if s == "" {
		return s
	}
	var b bytes.Buffer
	for i := 0; i < len(s); i++ {
		if s[i] == '$' {
			if i+1 < len(s) && s[i+1] == '{' {
				j := i + 2
				for j < len(s) && s[j] != '}' {
					j++
				}
				if j < len(s) {
					inner := s[i+2 : j]
					if k := strings.IndexByte(inner, ':'); k >= 0 && k+1 < len(inner) {
						b.WriteString(inner[k+1:])
					}
					i = j
					continue
				}
			}
			if i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '9' {
				i++
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func completionURI(path string) string {
	if path != "" {
		return pathToURI(path)
	}
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	return pathToURI(filepath.Join(cwd, "untitled.go"))
}

func pathToURI(path string) string {
	if path == "" {
		return "file:///untitled.go"
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(abs)}
	return u.String()
}

func identPrefixStart(buf []rune, caret int) int {
	if caret < 0 {
		caret = 0
	}
	if caret > len(buf) {
		caret = len(buf)
	}
	i := caret
	for i > 0 {
		r := buf[i-1]
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			i--
			continue
		}
		break
	}
	return i
}
