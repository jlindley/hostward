package history

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hostward/internal/monitor"
)

func TestAppendAndPruneHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.jsonl")
	oldAt := time.Now().UTC().Add(-48 * time.Hour)
	newAt := time.Now().UTC()

	if err := Append(path, Event{At: oldAt, MonitorID: "old", Status: monitor.StatusFailing}); err != nil {
		t.Fatalf("Append(old) error = %v", err)
	}
	if err := Append(path, Event{At: newAt, MonitorID: "new", Status: monitor.StatusOK}); err != nil {
		t.Fatalf("Append(new) error = %v", err)
	}

	if err := PruneOlderThan(path, time.Now().UTC().Add(-24*time.Hour)); err != nil {
		t.Fatalf("PruneOlderThan() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	text := string(data)
	if strings.Contains(text, "\"old\"") {
		t.Fatalf("pruned history still contains old event: %s", text)
	}
	if !strings.Contains(text, "\"new\"") {
		t.Fatalf("pruned history lost new event: %s", text)
	}
}

func TestPruneOlderThanSkipsMalformedAndLargeLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.jsonl")
	large := strings.Repeat("x", 70*1024)
	newAt := time.Now().UTC()

	payload := []byte("not json\n")
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := Append(path, Event{
		At:            newAt,
		MonitorID:     "large",
		Status:        monitor.StatusFailing,
		FailureStderr: large,
	}); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	if err := PruneOlderThan(path, time.Now().UTC().Add(-time.Hour)); err != nil {
		t.Fatalf("PruneOlderThan() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	text := string(data)
	if strings.Contains(text, "not json") {
		t.Fatalf("pruned history still contains malformed line")
	}
	if !strings.Contains(text, "\"large\"") {
		t.Fatalf("pruned history lost large event")
	}
}
