# Gocat

## Overview

This prototype is a small SDL-powered text editor that demonstrates Canon-Cat-style “Leap” navigation. It opens a single window, renders a text buffer with monospace font fallback (Menlo/DejaVu/Liberation), and tracks a caret plus optional selection.

## Core Behavior

- **Leap quasimode**: Hold Right Command to leap forward or Left Command to leap backward. Typing while held builds a query, and the caret jumps to the next match anchored at the origin caret. ESC clears selection or closes the picker; it no longer quits.
- **Dual-Cmd selection**: While leaping with one Command key held, press the other Command key to start a selection anchored at the original caret; further Leap moves extend the selection. Ctrl+Cmd (Left/Right) triggers Leap Again without entering quasimode.
- **Buffers & files**: `Ctrl+B` creates a new untitled buffer; `Tab` cycles buffers. `Ctrl+O` opens a file-picker buffer (non-hidden/vendor under CWD); leap to a filename and press `Ctrl+L` to load it. `Ctrl+W` saves the active buffer; `Ctrl+Shift+S` saves all. `Ctrl+Q` closes the current buffer; `Ctrl+Shift+Q` quits immediately. Startup accepts multiple filenames (regular files only), one buffer each.
- **Editing**: Text input, backspace/delete (with repeat), arrows and PageUp/Down (Shift to select), page scroll with `Ctrl+,` / `Ctrl+.`, line jumps (`Ctrl+A`/`Ctrl+E`), buffer jumps (`Ctrl+Shift+A`/`Ctrl+Shift+E`), comment toggle (`Ctrl+/` on selection or current line; `Ctrl+Shift+/` opens help buffer), kill-to-EOL (`Ctrl+K`), undo (`Ctrl+U`), Enter for newlines.
- **Clipboard**: `Ctrl+C` / `Ctrl+X` / `Ctrl+V` for copy/cut/paste via pluggable clipboard (Cmd is reserved for Leap).
- **Viewport**: The view scrolls to keep the caret on-screen while moving up or down through long files.
- **Rendering cues**: Helix-inspired dark palette; status line shows mode/query/buffer; caret is a blinking block; selection highlighted; active Leap match underlined.

## Shortcut Quick Reference

| Action | Keys |
| --- | --- |
| Leap forward / backward | Hold Right Cmd / Left Cmd (type query) |
| Leap Again | Ctrl+Right Cmd / Ctrl+Left Cmd |
| New buffer / cycle buffers | Ctrl+B / Tab |
| File picker / load line path | Ctrl+O / Ctrl+L (listing starts with `..`; current line filename opens new buffer or switches if already open) |
| Save current / save all | Ctrl+W / Ctrl+Shift+S |
| Close buffer / quit | Ctrl+Q / Ctrl+Shift+Q |
| Undo | Ctrl+U |
| Comment / uncomment | Ctrl+/ (selection or current line) |
| Line start / end | Ctrl+A / Ctrl+E (Shift = select) |
| Buffer start / end | Ctrl+Shift+A / Ctrl+Shift+E |
| Kill to EOL | Ctrl+K |
| Copy / Cut / Paste | Ctrl+C / Ctrl+X / Ctrl+V |
| Navigation | Arrows, PageUp/Down, Ctrl+, Ctrl+. (Shift = select) |
| Escape | Clear selection; close picker |
| Help buffer | Ctrl+Shift+/ (Ctrl+?) |

## Running

Requires Go 1.26+ (per `go.mod`) and SDL2/SDL2_ttf libraries available to CGO (e.g., via Homebrew on macOS). The UI expects `font/JetBrainsMono-Regular.ttf` to exist relative to the binary. Build the binary as `gc` and run it with:

```sh
go build -o gc .
./gc [optional-file]
```

`make build` does the same. Pass a filename to open it at startup (also sets the picker root to that file's directory).

## Testing and Structure

- Headless logic lives in `editor/` (no SDL dependency). Run unit tests with `go test ./editor`.
- The SDL driver is in `main.go` and uses `editor.Editor` for all buffer/leap operations.
- Tests in `editor/editor_logic_test.go` use a small fixture helper (`run(t, buf, caret, func(*fixture))`) so new behaviour specs stay terse and UI-free. Shortcut/picker/end-to-end/latency/chaos tests live in `main_shortcuts_test.go`; file helpers and guards are in `main_open_test.go`. Optional GUI smoke/input tests live in `main_gui_test.go` behind the `gui` build tag; run with `SDL_VIDEODRIVER=dummy go test -tags gui ./...` when SDL2/SDL2_ttf and fonts are available.

## SDL UI Driver (`main.go`)

- Boots SDL2/SDL2_ttf, opens a resizable window, and loads a mono font from common system paths (falls back to `DejaVuSansMono.ttf` beside the binary).
- Creates an `editor.Editor`, injects an SDL-backed clipboard, and enters an event loop that maps Cmd+typing into Leap, dual-Cmd selection, Ctrl+Cmd “Leap Again,” Ctrl+C/X/V clipboard, `Ctrl+B` new buffer, `Tab` buffer cycle, `Ctrl+O` file picker, `Ctrl+L` load selected file, `Ctrl+W` save, `Ctrl+Q` quit, and plain text/caret edits when Leap is inactive.
- Captures text via `TEXTINPUT` events (with KEYDOWN fallback when Cmd suppresses them on macOS) and records the last event/modifiers for on-screen debugging.
- Renders the buffer with a status line showing mode/query/buffer, draws a Helix-like caret block and selection rectangles using monospace metrics, and underlines the current match during an active Leap.
