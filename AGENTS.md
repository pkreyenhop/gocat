# Repository Guidelines

## Project Structure & Module Organization
- Identity: the editor is called “gc” (from GoCat) and draws inspiration from the Canon Cat, Helix, acme, AMP, and Emacs; keep README and RULES aligned with that positioning.
- `main.go` — SDL2/SDL2_ttf UI driver: window setup, event loop, rendering, and clipboard wiring. Depends on the headless editor package.
- `lsp_gopls.go` — minimal JSON-RPC client for `gopls` completion requests and snippet sanitization.
- `editor/` — UI-free core: buffer management, leap/search, selection, clipboard abstraction, and helpers for line/column math.
- `editor/editor_logic_test.go` — behaviour-focused tests that exercise the headless editor via a small fixture DSL.
- Root tests: `main_open_test.go` (open/find helpers + startup filtering), `main_buffer_test.go` (buffer switching), `main_shortcuts_test.go` (shortcut/chaos/latency coverage), `main_help_test.go` (README/help sync), `main_gui_test.go` (GUI smoke/input, gated behind `-tags gui`).
- `RULES.md` — canonical list of implemented behaviours; keep in sync when changing shortcuts or UI flows.
- `README.md` — user-facing overview/shortcuts; must stay consistent with help entries and RULES.
- `go.mod` / `go.sum` — module metadata (`gc`). Fonts are bundled under `font/` (JetBrains Mono); `pickFont()` loads `font/JetBrainsMono-Regular.ttf`.

## Build, Test, and Development Commands
- `go run .` — launch the SDL prototype window (needs SDL2 + SDL2_ttf headers/libs available to CGO).
- `go build .` — compile the binary with the same SDL prerequisites.
- `go test ./editor` — run headless logic tests only; safe when SDL is unavailable.
- `go test ./...` — full test/build (also compiles `main.go`); requires SDL toolchain to be installed and discoverable.
- `SDL_VIDEODRIVER=dummy go test -tags gui ./...` — run the optional GUI smoke test (`main_gui_test.go`) if SDL2/SDL2_ttf and a usable monospace font are installed.

## Coding Style & Naming Conventions
- Run `gofmt` before sending changes; default Go tabs/formatting.
- Keep logic UI-agnostic inside `editor/`; prefer injecting dependencies (e.g., clipboard) rather than reaching into SDL.
- Naming: directions as `DirFwd`/`DirBack`, caret/selection fields as `Caret`, `Sel`, `Leap`. Use `[]rune` for buffer text to preserve Unicode indexing. Buffers are tracked via `app.buffers`/`bufIdx`; keep UI-facing shortcuts (`Ctrl+B` new buffer, `Ctrl+Tab`/`Ctrl+Shift+Tab` cycle, `Ctrl+O` picker `Ctrl+L` load, `Ctrl+W` save with input prompt for `<untitled>`, `Ctrl+Shift+S` save dirty buffers, `Ctrl+Q`/`Ctrl+Shift+Q` close/quit, `Ctrl+/` comment toggle, `Ctrl+Shift+/` help buffer, Delete word, Shift+Delete line, `Tab` Go autocomplete) mapped in `handleEvent`. Double-space indents by inserting a tab at line start; Leap searches are case-insensitive.
- Keep comments brief and focused on behaviour (e.g., selection anchoring, wrap semantics).
- Startup filtering: `filterArgsToFiles` only loads regular files from CLI args (skips directories), and `loadStartupFiles` creates new buffers per file. Help overlay/buffer text is sourced from `helpEntries` and README; update both in lockstep.
 - Navigation shortcuts: arrows and PageUp/Down repeat, `Ctrl+,`/`Ctrl+.` page scroll (Shift extends selection), `Ctrl+A/E`, `Ctrl+Shift+A/E`, `Ctrl+K`, `Ctrl+U`. ESC clears selection, closes picker, or closes a clean buffer (dirty buffers stay open with a warning). Status bar shows `lang=<mode>` and `*unsaved*` when dirty; input line at bottom handles prompts like Save As. Leap search is case-insensitive and uses a line-number gutter with current-line highlight.
 - Go-mode autocompletion is powered by `gopls` and is non-interruptive: `Tab` triggers completion, auto-insert only on a single high-confidence candidate (prefix >= 3 and identifier-safe insert text), with no popup selection UI. If `gopls` is unavailable, `Tab` falls back to deterministic Go keyword completion when there is exactly one keyword match.
 - Syntax highlighting uses Tree-sitter for Go (`.go` / `package`), Markdown (`.md`/`.markdown`), C (`.c`/`.h`), and Miranda (`.m`, via Haskell grammar backend).

## Testing Guidelines
- Tests live in `editor/editor_logic_test.go` and use a `fixture` helper (`run(t, buf, caret, func(*fixture))`) to declare behaviour with minimal boilerplate. SDL-facing shortcuts, chaos, and latency checks live in `main_shortcuts_test.go`; README/help sync in `main_help_test.go`; GUI tests behind `-tags gui`.
- Prefer scenario-style tests (“leap again wraps”, “insert replaces selection”) that describe future behaviour. Use fixture helpers like `leap`, `leapAgain`, `commit`, `startSelection`, and expectations such as `expectCaret`/`expectBuffer`.
- Keep tests deterministic and headless; avoid SDL/cgo in tests to keep them portable (GUI tests are opt-in with `-tags gui`).
 - Scrolling and viewport behaviour is covered in `main_scroll_test.go`; add cases there when changing caret visibility rules. GUI smoke/input lives in `main_gui_test.go` (tagged). Dirty tracking drives `Ctrl+Shift+S` and ESC behaviour; keep tests aligned.

## Commit & Pull Request Guidelines
- Commit messages should be present-tense and imperative (e.g., “Add wrap-around leap test”). Mention key behaviours and commands run (`go test ./editor` or `go test ./...`).
- Run `go fix ./...` before committing when changes touch Go code.
- PRs: include a short summary of user-visible changes (UI or behaviour), call out dependency shifts (SDL/toolchain), and link related issues. Screenshots or screen recordings are helpful if rendering changes.

## Environment & Configuration Notes
- Go 1.26+ per `go.mod`. Ensure SDL2/SDL2_ttf headers and shared libs are available via `pkg-config` or well-known paths. `pickFont()` searches Menlo/DejaVu/Liberation; ensure at least one exists or place `DejaVuSansMono.ttf` beside the binary.
