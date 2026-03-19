# Hostward Implementation Plan

## PRD Gaps To Resolve

- The PRD state model is now best treated as four user-visible states:
  - `ok`: latest evaluation succeeded
  - `failing`: latest evaluation completed and found a problem, including runner errors and timeouts
  - `unknown`: enabled but not yet trustworthy, such as never-run or never-poked
  - `disabled`: explicitly disabled by the operator
- Path defaults are split between XDG-style config/state paths and "normal macOS-style" logs. The clean compromise is:
  - config: `~/.config/hostward`
  - monitors: `~/.config/hostward/monitors`
  - state/history: `~/.local/state/hostward`
  - cache/current snapshot: `~/.cache/hostward`
  - operational logs: `~/Library/Logs/hostward`
  - launch agent plist: `~/Library/LaunchAgents/com.hostward.scheduler.plist`
- Notification behavior should be one notification on failure start by default. Do not promise sticky delivery semantics that macOS does not let us force from the app side.
- Schedule syntax is still open. V0 should use simple interval strings like `every = "5m"` and avoid cron parsing until it is actually needed.
- Script runner policy is missing. We need concrete rules for timeout, inherited environment, working directory, max captured output size, and truncation behavior.
- Restart reconciliation is vague. The conservative default is:
  - overdue script monitors run immediately when the scheduler starts
  - dead man's switch monitors evaluate from the recorded poke timestamp and can flip straight to failing
- Monitor identity should use a stable `id` equal to the file stem. Optional `name` can be display-only.
- Disable semantics need to be explicit. Disabling should suppress scheduling, notifications, and failing counts, but should still write a history transition.
- History retention is defined. Operational log retention is not. We should bound logs by both days and max bytes.
- `shell-init` should print shell snippets, not edit dotfiles. Auto-editing `~/.zshrc` is a good way to get yelled at by future Jim.

## Runtime Options

### Go

- Best fit for a long-running per-user agent with low idle overhead.
- Single binary makes `launchd` setup cleaner than Python virtualenv paths.
- Standard library is enough for CLI plumbing, plist generation, JSON, subprocess execution, and file IO.
- Weak spot: if we later want richer native notification behavior, we may still want a tiny helper instead of shelling out.

### Python

- Fastest path for iteration and monitor authoring ergonomics.
- Great stdlib support for TOML, plist, subprocesses, and JSON.
- Worse operational story for `launchd` because interpreter and environment paths have to stay stable.

### Swift

- Best native access to macOS notifications and system APIs.
- Slowest path for CLI and scheduler implementation, and not worth it for a shell-first v0 unless native notification fidelity is the deciding requirement.

## Preferred Choice

Use Go for the main binary. It matches the long-running `launchd` agent model, keeps install/runtime simple, and should stay boring under load.

## Current Status

The core runtime described below is now implemented:

- config and monitor TOML loading
- atomic runtime state and cached snapshot writes
- scheduler reconciliation for script and deadman monitors
- per-user `launchd` agent render/install/status
- failure-start notifications through a best-effort `osascript` adapter
- built-in monitor scaffolds and manual lifecycle commands
- JSONL history plus operational logs with retention pruning

The remaining work is mostly hardening and operator UX:

- deeper `doctor` diagnostics where macOS APIs allow it
- runner policy refinement around environment, working directory, and truncation defaults
- more end-to-end CLI coverage and dogfooding
- any feature additions that survive real-world use

## Phased Implementation

### Phase 0: Foundations

- Lock the v0 path and identity decisions above.
- Create repo skeleton and shell-facing CLI seams.
- Define the cached state file contract used by shell integration.
- Add example config and monitor definitions.

Exit criteria:

- `hostward shell-init zsh|bash` prints usable shell hooks.
- `hostward banner`, `hostward env failing-count`, `hostward status`, and `hostward doctor` run against cached state.

### Phase 1: Config, State, and Logging

- Implement global config and per-monitor TOML loading.
- Validate monitor uniqueness, names, schedule fields, and per-type settings.
- Implement atomic current-state writes.
- Implement JSONL history writer and JSONL operational logger.
- Add retention pruning for history and logs.

Exit criteria:

- Invalid config fails clearly.
- State writes are atomic and recoverable.
- History/log files rotate or prune within configured bounds.

### Phase 2: Scheduler and Script Monitors

- Build the in-process scheduler loop.
- Implement interval scheduling and restart reconciliation.
- Implement script runner with timeout and output capture limits.
- Add `hostward monitor run <name>` and `hostward monitors list`.
- Render and install the per-user `launchd` plist.

Exit criteria:

- Script monitors execute on schedule via the agent.
- Manual run and background scheduling share the same execution path.
- Shell surfaces update from cached state without inline checks.

### Phase 3: Dead Man's Switch and Notifications

- Implement `hostward poke <name>`.
- Evaluate dead man monitors from persisted poke timestamps.
- Record status transitions in history.
- Implement failure-start notification dedupe.
- Add enable/disable behavior.

Exit criteria:

- Dead man monitors change state correctly across restarts.
- Notifications send once on failure start by default.
- Disabled monitors stay out of counts and scheduler runs.

### Phase 4: Built-ins and Polish

- Add built-ins for file freshness, filesystem free space, file exists, directory exists, command succeeds, and process running.
- Add `hostward add script` and `hostward add deadman`.
- Improve `doctor` to check `launchd`, config health, and shell integration.
- Harden truncation, pruning, corruption recovery, and tests.

Exit criteria:

- Common local checks no longer require custom scripts.
- First-run setup is clear and debuggable.
- State corruption or partial writes do not brick the tool.

## Initial Repo Layout

```text
cmd/hostward/main.go           CLI entrypoint
docs/implementation-plan.md    planning and decisions
examples/config.toml           example global config
examples/monitors/*.toml       example monitor definitions
internal/app/                  top-level wiring
internal/cli/                  command parsing and terminal output
internal/config/               path defaults, config load/validate
internal/state/                current snapshot and persisted state types
internal/shell/                banner, env helpers, shell init snippets
internal/scheduler/            scheduler loop and reconciliation
internal/runner/               subprocess monitor execution
internal/monitor/              monitor definitions and status model
internal/history/              JSONL history writes and pruning
internal/logging/              operational logging
internal/notify/               macOS notification adapter
internal/launchd/              plist render/install/status
```

Most of the packages above are now live. Treat the phased plan as the path that got the repo here, not as a statement that the codebase is still scaffold-only.
