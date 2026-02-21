# Repository Guidelines

## Project Structure & Module Organization
- Root contains `main.go` (SDL prototype + editor logic) and `editor_logic_test.go` (unit tests for leap/search logic).
- Go module is `sdl-alt-test` with dependencies in `go.mod`/`go.sum`; no subpackages.
- Assets/fonts are resolved at runtime by `pickFont()` from system locations (Menlo/DejaVu/Liberation); nothing is checked into the repo.

## Build, Test, and Development Commands
- `go run .` — launch the SDL window to exercise the Leap UI. Requires SDL2 and SDL2_ttf shared libs available to CGO (e.g., via Homebrew on macOS).
- `go build .` — compile the binary; same SDL prerequisites apply.
- `go test ./...` — run unit tests for editor/leap logic. This also invokes CGO; ensure SDL2/SDL2_ttf headers and libs are installed. If you only need pure logic, keep tests in `*_test.go` free of SDL calls.

## Coding Style & Naming Conventions
- Use `gofmt` before sending changes; default Go formatting and tabs.
- Prefer standard Go naming: exported types in `UpperCamelCase`, locals in `lowerCamelCase`.
- Buffers use `[]rune` for Unicode-aware indexing; caret/selection variables follow existing names (`caret`, `sel`, `leap`).
- Keep comments terse and explanatory when behavior is non-obvious (e.g., selection anchoring, wrap behavior).

## Testing Guidelines
- Tests use the Go `testing` package; function names follow `TestXxx`.
- Keep new tests deterministic and headless; avoid SDL calls in tests to minimize CGO/system dependency flakiness.
- When adding search/leap behavior, mirror scenarios in `editor_logic_test.go` and cover wrap-around, directionality, and selection.

## Commit & Pull Request Guidelines
- Write commit messages in present-tense, imperative mood (e.g., “Add wrap-around leap test”).
- Include a brief description of behavior changes and any test commands run.
- In PRs, summarize user-visible changes (if the UI shifts), call out dependency changes, and link related issues. Screenshots are optional but helpful if UI rendering is affected.

## Environment & Configuration Notes
- Tested with Go 1.25.5 per `go.mod`. Use a matching toolchain to avoid stdlib drift.
- Ensure SDL2/SDL2_ttf headers and dylibs are discoverable by CGO (`pkg-config` or `PKG_CONFIG_PATH` set via Homebrew). Fonts must exist at the paths tried in `pickFont()`.
