package state

import (
	"path/filepath"
	"testing"
	"time"

	"hostward/internal/monitor"
)

func TestSnapshotNormalizeCountsMonitorStates(t *testing.T) {
	snapshot := Snapshot{
		Monitors: []MonitorSnapshot{
			{ID: "backup", Status: monitor.StatusFailing},
			{ID: "rsync", Name: "Photos Sync", Status: monitor.StatusFailing},
			{ID: "freshness", Status: monitor.StatusUnknown},
			{ID: "disk", Status: monitor.StatusOK},
			{ID: "manual", Status: monitor.StatusDisabled},
		},
	}

	snapshot.Normalize()

	if snapshot.TotalCount != 4 {
		t.Fatalf("TotalCount = %d, want 4", snapshot.TotalCount)
	}
	if snapshot.FailingCount != 2 {
		t.Fatalf("FailingCount = %d, want 2", snapshot.FailingCount)
	}
	if snapshot.UnknownCount != 1 {
		t.Fatalf("UnknownCount = %d, want 1", snapshot.UnknownCount)
	}
	if snapshot.StatusCounts.Disabled != 1 {
		t.Fatalf("Disabled = %d, want 1", snapshot.StatusCounts.Disabled)
	}
	if len(snapshot.Failing) != 2 || snapshot.Failing[1] != "Photos Sync" {
		t.Fatalf("Failing = %#v, want backup and Photos Sync", snapshot.Failing)
	}
}

func TestWriteSnapshotRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "current-state.json")
	now := time.Now().UTC().Round(0)

	want := Snapshot{
		GeneratedAt: now,
		Monitors: []MonitorSnapshot{
			{
				ID:           "backup",
				Status:       monitor.StatusOK,
				LastCheckAt:  &now,
				LastChangeAt: &now,
			},
		},
	}

	if err := WriteSnapshot(path, want); err != nil {
		t.Fatalf("WriteSnapshot() error = %v", err)
	}

	got, err := LoadSnapshot(path)
	if err != nil {
		t.Fatalf("LoadSnapshot() error = %v", err)
	}

	if got.TotalCount != 1 || got.StatusCounts.OK != 1 {
		t.Fatalf("got counts = %#v, want one ok monitor", got)
	}
	if len(got.Monitors) != 1 || got.Monitors[0].ID != "backup" {
		t.Fatalf("got monitors = %#v, want backup", got.Monitors)
	}
}
