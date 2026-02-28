# Gocat Manual

## Overview

Gocat (“gc”) nods to GoCat and is inspired by the Canon Cat, Helix, acme, AMP, and Emacs. It pairs a Go TUI frontend (tcell) with a headless editing core, focusing on fast “Leap” navigation, multi-buffer workflows, and keyboard-centric shortcuts.

## Launching

```sh
go build -o gc .
./gc [file1 file2 ...]
```

- Passing existing files opens each in its own buffer.
- Missing filenames open empty buffers with that path; the file is created on first save.
- `Ctrl+B` creates a new `<untitled>` buffer; name it on save via the input line.

## Navigation & Selection

- **Leap (case-insensitive):** currently unbound in TUI mode.
- **Leap Again:** not currently mapped in TUI mode.
- **Selection while leaping:** available via the editor selection model; terminal mappings focus on reliable single-modifier input.
- **Arrows / PageUp / PageDown:** Move or select with Shift.
- **Page scroll shortcuts:** `Ctrl+,` pages up and `Ctrl+.` pages down (Shift extends selection).
- **Line start/end:** `Ctrl+A` / `Ctrl+E` (Shift extends selection).
- **Buffer start/end:** `Ctrl+Shift+A` / `Ctrl+Shift+E`.
- **Line jump assist:** Current line is highlighted; line numbers are shown in a gutter.

## Editing

- **Insert:** Normal typing; Enter inserts newline; double-space inserts a tab at line start.
- **Delete:** `Backspace` deletes backward; `Delete` removes the word under/left of the caret; `Shift+Delete` removes the current line.
- **Kill to EOL:** `Ctrl+K` deletes to end of line (and newline if not last line).
- **Undo:** `Ctrl+U` (single-step).
- **Comment toggle:** `Ctrl+/` toggles `//` on selection or current line.
- **Clipboard:** `Ctrl+C` copy, `Ctrl+X` cut, `Ctrl+V` paste.
- **Go autocompletion:** In Go buffers, completion is non-interruptive. The editor auto-completes only when there is a single high-confidence `gopls` result (prefix length at least 3 and identifier-safe insert text); no suggestion popup is shown.
- **Go symbol info:** `Esc` then `i` toggles a popup with information about the symbol under cursor (keywords/builtins with usage examples, local definitions, and hover text when available). `Esc` closes the popup; `Up/Down`, `PageUp/PageDown`, `Home/End` scroll long content.
- **Esc command mode:** `Esc` is a command prefix for control-style actions (`Esc+f`, `Esc+Shift+S`, `Esc+Shift+Q`, `Esc+i`, `Esc+Esc`).
- **Esc delayed help popup:** If `Esc` stays pending for a short delay, a lower-right popup appears showing grouped `Esc` commands by next letter (no `Ctrl+...` entries).
- **Search mode:** `Esc+/` enters incremental search. Type the pattern (caret jumps to full matches while typing), then press `/` to lock the pattern. While locked, `Tab`/`Shift+Tab` move to next/previous match with wrap. If the current pattern is empty when `/` is pressed, the editor reuses the last non-empty search pattern and jumps to the next match. Any other key exits search and performs its normal action; `x` exits search and enters line-highlight mode.
- **Line highlight mode:** `Esc+X` starts line highlighting from the current line. Press `x` repeatedly to extend selection by one line each time. `Esc` exits this mode.
- **Buffer clear:** `Esc+Shift+Delete` clears the entire active buffer.
- **Language mode cycle:** `Esc+M` cycles active buffer language mode (`text -> go -> markdown -> c -> miranda -> text`), including untitled buffers.
- **Less mode:** `Esc+Space` enters paging mode; `Space` pages forward and `Esc` exits less mode.

## Go Completion Details

- **Activation:** Completion runs only in Go mode (`lang=go` in status line).
- **Engine:** The editor starts `gopls` lazily and communicates over LSP.
- **When it updates:** Pressing `Tab` in a Go buffer triggers a confidence check.
- **Fast keyword path:** Unique Go keyword prefixes complete before any `gopls` request (for example, `pack` -> `package`).
- **Accept semantics:** When confidence is high, the identifier prefix directly before the caret is replaced automatically.
- **Failure mode:** If `gopls` is unavailable or fails, completion is disabled for that session; the editor remains fully usable.
- **Fallback mode:** If `gopls` is unavailable, pressing `Tab` still performs deterministic Go keyword completion when there is exactly one keyword match for the current prefix.
- **Limitations (current):**
  - Go-only completion
  - No completion choices/popup UI; completion only fires on a single strong candidate
  - Snippet placeholders are flattened to plain text
  - No completion docs/hover panel yet

## Buffers & Files

- **New / cycle buffers:** `Ctrl+B` creates `<untitled>`; `Shift+Tab` cycles.
- **File picker:** `Ctrl+O` opens a picker buffer rooted at the current directory; entries start with `..` to go up. Move the caret to a line and press `Ctrl+L` to open; directories open in-place; files open in new buffers or switch if already loaded.
- **Save current:** `Ctrl+W` saves the active buffer. If unnamed (`<untitled>`), the input line prompts “Save as:” — type a path and press Enter.
- **Save + fmt/fix + reload:** `Esc+F` saves current file, runs `go fmt` and `go fix` in the file's directory package context, then reloads the file into the current buffer.
- **Run package:** `Ctrl+R` invokes `go run .` in the active file's directory and opens a run-output buffer. It writes the executed command header first, streams stdout/stderr (`[stderr]`-prefixed), then appends an `[exit]` result line.
- **Save dirty buffers:** `Esc+Shift+S` saves only buffers marked dirty.
- **Close buffer / quit:** `Ctrl+Q` closes the current buffer; `Esc+Shift+Q` quits. `Esc` is a command prefix; press `Esc` then `Esc` to close the current buffer.

## Status & Input Lines

- **Status (above input):** Shows buffer name, mode (Leap/Edit/Open), language mode (`lang=text|go|markdown|c|miranda`), cwd, `*unsaved*` marker, and last event.
- **Input (bottom):** Used for prompts (e.g., Save as). Type to respond; Enter confirms; Esc cancels.

## Syntax Highlighting

- Pure-Go Tree-sitter highlighting (`gotreesitter`) is enabled with no CGO dependency for:
  - Go (`.go` or first non-empty line starting with `package `)
  - Markdown (`.md` / `.markdown`)
  - C (`.c` / `.h`)
  - Miranda (`.m`)

## Go Syntax Check

- In Go mode, source is parsed with Go's parser (`parser.AllErrors`).
- Any line with a parse error is marked with a red gutter indicator.
- Syntax checking is disabled for non-Go buffers.

## Tips & Examples

- **Jump around text:** leap remains available in editor core, but is currently unbound in TUI mode.
- **Indent quickly:** Press space twice on a line to insert a tab at its start.
- **Open by pattern:** `Ctrl+O`, type a few letters to filter/select the filename, `Ctrl+L` to open. Use `..` to go up a directory.
- **Save unnamed buffer:** `Ctrl+W`, type `notes/todo.txt` in the input line, Enter — file is created and saved, buffer is renamed.
- **Multiple files:** `./gc file1.txt dir/file2.txt` opens two buffers; `Shift+Tab` cycles.
