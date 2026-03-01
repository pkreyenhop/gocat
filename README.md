# Gocat

## Overview

This prototype is a small Go TUI text editor (tcell-based) that demonstrates Canon-Cat-style “Leap” navigation. The “gc” name nods to GoCat, and the editor draws inspiration from the Canon Cat, Helix, acme, AMP, and Emacs.

## Core Behavior

- **Leap quasimode**: Leap is currently unbound in TUI mode.
- **Leap selection model**: Leap selection remains in the editor core, but leap trigger keys are currently unbound in the TUI.
- **Buffers & files**: `Ctrl+B` creates a new `<untitled>` buffer; `Shift+Tab` cycles buffers. `Ctrl+O` opens a file-picker buffer (non-hidden/vendor under CWD); move the caret to a filename and press `Ctrl+L` to load it. `Esc+W` opens a write prompt (“Save as: …”) for the active buffer. `Esc+Shift+S` saves only dirty buffers. `Ctrl+Q` closes the current buffer; `Esc+Shift+Q` quits immediately. Startup accepts multiple filenames (regular files only), one buffer each; missing filenames open empty buffers and are created on first save.
- **Save + format/fix/reload**: `Esc+F` saves the current file, runs `go fmt` and `go fix` for the file’s package directory, then reloads the file into the active buffer.
- **Run package**: `Ctrl+R` invokes `go run .` in the active file’s directory and opens a new run-output buffer. The buffer starts with the command line, streams stdout/stderr (`[stderr]`-prefixed), and appends an `[exit]` status footer.
- **Editing**: Text input, backspace/delete (with repeat), Delete removes the word under/left of the caret, Shift+Delete removes the current line, arrows and PageUp/Down (Shift to select), page scroll with `Ctrl+,` / `Ctrl+.`, line jumps (`Ctrl+A`/`Ctrl+E`), buffer jumps (`Ctrl+Shift+A`/`Ctrl+Shift+E`), comment toggle (`Ctrl+/` on selection or current line; `Ctrl+Shift+/` opens help buffer), kill-to-EOL (`Ctrl+K`), undo (`Ctrl+U`), Enter for newlines. Double-space indents the current line by inserting a tab at its start. Passing a missing filename opens an empty buffer with that name; the file is created on first save.
- **Esc command mode**: `Esc` is a command prefix. Examples: `Esc+w` (write-as prompt), `Esc+f` (format/fix/reload), `Esc+Shift+S` (save dirty buffers), `Esc+Shift+Q` (quit all), `Esc+i` (symbol info), `Esc+Esc` (close buffer).
- **Esc delayed help popup**: If `Esc` is pressed and no next key is entered quickly, a bottom-right popup appears with grouped `Esc`-prefix commands (next-letter actions only).
- **Search mode**: `Esc+/` starts incremental search. Type the pattern and the caret jumps to full matches while typing. Press `/` to lock the pattern, then use `Tab` / `Shift+Tab` to move next/previous (with wrap). Entering `/` with an empty pattern repeats the last non-empty search and jumps to the next match. After lock, `x` switches into line-highlight mode; other keys exit search and run their normal action.
- **Line highlight mode**: `Esc+X` starts line highlighting at the current line. Press `x` again to extend by one more line each time. `Esc` exits line-highlight mode.
- **Buffer clear**: `Esc+Shift+Delete` deletes the entire contents of the active buffer.
- **Language mode cycle**: `Esc+M` cycles language mode for the active buffer (`text -> go -> markdown -> c -> miranda -> text`). This is useful for untitled buffers (for example, force Go mode before naming the file).
- **Less mode**: `Esc` then `Space` enters paging mode. While active, `Space` pages forward repeatedly and `Esc` exits less mode.
- **Go autocompletion**: In Go buffers, completion runs in a non-interruptive mode. Deterministic keyword completions run first (for example, `pack` -> `package`) and return immediately without waiting for `gopls`. Other completions auto-insert only when confidence is high (identifier prefix length at least 3, exactly one `gopls` candidate, identifier-only insert text), so there is no suggestion popup. If `gopls` is unavailable, completion is automatically disabled.
- **Clipboard**: `Ctrl+C` / `Ctrl+X` / `Ctrl+V` for copy/cut/paste via pluggable clipboard.
- **Viewport**: The view scrolls to keep the caret on-screen while moving up or down through long files.
- **Rendering cues**: Purple palette; status line shows mode/query/buffer, `lang=<mode>`, and `*unsaved*`; input line sits below for prompts; gutter shows line numbers (current line highlighted); caret is a blinking block; selection highlighted; active Leap match underlined. Go buffers (`.go` or `package ...`), Markdown buffers (`.md`/`.markdown`), C buffers (`.c`/`.h`), and Miranda buffers (`.m`) use a pure-Go Tree-sitter highlighter (`gotreesitter`) with no CGO dependency.
- **Go syntax markers**: In Go mode, parse errors are checked with the Go parser, and lines with syntax errors get a red marker in the gutter.
- **Go symbol info**: In Go mode, use `Esc` then `i` to toggle a symbol-info popup for the symbol under cursor (keyword/builtin details with usage examples, local definition lookup, and `gopls` hover fallback). Press `Esc` to close; use `Up/Down` (or `PageUp/PageDown`, `Home/End`) to scroll when needed.

## Shortcut Quick Reference

| Action | Keys |
| --- | --- |
| Leap forward / backward | Unbound in TUI mode |
| Leap Again | N/A in TUI mode |
| New buffer / cycle buffers | Ctrl+B / Shift+Tab |
| File picker / load line path | Ctrl+O / Ctrl+L (listing starts with `..`; current-line filename opens new buffer or switches if already open) |
| Write as / save all | Esc+W / Esc+Shift+S |
| Save + fmt/fix + reload | Esc+F |
| Run package (go run .) | Ctrl+R |
| Close buffer / quit | Ctrl+Q / Esc+Shift+Q |
| Undo | Ctrl+U |
| Comment / uncomment | Ctrl+/ (selection or current line) |
| Line start / end | Ctrl+A / Ctrl+E (Shift = select) |
| Buffer start / end | Ctrl+Shift+A / Ctrl+Shift+E |
| Kill to EOL | Ctrl+K |
| Copy / Cut / Paste | Ctrl+C / Ctrl+X / Ctrl+V |
| Symbol info under cursor (Go) | Esc+I |
| Cycle language mode | Esc+M |
| Search mode | Esc+/ then type pattern; / locks; Tab/Shift+Tab navigate; x enters line highlight mode |
| Search repeat | In search mode, / on empty pattern repeats last search |
| Line highlight mode | Esc+X (or x from locked search), then x to extend by line; Esc exits |
| Less mode | Esc+Space (Space page, Esc exit) |
| Autocomplete (Go mode) | Tab |
| Navigation | Arrows, PageUp/Down, Ctrl+, Ctrl+. (Shift = select) |
| Delete / line / buffer delete | Delete word under/left of caret / Shift+Delete line / Esc+Shift+Delete buffer |
| Delete buffer contents | Esc+Shift+Delete |
| Escape | Closes symbol info popup or exits less mode; otherwise command prefix (Esc then Esc closes current buffer) |
| Help buffer | Ctrl+Shift+/ (Ctrl+?) |

## Running

Requires Go 1.26+ (per `go.mod`). Build the binary as `gc` and run it with:

```sh
go build -o gc .
./gc [optional-file]
```

`make build` does the same. Pass a filename to open it at startup (also sets the picker root to that file's directory).

## Go Completion

- Completion is enabled only when the active buffer language mode is `go`.
- Backend: `gopls` over LSP (`stdio` JSON-RPC).
- Trigger: pressing `Tab` in a Go buffer checks for a high-confidence completion.
- Fast path: unique Go keyword matches complete immediately before any `gopls` request.
- Insert behavior: when confidence is high enough, the current identifier prefix at caret is replaced automatically.
- If `gopls` is missing or returns errors/timeouts, completion is disabled for the session and editing continues normally.
- When `gopls` is unavailable, `Tab` still supports deterministic Go keyword completion if the current prefix has exactly one keyword match (for example, `packa` -> `package`).
- Current scope/limitations:
  - Go-only completion (no completion for Markdown/C/Miranda/text modes)
  - Auto-complete only when prefix length is at least 3 and there is exactly one identifier-safe candidate (no popup choices)
  - Basic completion items only (snippet placeholders are stripped to plain text)
  - No hover/signature/help UI yet

## Testing and Structure

- Headless logic lives in `editor/` (no UI dependency). Run unit tests with `go test ./editor`.
- The editor core stores text in a gap-buffer-backed model and exposes accessors (`Runes()`, `String()`, `RuneLen()`) instead of direct buffer field mutation.
- Platform-neutral input/controller logic lives in `input_core.go` (`keyEvent`, `modMask`, `handleKeyEvent`, `handleTextEvent`), so frontends can reuse editing behavior independent of transport.
- Runtime frontend is the Go TUI in `main_tui.go` (tcell).
- Tests in `editor/editor_logic_test.go` use a small fixture helper (`run(t, buf, caret, func(*fixture))`) so new behaviour specs stay terse and UI-free. Core file helpers/scrolling/syntax/command-mode checks are in root `_test.go` files.

## TUI Frontend (`main_tui.go`)

- Uses `tcell` for terminal rendering/input and routes key/text actions through the shared controller in `input_core.go`.
- Keeps core shortcuts intact (`Ctrl+R`, `Ctrl+O`, `Ctrl+L`, editing/navigation/selection), including `Esc`-prefix command mode (`Esc+W`, `Esc+F`, `Esc+Shift+S`, `Esc+Shift+Q`, `Esc+I`, `Esc+M`, `Esc+Shift+Delete`) and less-mode paging.
- Leap activation is currently unbound in TUI mode.
- Renders a lightweight terminal view with gutter, status, input line, and caret visibility management.
