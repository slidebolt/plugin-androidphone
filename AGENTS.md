# plugin-androidphone Instructions

`plugin-androidphone` follows the standard SlideBolt runnable module architecture.

## Standard layout

Use this structure for every service or plugin unless there is a strong reason not to:

```text
<module>/
  AGENTS.md
  app/
    app.go
    commands.go
    seed.go
    *_test.go
  cmd/<binary>/main.go
  internal/
    <private implementation packages>
  pkg/
    <only if another module or harness must import it>
```

## Rules

- `cmd/<binary>/main.go` is a thin shell only. It should construct the importable app and call `runtime.Run(...)`.
- Runtime behavior lives in `app/`. `Hello`, `OnStart`, `OnShutdown`, dependency wiring, and orchestration belong there.
- Private helpers and translators live in `internal/...`.
- Use `pkg/...` only for APIs intentionally imported across modules. Do not default to `pkg`.
- Tests should target `app/` and `internal/...` by default. Keep `cmd` tests to smoke coverage only.
- If the shared test environment needs to import code, it must not live only in `cmd` or an inaccessible `internal` package.

## Why this exists

- `package main` cannot be the reusable integration surface.
- Thin binaries make runtime wiring obvious and consistent.
- Importable `app` packages make cross-module testing and harness reuse practical.
- A consistent tree reduces one-off layout decisions across repos.

## What to copy

When creating or refactoring another runnable module:

1. Copy the `cmd` thin-wrapper pattern.
2. Move lifecycle logic into `app/`.
3. Move translators/adapters into `internal/...`.
4. Only introduce `pkg/...` when another module truly needs the API.
