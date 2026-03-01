# Gocat Behaviour Rules

“gc” nods to GoCat and the editor draws inspiration from the Canon Cat, Helix, acme, AMP, and Emacs.

- **Leap navigation**
  - Leap trigger keys are currently unbound in TUI mode.
  - Leap selection/repeat behavior remains in editor core logic.
  - ESC exits Leap; outside Leap it closes symbol popup/exits less mode or acts as command prefix.

- **Buffers & files**
  - `Ctrl+B` creates a new `<untitled>` buffer; `Shift+Tab` cycles buffers.
  - `Ctrl+O` opens a file-picker rooted at the current dir (skips dot/vendor); `..` goes up; directories end with `/` and open in-place; `Ctrl+L` loads the selected path (new buffer or switch if already loaded).
  - Startup loads multiple filenames (skips directories). Missing filenames open empty buffers and are created on first save.
  - `Esc+W` opens write/save-as prompt for current buffer in the input line (“Save as: …”). `Esc+Shift+S` saves only dirty buffers.
  - `Esc+F` saves current file, runs `go fmt` and `go fix`, then reloads the file into the active buffer.
  - `Ctrl+R` invokes `go run .` in the active file directory and opens a new run-output buffer with command header, streamed stdout/stderr (`[stderr]` prefix), and trailing `[exit]` status.
  - `Ctrl+Q` closes the current buffer; `Esc+Shift+Q` quits. `Esc` is a command prefix; `Esc` then `Esc` closes the current buffer, `Esc` then `Shift+Q` quits all, and `Esc` then `Shift+S` saves dirty buffers.
  - If `Esc` is pending and no second key arrives quickly, a lower-right popup appears listing grouped `Esc` next-letter commands.
  - `Esc+M` cycles the active buffer language mode through `text -> go -> markdown -> c -> miranda -> text`.
  - `Esc+/` starts incremental search. While entering pattern text, caret jumps to full matches. Typing `/` locks the pattern; then `Tab`/`Shift+Tab` move next/previous with wrap.
  - In search mode, locking with `/` on an empty pattern redoes the last non-empty search and jumps to the next match.
  - In locked search mode, `x` exits search and enters line-highlight mode; other keys exit search and execute their normal behavior.
  - `Esc+X` starts line-highlight mode; repeated `x` extends selection by one line each time; `Esc` exits the mode.
  - `Esc+Shift+Delete` clears the entire active buffer contents and marks it dirty.

- **Editing & movement**
  - Text input inserts runes; Enter inserts newline; double-space inserts a tab at line start.
  - Backspace deletes backward; Delete removes the word under/left of caret; `Shift+Delete` removes the current line.
  - `Ctrl+,` / `Ctrl+.` page up/down; arrows and PageUp/Down repeat; Shift extends selection.
  - `Ctrl+A`/`Ctrl+E` to line start/end; `Ctrl+Shift+A`/`Ctrl+Shift+E` to buffer start/end.
  - `Ctrl+K` kills to end of line; `Ctrl+U` undo (single-step).
  - `Esc+Space` enters less mode: `Space` pages forward, `Esc` exits less mode.
  - Comment toggle: `Ctrl+/` toggles `//` on selection or current line.
  - Clipboard: `Ctrl+C` copy, `Ctrl+X` cut, `Ctrl+V` paste.
  - Go autocompletion: in Go mode, `Tab` first applies deterministic Go keyword completion for unique prefix matches and imported-package-name expansion for unique import prefixes.
  - Selector completion (`pkg.` / `pkg.pref`) opens a popup with `gopls` candidates; `Tab`/`Shift+Tab` (or Up/Down) move selection, Enter applies, Esc cancels.
  - If a completion popup selection is idle briefly, an upper-right detail popup appears with signature/description and formatted code examples.
  - If `gopls` is unavailable, selector popup completion is skipped; deterministic keyword/import-prefix completions still work.
  - In Go mode, `Esc+i` toggles a symbol-info popup for the symbol under cursor (keyword/builtin docs with usage examples, local definition lookup, and `gopls` hover fallback); `Esc` closes the popup; `Up/Down`, `PageUp/PageDown`, `Home/End` scroll long popup content.

- **UI & rendering**
  - Purple palette with line-number gutter; current line is highlighted; caret is a blinking block.
  - Editor text storage is gap-buffer-backed; runtime code uses editor accessor methods rather than mutating internal slices directly.
  - Go buffers (`.go` path or first non-empty line starting with `package `) use pure-Go Tree-sitter highlighting (`gotreesitter`, no CGO) for comments, strings, numbers, and keywords.
  - Go buffers run syntax checking via the Go parser; lines with parse errors show a red gutter marker, and the bottom input/info line shows the current-line error in red.
  - Markdown buffers (`.md`/`.markdown`) use pure-Go Tree-sitter highlighting (`gotreesitter`, no CGO) for headings and links.
  - C buffers (`.c`/`.h`) use pure-Go Tree-sitter highlighting (`gotreesitter`, no CGO) for comments, strings/chars, numeric literals, and C keywords.
  - Miranda buffers (`.m`) use pure-Go Tree-sitter highlighting (`gotreesitter`, no CGO) for comments, strings/chars, numeric literals, and declaration keywords.
  - Status bar (above input) shows buffer name, mode, detected language (`lang=<mode>`), cwd, `*unsaved*`, and last event. Input line at bottom handles prompts.
  - Gutter uses buffer background; line numbers dim except the current line, which is bright.

- **Dirty tracking**
  - Editing actions mark buffers dirty; loading/saving clears dirty.
  - `Esc+Shift+S` skips clean buffers; status shows `*unsaved*` when dirty.
