# Hostward

Hostward is local monitoring for one Mac.

It is for the small operational chores that are easy to forget and annoying to rebuild into a full monitoring stack: a cert that needs rotating, a backup job that should keep poking, a file that should stay fresh, a disk that should not fill up, a local process that should still be around.

Hostward runs checks on a schedule, stores state in plain files, and shows the result where you already look:

- a shell banner when you open a terminal
- a prompt count you can wire into your own prompt
- optional macOS notifications
- a local CLI

It is intentionally not fleet monitoring, not a hosted service, and not another dashboard.

Healthy shell banner:

```text
Last login: ...
hostward: 2 ok
you@mac ~ %
```

Failing shell banner:

```text
Last login: ...
hostward: failing: 1 of 2
you@mac ~ %
```

## What It Checks

Hostward has two core monitor types:

- `script`: run a command on a schedule; exit `0` means OK, non-zero means failing
- `deadman`: expect a periodic `hostward poke <name>` before a deadline

There are also built-in helpers for common local checks:

- `file-exists`
- `dir-exists`
- `file-freshness`
- `free-space`
- `dir-size`
- `process-running`

## Why It Exists

The usual pattern for local machine health is a pile of scripts, shell aliases, launch agents, calendar reminders, and good intentions. That works right up until it does not.

Hostward keeps the setup lightweight:

- local-only
- single-user
- flat-file config and state
- one per-user `launchd` agent for background scheduling
- cached reads for shell startup and prompt integration

The shell hook never runs checks inline. It reads cached state only, which is the difference between a useful tool and a self-inflicted annoyance.

## Install

Hostward is written in Go.

```bash
git clone https://github.com/jlindley/hostward.git
cd hostward
go build -o ~/.local/bin/hostward ./cmd/hostward
```

Requirements:

- macOS
- Go 1.25+

## Quick Start

Create the config and monitor directories:

```bash
mkdir -p ~/.config/hostward/monitors
mkdir -p ~/.local/state/hostward
mkdir -p ~/.cache/hostward
```

Optional global config:

```toml
# ~/.config/hostward/config.toml
banner_mode = "count"
history_retention = "31d"
log_level = "info"
log_retention = "31d"
log_max_bytes = 10485760

[notifications]
enabled = true
mode = "failure-start"
```

Add a couple of monitors:

```bash
hostward add free-space root-space --path / --min-free-percent 10 --every 15m
hostward add deadman backup-nightly --every 5m --grace 24h
```

Install the scheduler:

```bash
hostward launchd install --binary ~/.local/bin/hostward
hostward doctor
```

Enable shell integration:

```bash
eval "$(hostward shell-init zsh)"
```

Then check status:

```bash
hostward status
hostward monitors list
hostward monitor show backup-nightly
```

## Example Monitors

A script monitor:

```toml
type = "script"
every = "15m"
timeout = "10s"
command = ["sh", "-c", "test \"$(df -Pk / | awk 'NR==2 {print 100 - $5}')\" -ge 10"]
```

A deadman's switch:

```toml
type = "deadman"
every = "5m"
grace = "24h"
name = "Nightly Backup"
```

Your backup job or cron-friendly script would call:

```bash
hostward poke backup-nightly
```

## Commands You Will Actually Use

Setup and integration:

- `hostward launchd install --binary ~/.local/bin/hostward`
- `hostward launchd status`
- `hostward doctor`
- `hostward shell-init zsh`

Day-to-day:

- `hostward status`
- `hostward monitors list`
- `hostward monitor show <name>`
- `hostward monitor run <name>`
- `hostward poke <name>`

Authoring:

- `hostward add script`
- `hostward add deadman`
- `hostward add file-exists`
- `hostward add dir-exists`
- `hostward add file-freshness`
- `hostward add free-space`
- `hostward add dir-size`
- `hostward add process-running`

## Files and Data

Hostward follows XDG-style paths:

- config: `~/.config/hostward/`
- monitors: `~/.config/hostward/monitors/`
- runtime state: `~/.local/state/hostward/`
- cache: `~/.cache/hostward/`

Data stays human-inspectable:

- config is TOML
- current state is a compact cached file
- history and operational logs are JSONL

Logs and history have bounded retention. Hostward should not quietly grow junk forever.

## Notes

- Hostward is built for one machine, not a cluster.
- `shell-init` prints shell snippets. It does not edit your dotfiles.
- macOS notifications are best-effort, not guaranteed sticky alerts.
- If you want shell or Python checks, write them that way. Hostward does not care as long as it can execute them.

## Development

The main CLI entrypoint is [`cmd/hostward`](cmd/hostward).

Useful files and docs:

- [`PRD.md`](PRD.md)
- [`docs/implementation-plan.md`](docs/implementation-plan.md)
- [`examples/config.toml`](examples/config.toml)
- [`examples/monitors`](examples/monitors)

Before closing changes:

- run `gofmt -w` on changed Go files
- run `go test ./...`
