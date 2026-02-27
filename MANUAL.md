# Gocat Manual

## Overview

Gocat (“gc”) nods to GoCat and is inspired by the Canon Cat, Helix, acme, AMP, and Emacs. It pairs an SDL2 UI with a headless editing core, focusing on fast “Leap” navigation, multi-buffer workflows, and keyboard-centric shortcuts. Fonts are bundled (JetBrains Mono), the background is purple-toned, and the status/input lines sit at the bottom for mode and prompt feedback.

## Launching

```sh
go build -o gc .
./gc [file1 file2 ...]
```

- Passing existing files opens each in its own buffer.
- Missing filenames open empty buffers with that path; the file is created on first save.
- `Ctrl+B` creates a new `<untitled>` buffer; name it on save via the input line.

## Navigation & Selection

- **Leap (case-insensitive):** Hold Right Cmd to leap forward, Left Cmd to leap backward. Type while held to jump to the next match anchored at the origin caret. Release both to commit.
- **Leap Again:** `Ctrl+Right Cmd` / `Ctrl+Left Cmd` repeats the last query forward/backward.
- **Selection while leaping:** Hold both Cmd keys; further typing extends the selection.
- **Arrows / PageUp / PageDown:** Move or select with Shift.
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

## Buffers & Files

- **New / cycle buffers:** `Ctrl+B` creates `<untitled>`; `Tab` / `Shift+Tab` cycles.
- **File picker:** `Ctrl+O` opens a picker buffer rooted at the current directory; entries start with `..` to go up. Leap to a line and press `Ctrl+L` to open; directories open in-place; files open in new buffers or switch if already loaded.
- **Save current:** `Ctrl+W` saves the active buffer. If unnamed (`<untitled>`), the input line prompts “Save as:” — type a path and press Enter.
- **Save dirty buffers:** `Ctrl+Shift+S` saves only buffers marked dirty.
- **Close buffer / quit:** `Ctrl+Q` closes the current buffer; `Ctrl+Shift+Q` quits. `ESC` clears selection, closes the picker, or closes a clean buffer (dirty buffers warn in the status line).

## Status & Input Lines

- **Status (above input):** Shows buffer name, mode (Leap/Edit/Open), language mode (`lang=text|go|markdown|c|miranda`), cwd, `*unsaved*` marker, and last event.
- **Input (bottom):** Used for prompts (e.g., Save as). Type to respond; Enter confirms; Esc cancels.

## Syntax Highlighting

- Tree-sitter highlighting is enabled for:
  - Go (`.go` or first non-empty line starting with `package `)
  - Markdown (`.md` / `.markdown`)
  - C (`.c` / `.h`)
  - Miranda (`.m`, currently parsed via the Haskell Tree-sitter grammar backend)

## Tips & Examples

- **Jump around text:** Hold Right Cmd, type `foo`, release — caret jumps to `foo` ignoring case. Press `Ctrl+Right Cmd` to leap again to the next `foo`.
- **Indent quickly:** Press space twice on a line to insert a tab at its start.
- **Open by pattern:** `Ctrl+O`, type a few letters of the filename with Leap, `Ctrl+L` to open. Use `..` to go up a directory.
- **Save unnamed buffer:** `Ctrl+W`, type `notes/todo.txt` in the input line, Enter — file is created and saved, buffer is renamed.
- **Multiple files:** `./gc file1.txt dir/file2.txt` opens two buffers; `Tab`/`Shift+Tab` cycles.
