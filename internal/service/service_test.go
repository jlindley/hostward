package service

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hostward/internal/config"
	"hostward/internal/monitor"
	"hostward/internal/notify"
)

func TestRunMonitorUpdatesStateAndSnapshot(t *testing.T) {
	paths := testPaths(t)
	writeMonitor(t, paths.MonitorsDir, "script-check.toml", `type = "script"
every = "1m"
timeout = "5s"
command = ["sh", "-c", "echo fail >&2; exit 3"]
`)

	svc := New(paths)
	snapshot, err := svc.RunMonitor("script-check")
	if err != nil {
		t.Fatalf("RunMonitor() error = %v", err)
	}

	if snapshot.Status != monitor.StatusFailing {
		t.Fatalf("Status = %q, want failing", snapshot.Status)
	}

	storeData, err := os.ReadFile(paths.RuntimeStatePath)
	if err != nil {
		t.Fatalf("ReadFile(runtime state) error = %v", err)
	}
	if !strings.Contains(string(storeData), "status 3") {
		t.Fatalf("runtime state missing exit status: %s", string(storeData))
	}

	historyData, err := os.ReadFile(paths.HistoryLogPath)
	if err != nil {
		t.Fatalf("ReadFile(history) error = %v", err)
	}
	if !strings.Contains(string(historyData), "\"script-check\"") {
		t.Fatalf("history missing monitor id: %s", string(historyData))
	}

	logData, err := os.ReadFile(paths.OperationalLogPath)
	if err != nil {
		t.Fatalf("ReadFile(log) error = %v", err)
	}
	if !strings.Contains(string(logData), "monitor run completed") {
		t.Fatalf("operational log missing run entry: %s", string(logData))
	}

	snapshotData, err := os.ReadFile(paths.CurrentStatePath)
	if err != nil {
		t.Fatalf("ReadFile(snapshot) error = %v", err)
	}
	if !strings.Contains(string(snapshotData), "\"failing_count\":1") {
		t.Fatalf("snapshot missing failing count: %s", string(snapshotData))
	}
}

func TestPokeMonitorTransitionsDeadmanToOK(t *testing.T) {
	paths := testPaths(t)
	writeMonitor(t, paths.MonitorsDir, "backup.toml", `type = "deadman"
every = "1m"
grace = "24h"
`)

	svc := New(paths)
	snapshot, err := svc.PokeMonitor("backup")
	if err != nil {
		t.Fatalf("PokeMonitor() error = %v", err)
	}

	if snapshot.Status != monitor.StatusOK {
		t.Fatalf("Status = %q, want ok", snapshot.Status)
	}

	bundle, _, builtSnapshot, err := svc.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if len(bundle.Monitors) != 1 {
		t.Fatalf("len(Monitors) = %d, want 1", len(bundle.Monitors))
	}
	if builtSnapshot.StatusCounts.OK != 1 {
		t.Fatalf("OK count = %d, want 1", builtSnapshot.StatusCounts.OK)
	}

	logData, err := os.ReadFile(paths.OperationalLogPath)
	if err != nil {
		t.Fatalf("ReadFile(log) error = %v", err)
	}
	if !strings.Contains(string(logData), "deadman poke received") {
		t.Fatalf("operational log missing poke entry: %s", string(logData))
	}
}

func TestSetMonitorDisabledPersistsAndUpdatesSnapshot(t *testing.T) {
	paths := testPaths(t)
	writeMonitor(t, paths.MonitorsDir, "backup.toml", `type = "deadman"
every = "1m"
grace = "24h"
`)

	svc := New(paths)
	snapshot, err := svc.SetMonitorDisabled("backup", true)
	if err != nil {
		t.Fatalf("SetMonitorDisabled(true) error = %v", err)
	}
	if snapshot.Status != monitor.StatusDisabled {
		t.Fatalf("Status = %q, want disabled", snapshot.Status)
	}

	data, err := os.ReadFile(filepath.Join(paths.MonitorsDir, "backup.toml"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(data), "disabled = true") {
		t.Fatalf("monitor file missing disabled flag: %s", string(data))
	}

	snapshot, err = svc.SetMonitorDisabled("backup", false)
	if err != nil {
		t.Fatalf("SetMonitorDisabled(false) error = %v", err)
	}
	if snapshot.Status != monitor.StatusUnknown {
		t.Fatalf("Status = %q, want unknown after re-enable before first poke", snapshot.Status)
	}
}

func TestRunMonitorSendsFailureStartNotification(t *testing.T) {
	paths := testPaths(t)
	writeMonitor(t, paths.MonitorsDir, "script-check.toml", `type = "script"
every = "1m"
timeout = "5s"
command = ["sh", "-c", "echo fail >&2; exit 3"]
`)

	var called bool
	var args []string
	svc := NewWithNotifier(paths, notify.NewWithRunner(func(cmd string, cmdArgs ...string) error {
		called = true
		args = append([]string{cmd}, cmdArgs...)
		return nil
	}))

	_, err := svc.RunMonitor("script-check")
	if err != nil {
		t.Fatalf("RunMonitor() error = %v", err)
	}
	if !called {
		t.Fatalf("expected notifier to be called")
	}
	if len(args) < 3 || args[0] != "osascript" || args[1] != "-e" {
		t.Fatalf("unexpected notifier call: %#v", args)
	}
}

func TestRunMonitorIgnoresNotificationDeliveryFailure(t *testing.T) {
	paths := testPaths(t)
	writeMonitor(t, paths.MonitorsDir, "script-check.toml", `type = "script"
every = "1m"
timeout = "5s"
command = ["sh", "-c", "echo fail >&2; exit 3"]
`)

	svc := NewWithNotifier(paths, notify.NewWithRunner(func(cmd string, cmdArgs ...string) error {
		return errors.New("osascript failed")
	}))

	snapshot, err := svc.RunMonitor("script-check")
	if err != nil {
		t.Fatalf("RunMonitor() error = %v", err)
	}
	if snapshot.Status != monitor.StatusFailing {
		t.Fatalf("Status = %q, want failing", snapshot.Status)
	}

	logData, err := os.ReadFile(paths.OperationalLogPath)
	if err != nil {
		t.Fatalf("ReadFile(log) error = %v", err)
	}
	if !strings.Contains(string(logData), "notification delivery failed") {
		t.Fatalf("operational log missing notification failure warning: %s", string(logData))
	}
}

func TestReconcileOncePrunesOperationalLogByAgeAndBytes(t *testing.T) {
	paths := testPaths(t)
	if err := os.WriteFile(paths.GlobalConfigPath, []byte(`log_retention = "1h"
log_max_bytes = 120
`), 0o644); err != nil {
		t.Fatalf("WriteFile(global config) error = %v", err)
	}
	writeMonitor(t, paths.MonitorsDir, "script-check.toml", `type = "script"
every = "1m"
timeout = "5s"
command = ["sh", "-c", "exit 0"]
`)

	old := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	newer := time.Now().UTC().Add(-5 * time.Minute).Format(time.RFC3339)
	logBody := strings.Join([]string{
		`{"at":"` + old + `","level":"info","message":"old"}`,
		`{"at":"` + newer + `","level":"info","message":"middle"}`,
		`{"at":"` + newer + `","level":"info","message":"new"}`,
	}, "\n") + "\n"
	if err := os.MkdirAll(filepath.Dir(paths.OperationalLogPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(log dir) error = %v", err)
	}
	if err := os.WriteFile(paths.OperationalLogPath, []byte(logBody), 0o644); err != nil {
		t.Fatalf("WriteFile(log) error = %v", err)
	}

	svc := New(paths)
	if _, err := svc.ReconcileOnce(time.Now().UTC()); err != nil {
		t.Fatalf("ReconcileOnce() error = %v", err)
	}

	data, err := os.ReadFile(paths.OperationalLogPath)
	if err != nil {
		t.Fatalf("ReadFile(log) error = %v", err)
	}
	text := string(data)
	if strings.Contains(text, `"message":"old"`) {
		t.Fatalf("pruned log still contains old entry: %s", text)
	}
	if len(data) > 120 {
		t.Fatalf("pruned log length = %d, want <= 120", len(data))
	}
}

func testPaths(t *testing.T) config.Paths {
	t.Helper()

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
		t.Fatalf("MkdirAll(monitors) error = %v", err)
	}

	return paths
}

func writeMonitor(t *testing.T, dir, name, body string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", name, err)
	}
}
