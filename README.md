## Overview

This prototype is a small SDL-powered text editor that demonstrates Canon-Cat-style “Leap” navigation. It opens a single window, renders a text buffer with monospace font fallback (Menlo/DejaVu/Liberation), and tracks a caret plus optional selection.

## Core Behavior

- **Leap quasimode**: Hold Right Command to leap forward or Left Command to leap backward. Typing while held builds a query, and the caret jumps to the next match anchored at the origin caret.
- **Dual-Cmd selection**: While leaping with one Command key held, press the other Command key to start a selection anchored at the original caret; further Leap moves extend the selection.
- **Leap Again**: Press Ctrl+Right Command (forward) or Ctrl+Left Command (backward) when not leaping to repeat the last committed query with wrap-around search.
- **Editing**: Supports text input, backspace/delete, arrow-key caret movement (with Shift to extend selection), and Enter to insert newlines.
- **Clipboard**: Uses Ctrl+C / Ctrl+X / Ctrl+V for copy/cut/paste (Cmd is reserved for Leap).
- **Rendering cues**: Status line shows mode and last query, caret is drawn per line, selection is highlighted, and found matches underline during Leap.

## Running

Requires Go 1.25.5 (per `go.mod`) and SDL2/SDL2_ttf libraries available to CGO (e.g., via Homebrew on macOS). Run the prototype with:

```sh
go run .
```

## Testing and Structure

- Headless logic lives in `editor/` (no SDL dependency). Run unit tests with `go test ./editor`.
- The SDL driver is in `main.go` and uses `editor.Editor` for all buffer/leap operations.
- Tests in `editor/editor_logic_test.go` use a small fixture helper (`run(t, buf, caret, func(*fixture))`) so new behaviour specs stay terse and UI-free.
- Optional GUI smoke test lives in `main_gui_test.go` behind the `gui` build tag; run with `SDL_VIDEODRIVER=dummy go test -tags gui ./...` when SDL2/SDL2_ttf and fonts are available.

## SDL UI Driver (`main.go`)

- Boots SDL2/SDL2_ttf, opens a resizable window, and loads a mono font from common system paths (falls back to `DejaVuSansMono.ttf` beside the binary).
- Creates an `editor.Editor`, injects an SDL-backed clipboard, and enters an event loop that maps Cmd+typing into Leap, dual-Cmd selection, Ctrl+Cmd “Leap Again,” Ctrl+C/X/V clipboard, and plain text/caret edits when Leap is inactive.
- Captures text via `TEXTINPUT` events (with KEYDOWN fallback when Cmd suppresses them on macOS) and records the last event/modifiers for on-screen debugging.
- Renders the buffer with a status line showing mode/query, draws the caret and selection rectangles using monospace cell metrics, and underlines the current match during an active Leap.
