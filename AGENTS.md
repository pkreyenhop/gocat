# Repository Guidelines

## Project Structure & Module Organization
- `main.go` — SDL2/SDL2_ttf UI driver: window setup, event loop, rendering, and clipboard wiring. Depends on the headless editor package.
- `editor/` — UI-free core: buffer management, leap/search, selection, clipboard abstraction, and helpers for line/column math.
- `editor/editor_logic_test.go` — behaviour-focused tests that exercise the headless editor via a small fixture DSL.
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
- Naming: directions as `DirFwd`/`DirBack`, caret/selection fields as `Caret`, `Sel`, `Leap`. Use `[]rune` for buffer text to preserve Unicode indexing.
- Keep comments brief and focused on behaviour (e.g., selection anchoring, wrap semantics).

## Testing Guidelines
- Tests live in `editor/editor_logic_test.go` and use a `fixture` helper (`run(t, buf, caret, func(*fixture))`) to declare behaviour with minimal boilerplate.
- Prefer scenario-style tests (“leap again wraps”, “insert replaces selection”) that describe future behaviour. Use fixture helpers like `leap`, `leapAgain`, `commit`, `startSelection`, and expectations such as `expectCaret`/`expectBuffer`.
- Keep tests deterministic and headless; avoid SDL/cgo in tests to keep them portable.

## Commit & Pull Request Guidelines
- Commit messages should be present-tense and imperative (e.g., “Add wrap-around leap test”). Mention key behaviours and commands run (`go test ./editor` or `go test ./...`).
- PRs: include a short summary of user-visible changes (UI or behaviour), call out dependency shifts (SDL/toolchain), and link related issues. Screenshots or screen recordings are helpful if rendering changes.

## Environment & Configuration Notes
- Go 1.26+ per `go.mod`. Ensure SDL2/SDL2_ttf headers and shared libs are available via `pkg-config` or well-known paths. `pickFont()` searches Menlo/DejaVu/Liberation; ensure at least one exists or place `DejaVuSansMono.ttf` beside the binary.
