# Repository Guidelines

## Project Structure & Module Organization
- Identity: the editor is called "gc" (from GoCat) and draws inspiration from the Canon Cat, Helix, acme, AMP, and Emacs; keep README and RULES aligned with that positioning.
- `main_tui.go` — Go TUI frontend (tcell): screen setup, event loop, terminal rendering, and key dispatch into the controller.
- `main.go` — shared app state and non-UI editor/file helpers used by the TUI and tests.
- `input_core.go` — platform-agnostic input/controller layer (`keyEvent`, `modMask`, text/open/input handlers).
- `lsp_gopls.go` — minimal JSON-RPC client for `gopls` completion/hover requests, snippet sanitization, and completion-doc extraction.
- `editor/` — UI-free core: buffer management, leap/search, selection, clipboard abstraction, and helpers for line/column math.
- `editor/editor_logic_test.go` — behaviour-focused tests using a small fixture DSL.
- Root tests: `main_open_test.go`, `main_buffer_test.go`, `main_scroll_test.go`, `main_syntax_test.go`, `main_tui_test.go`, `main_help_test.go`.
- `RULES.md` — canonical list of implemented behaviours; keep in sync when changing shortcuts or UI flows.
- `README.md` — user-facing overview/shortcuts; must stay consistent with help entries and RULES.

## Build, Test, and Development Commands
- `go run .` — launch the TUI editor.
- `go build .` — compile the TUI binary.
- `go test ./editor` — run headless logic tests only.
- `go test ./...` — full test/build for TUI + headless editor logic.

## Coding Style & Naming Conventions
- Run `gofmt` before sending changes; default Go tabs/formatting.
- Keep logic UI-agnostic inside `editor/`; prefer injecting dependencies (e.g., clipboard) rather than reaching into TUI implementation details.
- Naming: directions as `DirFwd`/`DirBack`, caret/selection fields as `Caret`, `Sel`, `Leap`.
- Use `[]rune` for buffer text to preserve Unicode indexing.
- Editor text storage is gap-buffer-backed; prefer editor APIs (`Runes`, `String`, `RuneLen`, `SetRunes`) instead of direct field access.
- Buffers are tracked via `app.buffers`/`bufIdx`; keep UI-facing shortcuts and help text (`helpEntries`) in sync with README/RULES.
- Esc-prefixed command shortcuts include `Esc+M` for cycling forced buffer language mode (`text/go/markdown/c/miranda`), which affects highlighting and Go-only tooling behavior in untitled buffers.
- Esc-prefixed destructive edit includes `Esc+Shift+Delete`, which clears the active buffer contents and marks it dirty.
- Go completion uses `Tab`: unique keyword/import-prefix expansions apply directly; selector completion (`pkg.`) opens a chooser popup with a delayed upper-right details popup.

## Testing Guidelines
- Prefer scenario-style tests that describe behaviour.
- Keep tests deterministic and headless; avoid external UI/system dependencies.
- Add TUI key-flow checks in `main_tui_test.go` and core command-mode checks in `input_core_test.go`.
- Keep README/help sync via `main_help_test.go`.

## Commit & Pull Request Guidelines
- Commit messages should be present-tense and imperative (e.g., "Add wrap-around leap test").
- Mention key behaviours and commands run (`go test ./editor` or `go test ./...`).
- Run `go fix ./...` before committing when changes touch Go code.

## Environment & Configuration Notes
- Go 1.26+ per `go.mod`.
