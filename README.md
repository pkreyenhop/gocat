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
