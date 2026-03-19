package scheduler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hostward/internal/config"
	"hostward/internal/monitor"
	"hostward/internal/service"
)

func TestRunOnceReconcilesScriptAndDeadman(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))

	paths, err := config.DefaultPaths()
	if err != nil {
		t.Fatalf("DefaultPaths() error = %v", err)
	}
	if err := os.MkdirAll(paths.MonitorsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	if err := os.WriteFile(filepath.Join(paths.MonitorsDir, "script.toml"), []byte(`type = "script"
every = "1m"
command = ["sh", "-c", "exit 0"]
`), 0o644); err != nil {
		t.Fatalf("WriteFile(script) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(paths.MonitorsDir, "backup.toml"), []byte(`type = "deadman"
every = "1m"
grace = "1h"
`), 0o644); err != nil {
		t.Fatalf("WriteFile(deadman) error = %v", err)
	}

	snapshot, err := Runner{
		Service: service.New(paths),
		Tick:    time.Second,
	}.RunOnce(time.Now().UTC())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	if snapshot.StatusCounts.OK != 1 {
		t.Fatalf("OK count = %d, want 1", snapshot.StatusCounts.OK)
	}
	if snapshot.StatusCounts.Unknown != 1 {
		t.Fatalf("Unknown count = %d, want 1", snapshot.StatusCounts.Unknown)
	}
	if snapshot.Monitors[0].Status != monitor.StatusUnknown && snapshot.Monitors[1].Status != monitor.StatusUnknown {
		t.Fatalf("expected one unknown monitor in snapshot %#v", snapshot.Monitors)
	}

	logData, err := os.ReadFile(paths.OperationalLogPath)
	if err != nil {
		t.Fatalf("ReadFile(log) error = %v", err)
	}
	if !strings.Contains(string(logData), "monitor run completed") {
		t.Fatalf("log missing monitor run entry: %s", string(logData))
	}
}
