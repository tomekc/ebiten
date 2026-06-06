# Repository Guidelines

## Project Structure & Module Organization
- Public APIs and game loop entry points live at the root (e.g. `run.go`, `window.go`); reusable helpers sit under `audio/`, `inpututil/`, and `text/`.
- Rendering flows from `internal/graphicscommand` through `internal/restorable` into `internal/graphicsdriver`, which defines the cross-platform driver interface consumed by the `opengl`, `metal`, `directx`, and `playstation5` backends.
- Shared math and color utilities (`colorm`, `vector`, `geom`) support every driver, while samples in `examples/` and per-package `*_test.go` files keep verification close to the code.

## Build, Test, and Development Commands
- `go build ./...` validates compilation for all packages and surfaces new cross-module breakages.
- `go test ./...` runs the full suite; narrow the focus with commands like `GOOS=js GOARCH=wasm go test ./internal/graphicsdriver/opengl` or `GOOS=darwin go test ./internal/graphicsdriver/metal/...`.
- `go run ./examples/sprites` (or any sample) offers a fast manual smoke-test for rendering adjustments.
- `go generate ./...` refreshes derived assets such as key maps declared in `generate.go`; run whenever generator inputs change.

## Coding Style & Naming Conventions
- Follow Go idioms (tabs for indentation, mixed-case identifiers); always format with `gofmt` or `go fmt ./...`.
- Preserve the Apache 2.0 header described in `CONTRIBUTING.md` when adding new source files.
- Name backend-specific types with their driver prefix (`openglProgram`, `metalCommandQueue`) and hide platform branches behind build tags instead of runtime switches.

## Testing Guidelines
- Use the standard `testing` package; keep backend assertions near the implementation (`internal/graphicsdriver/opengl/*_test.go`, etc.).
- Probe the command queue via helpers in `internal/graphicsdriver/testing` and confirm image results with `imagedumper.go` when changing rasterization paths.
- Record any manual or platform-gated verification steps in code comments or a package-level README so other maintainers can reproduce them.

## Commit & Pull Request Guidelines
- Mirror the `package: summary` convention shown in `git log` (e.g. `graphicsdriver: fix framebuffer leak`) and link the motivating issue where possible.
- Describe affected platforms, reproduction steps, and visual diffs or frame dumps for rendering-visible patches.
- Keep pull requests narrowly scoped—separate backend refactors, feature work, and documentation updates to simplify review.

## Graphics Backend Notes
- All backends satisfy `internal/graphicsdriver.Graphics`; update the interface and `internal/graphicscommand` in lockstep when adding new capabilities.
- Respect thread affinity and driver ownership boundaries by using `internal/thread` helpers for calls that must land on the main OS thread.
- Guard resource lifetimes with `internal/restorable` and verify caps (texture size, uniform counts, vsync toggles) across every supported backend before merging GPU-affecting changes.
