# Repository Guidelines

## Project Structure & Module Organization
- `main.go` — SDL2/SDL2_ttf UI driver: window setup, event loop, rendering, and clipboard wiring. Depends on the headless editor package.
- `editor/` — UI-free core: buffer management, leap/search, selection, clipboard abstraction, and helpers for line/column math.
- `editor/editor_logic_test.go` — behaviour-focused tests that exercise the headless editor via a small fixture DSL.
- Root tests: `main_open_test.go` (open/find helpers + startup filtering), `main_buffer_test.go` (buffer switching), `main_shortcuts_test.go` (shortcut/chaos/latency coverage), `main_help_test.go` (README/help sync), `main_gui_test.go` (GUI smoke/input, gated behind `-tags gui`).
- `go.mod` / `go.sum` — module metadata (`sdl-alt-test`). Fonts and other assets are resolved at runtime by `pickFont()`; none are tracked in the repo.

## Build, Test, and Development Commands
- `go run .` — launch the SDL prototype window (needs SDL2 + SDL2_ttf headers/libs available to CGO).
- `go build .` — compile the binary with the same SDL prerequisites.
- `go test ./editor` — run headless logic tests only; safe when SDL is unavailable.
- `go test ./...` — full test/build (also compiles `main.go`); requires SDL toolchain to be installed and discoverable.
- `SDL_VIDEODRIVER=dummy go test -tags gui ./...` — run the optional GUI smoke test (`main_gui_test.go`) if SDL2/SDL2_ttf and a usable monospace font are installed.

## Coding Style & Naming Conventions
- Run `gofmt` before sending changes; default Go tabs/formatting.
- Keep logic UI-agnostic inside `editor/`; prefer injecting dependencies (e.g., clipboard) rather than reaching into SDL.
- Naming: directions as `DirFwd`/`DirBack`, caret/selection fields as `Caret`, `Sel`, `Leap`. Use `[]rune` for buffer text to preserve Unicode indexing. Buffers are tracked via `app.buffers`/`bufIdx`; keep UI-facing shortcuts (`Ctrl+B` new buffer, `Tab` cycle, `Ctrl+O` picker `Ctrl+L` load, `Ctrl+W` save, `Ctrl+Shift+S` save-all, `Ctrl+Q`/`Ctrl+Shift+Q` close/quit, `Ctrl+/` comment toggle, `Ctrl+Shift+/` help buffer) mapped in `handleEvent`.
- Keep comments brief and focused on behaviour (e.g., selection anchoring, wrap semantics).
- Startup filtering: `filterArgsToFiles` only loads regular files from CLI args (skips directories), and `loadStartupFiles` creates new buffers per file. Help overlay/buffer text is sourced from `helpEntries` and README; update both in lockstep.
 - Navigation shortcuts: arrows and PageUp/Down repeat, `Ctrl+,`/`Ctrl+.` page scroll (Shift extends selection), `Ctrl+A/E`, `Ctrl+Shift+A/E`, `Ctrl+K`, `Ctrl+U`. ESC clears selection or closes the picker—never quits.

## Testing Guidelines
- Tests live in `editor/editor_logic_test.go` and use a `fixture` helper (`run(t, buf, caret, func(*fixture))`) to declare behaviour with minimal boilerplate. SDL-facing shortcuts, chaos, and latency checks live in `main_shortcuts_test.go`; README/help sync in `main_help_test.go`; GUI tests behind `-tags gui`.
- Prefer scenario-style tests (“leap again wraps”, “insert replaces selection”) that describe future behaviour. Use fixture helpers like `leap`, `leapAgain`, `commit`, `startSelection`, and expectations such as `expectCaret`/`expectBuffer`.
- Keep tests deterministic and headless; avoid SDL/cgo in tests to keep them portable (GUI tests are opt-in with `-tags gui`).
 - Scrolling and viewport behaviour is covered in `main_scroll_test.go`; add cases there when changing caret visibility rules. GUI smoke/input lives in `main_gui_test.go` (tagged).

## Commit & Pull Request Guidelines
- Commit messages should be present-tense and imperative (e.g., “Add wrap-around leap test”). Mention key behaviours and commands run (`go test ./editor` or `go test ./...`).
- PRs: include a short summary of user-visible changes (UI or behaviour), call out dependency shifts (SDL/toolchain), and link related issues. Screenshots or screen recordings are helpful if rendering changes.

## Environment & Configuration Notes
- Go 1.26+ per `go.mod`. Ensure SDL2/SDL2_ttf headers and shared libs are available via `pkg-config` or well-known paths. `pickFont()` searches Menlo/DejaVu/Liberation; ensure at least one exists or place `DejaVuSansMono.ttf` beside the binary.
