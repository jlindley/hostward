# Hostward Repo Instructions

## Toolchain

- Implementation language is Go.
- Run `gofmt -w` on changed Go files.
- Run `go test ./...` before closing work.
- Main CLI entrypoint is `cmd/hostward`.

## Product Invariants

- Hostward is local-only and single-user for one Mac.
- Shell startup and prompt integration must read cached state only.
- Do not run monitors inline from shell hooks.
- `shell-init` prints shell snippets. It does not edit user dotfiles.

## Runtime Model

- Background scheduling uses one per-user `launchd` agent.
- Do not create one `launchd` job per monitor unless explicitly asked.

## Persistence

- Config is TOML.
- Current state is a compact file for fast reads.
- History and operational logs are JSONL.
- Current-state writes must be atomic.
- State should stay human-inspectable.
- Do not introduce SQLite unless explicitly approved after flat files prove insufficient.

## Data Retention

- Every persistent artifact must have bounded retention or rotation.
- Do not add logs, caches, or state files that can grow without a pruning policy.

## Monitor Model

- Script monitors report status through exit code plus captured stdout/stderr.
- Do not add severity levels, acknowledgements, or escalation workflows unless explicitly requested.
- Treat config and on-disk state formats as user-facing contracts.
- If a change would break those contracts, add a migration or stop and ask.

## macOS Notifications

- Sticky notifications are not trivial from plain Go.
- If implementing notifications, document the mechanism and any platform limitations clearly instead of hand-waving.
