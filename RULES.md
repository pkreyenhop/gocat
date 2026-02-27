# Gocat Behaviour Rules

“gc” nods to GoCat and the editor draws inspiration from the Canon Cat, Helix, acme, AMP, and Emacs.

- **Leap navigation**
  - Hold Right Cmd to leap forward, Left Cmd to leap backward; queries are case-insensitive.
  - Dual-Cmd selection: hold both Cmd keys while leaping to extend a selection.
  - Leap Again: `Ctrl+Right Cmd` / `Ctrl+Left Cmd` repeats the last query in the respective direction.
  - ESC exits Leap, clears selection if active; during Leap, Cmd+Q is ignored.

- **Buffers & files**
  - `Ctrl+B` creates a new `<untitled>` buffer; `Ctrl+Tab` / `Ctrl+Shift+Tab` cycles buffers.
  - `Ctrl+O` opens a file-picker rooted at the current dir (skips dot/vendor); `..` goes up; directories end with `/` and open in-place; `Ctrl+L` loads the selected path (new buffer or switch if already loaded).
  - Startup loads multiple filenames (skips directories). Missing filenames open empty buffers and are created on first save.
  - `Ctrl+W` saves current; unnamed buffers prompt in the input line (“Save as: …”). `Ctrl+Shift+S` saves only dirty buffers.
  - `Ctrl+Q` closes the current buffer; `Ctrl+Shift+Q` quits. ESC closes clean buffer or picker; dirty buffers stay open and warn.

- **Editing & movement**
  - Text input inserts runes; Enter inserts newline; double-space inserts a tab at line start.
  - Backspace deletes backward; Delete removes the word under/left of caret; `Shift+Delete` removes the current line.
  - `Ctrl+,` / `Ctrl+.` page up/down; arrows and PageUp/Down repeat; Shift extends selection.
  - `Ctrl+A`/`Ctrl+E` to line start/end; `Ctrl+Shift+A`/`Ctrl+Shift+E` to buffer start/end.
  - `Ctrl+K` kills to end of line; `Ctrl+U` undo (single-step).
  - Comment toggle: `Ctrl+/` toggles `//` on selection or current line.
  - Clipboard: `Ctrl+C` copy, `Ctrl+X` cut, `Ctrl+V` paste.
  - Go autocompletion: in Go mode, `Tab` triggers `gopls`; completion auto-inserts only when prefix length is at least 3, exactly one candidate is returned, and insert text is identifier-safe (no popup choices).
  - If `gopls` is unavailable, `Tab` falls back to deterministic Go keyword completion when the current prefix has exactly one keyword match.
  - In Go mode, `Ctrl+I` opens a symbol-info popup for the symbol under cursor (keyword/builtin docs with `gopls` hover fallback); `Esc` closes the popup; `Up/Down`, `PageUp/PageDown`, `Home/End` scroll long popup content.

- **UI & rendering**
  - Purple palette with line-number gutter; current line is highlighted; caret is a blinking block.
  - Go buffers (`.go` path or first non-empty line starting with `package `) use Tree-sitter highlighting for comments, strings, numbers, keywords, type identifiers, and function identifiers.
  - Go buffers run syntax checking via the Go parser; lines with parse errors show a red gutter marker.
  - Markdown buffers (`.md`/`.markdown`) use Tree-sitter highlighting for headings, fenced/indented code blocks, links, list/quote/table punctuation, and thematic breaks.
  - C buffers (`.c`/`.h`) use Tree-sitter highlighting for comments, strings/chars, numeric literals, C keywords, preprocessor directives, and type identifiers.
  - Miranda buffers (`.m`) use Tree-sitter highlighting (currently via the Haskell grammar backend) for comments, strings/chars, numeric literals, declaration keywords, and type nodes.
  - Status bar (above input) shows buffer name, mode, detected language (`lang=<mode>`), cwd, `*unsaved*`, and last event. Input line at bottom handles prompts.
  - Gutter uses buffer background; line numbers dim except the current line, which is bright.

- **Dirty tracking**
  - Editing actions mark buffers dirty; loading/saving clears dirty.
  - `Ctrl+Shift+S` skips clean buffers; status shows `*unsaved*` when dirty.
