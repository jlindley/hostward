package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"hostward/internal/fileio"
	"hostward/internal/monitor"
)

type Store struct {
	UpdatedAt time.Time         `json:"updated_at,omitempty"`
	Monitors  map[string]Record `json:"monitors,omitempty"`
}

type Record struct {
	Status        monitor.Status `json:"status,omitempty"`
	Summary       string         `json:"summary,omitempty"`
	LastCheckAt   *time.Time     `json:"last_check_at,omitempty"`
	LastChangeAt  *time.Time     `json:"last_change_at,omitempty"`
	LastSuccessAt *time.Time     `json:"last_success_at,omitempty"`
	LastFailureAt *time.Time     `json:"last_failure_at,omitempty"`
	LastPokeAt    *time.Time     `json:"last_poke_at,omitempty"`
	FailureStdout string         `json:"failure_stdout,omitempty"`
	FailureStderr string         `json:"failure_stderr,omitempty"`
}

func LoadStore(path string) (Store, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Store{Monitors: map[string]Record{}}, nil
	}
	if err != nil {
		return Store{}, fmt.Errorf("read runtime state %s: %w", path, err)
	}

	var store Store
	if err := json.Unmarshal(data, &store); err != nil {
		return Store{}, fmt.Errorf("decode runtime state %s: %w", path, err)
	}
	if store.Monitors == nil {
		store.Monitors = map[string]Record{}
	}

	return store, nil
}

func WriteStore(path string, store Store) error {
	if store.Monitors == nil {
		store.Monitors = map[string]Record{}
	}
	if store.UpdatedAt.IsZero() {
		store.UpdatedAt = time.Now().UTC()
	}

	data, err := json.Marshal(store)
	if err != nil {
		return fmt.Errorf("marshal runtime state: %w", err)
	}

	return fileio.AtomicWriteFile(path, data, 0o644)
}

func BuildSnapshot(definitions []monitor.Definition, store Store, now time.Time) Snapshot {
	snapshot := Snapshot{
		GeneratedAt: now,
		Monitors:    make([]MonitorSnapshot, 0, len(definitions)),
	}

	for _, definition := range definitions {
		record := store.Monitors[definition.ID]
		status, summary := ResolveStatus(definition, record, now)
		snapshot.Monitors = append(snapshot.Monitors, MonitorSnapshot{
			ID:            definition.ID,
			Name:          definition.DisplayName(),
			Status:        status,
			Summary:       summary,
			LastCheckAt:   record.LastCheckAt,
			LastChangeAt:  record.LastChangeAt,
			LastSuccessAt: record.LastSuccessAt,
			LastFailureAt: record.LastFailureAt,
		})
	}

	snapshot.Normalize()
	return snapshot
}

func ResolveStatus(definition monitor.Definition, record Record, now time.Time) (monitor.Status, string) {
	if definition.Disabled {
		return monitor.StatusDisabled, "disabled"
	}

	switch definition.Type {
	case monitor.TypeScript:
		if record.LastCheckAt == nil {
			return monitor.StatusUnknown, "never run"
		}
		if record.Status == monitor.StatusFailing {
			return monitor.StatusFailing, fallbackFailureSummary(record)
		}
		if record.Status == monitor.StatusOK {
			if record.Summary != "" {
				return monitor.StatusOK, record.Summary
			}
			return monitor.StatusOK, "last run succeeded"
		}
		if record.Status == monitor.StatusUnknown {
			if record.Summary != "" {
				return monitor.StatusUnknown, record.Summary
			}
			return monitor.StatusUnknown, "status unknown"
		}

		return monitor.StatusUnknown, "status unknown"
	case monitor.TypeDeadman:
		if record.LastPokeAt == nil {
			return monitor.StatusUnknown, "awaiting first poke"
		}

		age := now.Sub(*record.LastPokeAt).Round(time.Second)
		if definition.Deadman != nil && age > definition.Deadman.Grace {
			return monitor.StatusFailing, fmt.Sprintf("last poke %s ago exceeds %s", age, definition.Deadman.Grace)
		}

		return monitor.StatusOK, fmt.Sprintf("last poke %s ago", age)
	default:
		return monitor.StatusUnknown, "unsupported monitor type"
	}
}

func fallbackFailureSummary(record Record) string {
	if record.Summary != "" {
		return record.Summary
	}
	if record.FailureStderr != "" {
		return record.FailureStderr
	}
	if record.FailureStdout != "" {
		return record.FailureStdout
	}
	return "monitor failing"
}
