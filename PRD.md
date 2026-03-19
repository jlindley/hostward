# Hostward PRD

## Summary

Hostward is a lightweight, local-only monitoring tool for a single Mac. It runs simple checks on a schedule or tracks dead man's switch expectations, then surfaces state where the user already works: shell startup, shell prompt, desktop notifications, and a local CLI.

The product is intentionally biased toward "things this machine's operator should notice while using the machine" rather than fleet monitoring, hosted alerting, or remote incident paging.

## Problem

Developers and operators often have small but important local responsibilities that fall between full monitoring stacks and ad hoc shell scripts:

- A file should be touched every few hours.
- A periodic job should poke a heartbeat.
- Backups or rsync syncs should keep happening.
- Disk usage or directory growth should stay within bounds.

Today, these checks usually live as scattered scripts, cron jobs, sticky notes, or half-remembered shell aliases. Failures are easy to miss because the signal is disconnected from the place where the user spends time: the terminal on the machine that needs attention.

## Goals

- Make local machine health visible at shell start and while working in the shell.
- Support simple script-based monitors with minimal framework overhead.
- Support dead man's switch style checks that fail when a poke has not arrived in time.
- Provide a local CLI for status review, configuration help, and helper commands.
- Store config and state in standard dotfile-friendly locations.
- Keep installation and ongoing maintenance lightweight.
- Retain history and logs with bounded, configurable limits so the tool never grows data without limit.

## Non-Goals

- Multi-host monitoring.
- Hosted service dependencies.
- Off-system paging, email, SMS, push, or webhooks.
- Rich dashboards or time-series analytics.
- Complex policy engines, acknowledgements, or escalation workflows.
- Cross-platform support in the initial release.
- A TUI in v0.

## Target User

Primary user: a technically competent Mac user who regularly works in interactive shells on the monitored machine and wants reliable reminders about local operational obligations without deploying a full monitoring stack.

## Product Principles

- Local first: no network service is required for core operation.
- Scriptable over abstract: monitors should be easy to write in shell, Python, or any executable language.
- Visible where work happens: shell and desktop surfaces matter more than web UIs.
- Quiet when healthy: healthy state should be compact and unobtrusive.
- Bounded by default: every log, history file, cache, and retained artifact must have a retention or size policy.
- File first: prefer plain files over embedded databases unless correctness or usability clearly demands otherwise.

## User Experience

### Primary Signals

- Shell startup banner.
  - On interactive shell launch, print a compact summary based on cached state.
  - Default behavior should be configurable between a count summary and a short list of failing monitor names.
  - Example healthy banner: `hostward: 6 ok`
  - Example failing banner: `hostward: failing: backup-nightly, photos-rsync`
- Prompt integration.
  - Shell integration should update a single shell variable before each prompt render: `HOSTWARD_FAILING_COUNT`.
  - Users own prompt formatting. Hostward only provides the count in a way that matches normal shell customization patterns.
  - Prompt integration must read cached state and never run monitors inline.
- macOS notification.
  - GUI notifications are optional.
  - Default GUI behavior should be one notification when a monitor first transitions from non-failing to failing.
  - Repeated notifications for an unchanged failure should not be sent by default.

### Operator Interface

- CLI for scripting, setup, and quick status checks.

## Core Concepts

### Monitor Types

#### Script Check

A scheduled executable that returns status to Hostward.

Expected behavior:

- Exit `0` for OK.
- Exit non-zero for failing.
- Stdout and stderr are captured as monitor output and become the failure payload when the monitor fails.

Examples:

- File mtime is newer than threshold.
- Directory size is below limit.
- `rsync` last-run marker exists and is fresh.

#### Dead Man's Switch

A named expectation that must be poked before a deadline.

Expected behavior:

- External process or user runs a `poke` command.
- Hostward records the poke timestamp.
- If time since last poke exceeds the configured threshold, state becomes failing.

Examples:

- Backup job must poke every 24 hours.
- A manual maintenance routine must be completed weekly.

### Monitor Definition Layout

Configuration should be split between one top-level config file and per-monitor files.

Suggested paths:

- Global config: `~/.config/hostward/config.toml`
- Monitor definitions: `~/.config/hostward/monitors/*.toml`
- State: `~/.local/state/hostward/`
- Cache: `~/.cache/hostward/`

Monitor naming rules:

- A monitor file defines one monitor.
- The file name without the extension is the default monitor name.
- A monitor may override that default with an explicit configured name.

### Monitor State

Initial state model:

- `ok`
- `failing`
- `unknown`
- `disabled`

Recommended conditions:

- `ok`
  - Monitor is enabled.
  - Latest evaluation completed successfully.
  - Script monitor exited `0`, or dead man's switch is still within its allowed interval.
- `failing`
  - Monitor is enabled.
  - Latest evaluation completed and found a problem.
  - Includes non-zero script exit, execution failure, timeout, or overdue dead man's switch.
- `unknown`
  - Monitor is enabled, but Hostward does not yet have enough trustworthy information to classify it as OK or failing.
  - Examples: monitor has never run, dead man's switch has never been poked, or required state is missing after recovery.
- `disabled`
  - Monitor is explicitly disabled by the operator.
  - Disabled monitors do not contribute to failing counts, scheduling, or notifications.

Suggested metadata per monitor:

- Name
- Type
- Status
- Last check time
- Last success time
- Last failure time
- Last failure output
- Last notification time
- Threshold or schedule settings
- Definition file path

### Failure Record Shape

Every active or historical failure must be traceable without guesswork.

Required fields:

- `monitor_name`: stable monitor identifier
- `definition_ref`: reference to where the monitor is defined, at minimum the config file path
- `payload`: monitor-provided failure output

Recommended additional fields:

- `status`
- `started_at`
- `last_seen_at`
- `last_changed_at`

Example conceptual shape:

```json
{
  "monitor_name": "backup-nightly",
  "definition_ref": {
    "path": "~/.config/hostward/monitors/backup-nightly.toml"
  },
  "payload": {
    "stdout": "",
    "stderr": "backup overdue by 11h"
  }
}
```

## Functional Requirements

### Scheduling

- Support scheduled execution of script monitors on fixed intervals.
- Support dead man's switch deadline evaluation on fixed intervals.
- Scheduling must persist across shell sessions and not depend on an active terminal.
- On startup, the scheduler should reconcile missed runs conservatively rather than assuming success.

### Status Evaluation

- Each monitor must have a current status and last evaluation timestamp.
- Status transitions should be recorded in a single local history log.
- History retention must be configurable and default to `31d`.
- No managed history or state data may grow without a retention or pruning rule.
- Failure detail comes from captured monitor output rather than internal severity classes.
- For shell and notification purposes, `failing` is the primary alerting state. `unknown` should remain visible in CLI output but should not be conflated with `failing`.

### Notifications

- Emit a GUI notification when a monitor first transitions into failing state, if GUI notifications are enabled.
- Default notification behavior should be one notification on failure start.
- Do not resend notifications for an unchanged failing condition by default.
- Terminal output and shell surfaces are the primary alert channel; GUI notifications are secondary.

### Shell Integration

- Provide a `shell-init` command that installs shell hooks and environment updates in a shell-native way.
- For zsh, recommend adding the init hook to `~/.zshrc`, which is the practical place for interactive shell behavior on macOS.
- For bash, recommend `~/.bashrc`.
- The shell init path should:
  - print a startup banner for interactive shells
  - refresh `HOSTWARD_FAILING_COUNT` before each prompt
  - avoid slow work by reading cached state only
- Banner mode should be configurable between count summary and failing-name list.

### Configuration

- Store configuration in standard per-user paths.
- Use TOML for the global config and each per-monitor definition.
- Support configuration for banner mode, log level, log destination, history retention, and notification behavior.

### Monitor Authoring

- Provide CLI helpers to scaffold common monitor types.
- Make the simplest path "drop in a script and declare its schedule."
- Ship a small set of built-in monitor types for common local checks.

Initial built-in monitors should include a practical subset of these:

- File freshness
- Dead man's switch
- Directory size
- Filesystem free space
- Rsync freshness
- File exists
- Directory exists
- Process running
- Command succeeds
- File count or directory growth

### CLI

Initial CLI commands:

- `hostward status`
- `hostward monitors list`
- `hostward monitor show <name>`
- `hostward monitor run <name>`
- `hostward poke <name>`
- `hostward enable <name>`
- `hostward disable <name>`
- `hostward add script`
- `hostward add deadman`
- `hostward shell-init <shell>`
- `hostward doctor`

### Logging

- Operational logs must be written as JSON Lines.
- Logging must be configurable, including at minimum level and destination.
- Supported log levels should include `error`, `warn`, `info`, and `debug`.
- Use the normal macOS-style per-user state area for default log location rather than inventing a strange layout.
- Human-facing monitor output and operator-facing logs are distinct; logs are for tool behavior, not a substitute for failure state.

## Non-Functional Requirements

- macOS only for v1.
- Low idle resource usage.
- Prompt-related shell hooks should feel instant.
- State corruption should be recoverable; no single failure should brick the tool.
- Installation should not require root for basic per-user operation.
- All data growth must be bounded by explicit retention or rotation policy.

## Proposed Architecture

### Local Components

- Config loader and validator.
- Scheduler and monitor runner.
- State store.
- History log writer.
- Notification adapter for macOS.
- CLI front end.
- Shell integration helpers.

### Storage Model

- Config in TOML files.
- One current-state file for fast reads by shell and CLI.
- One history log in JSONL with retention pruning.
- One operational log in JSONL.

Start with flat files unless they prove insufficient.

Suggested first cut:

- Global config: TOML
- Monitor definitions: one TOML file per monitor
- Current state: compact shared JSON state file
- History: JSONL
- Tool logs: JSONL

SQLite should be introduced only if flat files become materially awkward for correctness, performance, or operator experience.

### Scheduler Options

Option 1: one `launchd` job per monitor.
This is simple in spirit, but it creates a pile of generated jobs, makes shared state coordination clumsy, and is awkward for dead man's switch evaluation.

Option 2: one long-running Hostward scheduler started manually by the user.
This is simple in code, but process supervision becomes your problem.

Option 3: one per-user `launchd` agent that runs Hostward as a lightweight long-running scheduler.
This is the preferred option. It is the boring macOS-native approach, survives shell exits, centralizes scheduling and state updates, and avoids managing a forest of `launchd` jobs.

## Example Monitor Definitions

### File Freshness

- Check: file has been touched within `4h`
- Type: built-in file freshness monitor or script monitor
- Failing output: file age exceeds threshold

### Dead Man's Switch

- Expectation: `backup-nightly` must be poked every `24h`
- Poke source: backup script calls `hostward poke backup-nightly`

### Rsync Backup Freshness

- Check: marker file or log entry updated after successful rsync
- Failing output: no successful sync within threshold

### Directory Growth

- Check: directory or filesystem size below configured limit
- Failing output: current size exceeds threshold

## Scope for Initial Release

### In Scope for v0

- Per-user install on macOS from the repo.
- One per-user `launchd` agent running the scheduler.
- Script monitors on interval.
- Dead man's switch monitors.
- Local state store, history log, and operational log.
- Shell startup banner.
- Prompt variable integration.
- macOS notifications.
- Basic CLI.
- Scaffold helpers for new monitors.
- A practical set of built-in monitors.

### Out of Scope for v0

- TUI.
- Machine-readable plugin API beyond "run an executable."
- Remote sync of monitor state.
- Email, SMS, webhooks, or hosted services.
- Team or multi-user features.
- Automatic remediation actions.
- Homebrew packaging in the first cut.

## Risks and Tradeoffs

- Prompt integration becomes obnoxious immediately if it is slow; cached state is mandatory.
- A long-running agent is the right operational model on macOS, but it adds lifecycle details and failure modes that must be handled cleanly.
- Supporting arbitrary scripts is flexible, but the contract has to stay strict: exit code plus captured stdout and stderr.
- Flat-file state keeps the system inspectable, but atomic writes and pruning need careful design to avoid partial writes or stale reads.

## Open Questions

- Should the startup banner default to count mode or failing-name mode?
- What install path and build tooling do we want for the first implementation from source?

## Recommendation

Build the first version around a per-user `launchd` agent, TOML config, flat-file state, JSONL history and logs, simple executable checks, dead man's switch deadlines, and shell-first UX. The product wins if adding a new monitor feels closer to adding a cron-friendly script than adopting a monitoring platform, and if every artifact it writes stays inspectable and bounded.
