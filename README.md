# Hostward

Hostward is a lightweight, local-only monitoring tool for a single Mac. It runs simple checks on schedules or tracks dead man's switch expectations, then exposes failures where the operator already looks: shell startup, shell prompt state, optional GUI notifications, and a local CLI.

The product direction lives in [PRD.md](/Users/jlindley/Code/hostward/PRD.md) and the implementation breakdown lives in [docs/implementation-plan.md](/Users/jlindley/Code/hostward/docs/implementation-plan.md).

## Current Repo Shape

This repo now has a working Go CLI around the shell-facing UX, monitor authoring, manual lifecycle commands, scheduler reconciliation, and `launchd` agent management:

- `hostward banner`
- `hostward env failing-count`
- `hostward build`
- `hostward add script`
- `hostward add deadman`
- `hostward add file-exists`
- `hostward add dir-exists`
- `hostward add file-freshness`
- `hostward add free-space`
- `hostward add dir-size`
- `hostward add process-running`
- `hostward enable`
- `hostward disable`
- `hostward monitors list`
- `hostward monitor show <name>`
- `hostward monitor run <name>`
- `hostward notify test`
- `hostward poke <name>`
- `hostward scheduler once`
- `hostward scheduler run`
- `hostward launchd print-plist|install|uninstall|status`
- `hostward shell-init zsh|bash`
- `hostward status`
- `hostward doctor`

Shell startup and prompt hooks still read cached state only. Scheduler reconciliation, monitor execution, history, `launchd` integration, and failure-start notifications are now implemented.

The repo also now includes:

- TOML config loading and validation
- monitor definition parsing with stable file-stem IDs
- built-in monitor scaffolds for file existence, directory existence, file freshness, free space, directory size, and process-running checks
- persistent enable/disable by rewriting monitor TOML
- runtime state persistence and cached snapshot synthesis
- scheduler reconciliation for script and deadman monitors
- best-effort macOS notifications on failure start and `hostward notify test`
- atomic snapshot writes
- JSONL history pruning and operational log pruning with age and byte caps
- `doctor` diagnostics for shell hook detection, snapshot freshness, `launchd`, and notification adapter availability

## Basic Flow

Build a stable binary first. `launchd install` refuses temporary `go run` executables on purpose.

```bash
hostward build --output ~/.local/bin/hostward
~/.local/bin/hostward launchd install --binary ~/.local/bin/hostward
~/.local/bin/hostward doctor
```

For shell integration:

```bash
eval "$(~/.local/bin/hostward shell-init zsh)"
```

For notification setup:

```bash
~/.local/bin/hostward notify test
```

Notification delivery is best-effort macOS notification behavior, not a guaranteed sticky alert.
Notification delivery failures are logged, but they do not abort monitor reconciliation or manual runs.

## Working Assumptions

- Runtime: Go 1.25+
- Config: TOML
- Monitor definitions: one file per monitor
- State and history: flat files, atomically updated where needed
- Operational log retention: bounded by age and max bytes
- Scheduler: one per-user `launchd` agent

## Layout

```text
cmd/hostward/         CLI entrypoint
docs/                 planning and design notes
examples/             example config and monitor definitions
internal/app/         top-level run entrypoint
internal/cli/         command dispatch and text output
internal/config/      config loading, validation, default paths
internal/fileio/      atomic file writes and append helpers
internal/history/     JSONL state transition history
internal/launchd/     plist render/install/status helpers
internal/logging/     JSONL operational logging
internal/monitor/     monitor and status models
internal/runner/      subprocess script execution
internal/scaffold/    monitor file scaffolding
internal/scheduler/   reconciliation loop
internal/service/     app orchestration
internal/shell/       banner and shell-init helpers
internal/state/       runtime store and cached snapshot model
```
