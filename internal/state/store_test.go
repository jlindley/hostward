package state

import (
	"path/filepath"
	"testing"
	"time"

	"hostward/internal/monitor"
)

func TestStoreRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "monitor-state.json")
	now := time.Now().UTC().Round(0)

	store := Store{
		UpdatedAt: now,
		Monitors: map[string]Record{
			"backup": {
				Status:      monitor.StatusFailing,
				Summary:     "backup overdue",
				LastCheckAt: &now,
				LastPokeAt:  &now,
			},
		},
	}

	if err := WriteStore(path, store); err != nil {
		t.Fatalf("WriteStore() error = %v", err)
	}

	got, err := LoadStore(path)
	if err != nil {
		t.Fatalf("LoadStore() error = %v", err)
	}

	if got.Monitors["backup"].Summary != "backup overdue" {
		t.Fatalf("Summary = %q, want backup overdue", got.Monitors["backup"].Summary)
	}
}

func TestBuildSnapshotResolvesDeadmanState(t *testing.T) {
	now := time.Now().UTC().Round(0)
	lastPoke := now.Add(-2 * time.Hour)

	snapshot := BuildSnapshot([]monitor.Definition{
		{
			ID:    "backup",
			Type:  monitor.TypeDeadman,
			Name:  "Nightly Backup",
			Every: time.Minute,
			Deadman: &monitor.DeadmanConfig{
				Grace: time.Hour,
			},
		},
	}, Store{
		Monitors: map[string]Record{
			"backup": {LastPokeAt: &lastPoke},
		},
	}, now)

	if snapshot.FailingCount != 1 {
		t.Fatalf("FailingCount = %d, want 1", snapshot.FailingCount)
	}
	if snapshot.Monitors[0].Status != monitor.StatusFailing {
		t.Fatalf("Status = %q, want failing", snapshot.Monitors[0].Status)
	}
}
