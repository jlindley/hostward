package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hostward/internal/monitor"
)

func TestLoadUsesDefaultsWhenGlobalConfigMissing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	paths, err := DefaultPaths()
	if err != nil {
		t.Fatalf("DefaultPaths() error = %v", err)
	}

	bundle, err := Load(paths)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if bundle.Global.BannerMode != "count" {
		t.Fatalf("BannerMode = %q, want count", bundle.Global.BannerMode)
	}
	if bundle.Global.HistoryRetention != DefaultHistoryRetention {
		t.Fatalf("HistoryRetention = %v, want %v", bundle.Global.HistoryRetention, DefaultHistoryRetention)
	}
	if bundle.Global.LogMaxBytes != DefaultLogMaxBytes {
		t.Fatalf("LogMaxBytes = %d, want %d", bundle.Global.LogMaxBytes, DefaultLogMaxBytes)
	}
}

func TestLoadMonitorDefinitions(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))

	paths, err := DefaultPaths()
	if err != nil {
		t.Fatalf("DefaultPaths() error = %v", err)
	}

	if err := os.MkdirAll(paths.MonitorsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	globalConfig := `banner_mode = "list"
history_retention = "14d"
log_level = "debug"
log_retention = "7d"
log_max_bytes = 2048

[notifications]
enabled = false
mode = "failure-start"
`
	if err := os.WriteFile(paths.GlobalConfigPath, []byte(globalConfig), 0o644); err != nil {
		t.Fatalf("WriteFile(global config) error = %v", err)
	}

	scriptMonitor := `type = "script"
every = "5m"
timeout = "45s"
command = ["sh", "-c", "exit 0"]
max_output_bytes = 4096
working_dir = "/tmp"
`
	if err := os.WriteFile(filepath.Join(paths.MonitorsDir, "backup.toml"), []byte(scriptMonitor), 0o644); err != nil {
		t.Fatalf("WriteFile(script monitor) error = %v", err)
	}

	deadmanMonitor := `type = "deadman"
every = "1m"
grace = "24h"
name = "Nightly Backup"
disabled = true
`
	if err := os.WriteFile(filepath.Join(paths.MonitorsDir, "deadman.toml"), []byte(deadmanMonitor), 0o644); err != nil {
		t.Fatalf("WriteFile(deadman monitor) error = %v", err)
	}

	bundle, err := Load(paths)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if bundle.Global.BannerMode != "list" {
		t.Fatalf("BannerMode = %q, want list", bundle.Global.BannerMode)
	}
	if bundle.Global.HistoryRetention != 14*24*time.Hour {
		t.Fatalf("HistoryRetention = %v, want 14d", bundle.Global.HistoryRetention)
	}
	if bundle.Global.LogLevel != "debug" {
		t.Fatalf("LogLevel = %q, want debug", bundle.Global.LogLevel)
	}
	if bundle.Global.LogMaxBytes != 2048 {
		t.Fatalf("LogMaxBytes = %d, want 2048", bundle.Global.LogMaxBytes)
	}
	if bundle.Global.Notifications.Enabled {
		t.Fatalf("Notifications.Enabled = true, want false")
	}

	if len(bundle.Monitors) != 2 {
		t.Fatalf("len(Monitors) = %d, want 2", len(bundle.Monitors))
	}

	if bundle.Monitors[0].Type != monitor.TypeScript {
		t.Fatalf("Monitors[0].Type = %q, want script", bundle.Monitors[0].Type)
	}
	if bundle.Monitors[0].Script == nil || bundle.Monitors[0].Script.Timeout != 45*time.Second {
		t.Fatalf("Monitors[0].Script.Timeout = %v, want 45s", bundle.Monitors[0].Script.Timeout)
	}
	if bundle.Monitors[1].Type != monitor.TypeDeadman {
		t.Fatalf("Monitors[1].Type = %q, want deadman", bundle.Monitors[1].Type)
	}
	if bundle.Monitors[1].DisplayName() != "Nightly Backup" {
		t.Fatalf("Monitors[1].DisplayName() = %q, want Nightly Backup", bundle.Monitors[1].DisplayName())
	}
	if !bundle.Monitors[1].Disabled {
		t.Fatalf("Monitors[1].Disabled = false, want true")
	}
}

func TestLoadRejectsUnknownKeys(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))

	paths, err := DefaultPaths()
	if err != nil {
		t.Fatalf("DefaultPaths() error = %v", err)
	}

	if err := os.MkdirAll(paths.MonitorsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	monitorConfig := `type = "script"
every = "5m"
command = ["true"]
surprise = "nope"
`
	if err := os.WriteFile(filepath.Join(paths.MonitorsDir, "bad.toml"), []byte(monitorConfig), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if _, err := Load(paths); err == nil {
		t.Fatalf("Load() error = nil, want error")
	}
}

func TestParseDurationSupportsDaysAndWeeks(t *testing.T) {
	got, err := ParseDuration("1w2d3h15m")
	if err != nil {
		t.Fatalf("ParseDuration() error = %v", err)
	}

	want := 9*24*time.Hour + 3*time.Hour + 15*time.Minute
	if got != want {
		t.Fatalf("ParseDuration() = %v, want %v", got, want)
	}
}

func TestSetMonitorDisabledRewritesMonitorFile(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))

	paths, err := DefaultPaths()
	if err != nil {
		t.Fatalf("DefaultPaths() error = %v", err)
	}
	if err := os.MkdirAll(paths.MonitorsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	monitorConfig := `type = "script"
every = "5m"
command = ["true"]
`
	monitorPath := filepath.Join(paths.MonitorsDir, "disk.toml")
	if err := os.WriteFile(monitorPath, []byte(monitorConfig), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	definition, err := SetMonitorDisabled(paths, "disk", true)
	if err != nil {
		t.Fatalf("SetMonitorDisabled(true) error = %v", err)
	}
	if !definition.Disabled {
		t.Fatalf("Disabled = false, want true")
	}

	data, err := os.ReadFile(monitorPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) == monitorConfig || !strings.Contains(string(data), "disabled = true") {
		t.Fatalf("rewritten monitor missing disabled flag: %s", string(data))
	}

	definition, err = SetMonitorDisabled(paths, "disk", false)
	if err != nil {
		t.Fatalf("SetMonitorDisabled(false) error = %v", err)
	}
	if definition.Disabled {
		t.Fatalf("Disabled = true, want false")
	}

	data, err = os.ReadFile(monitorPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if strings.Contains(string(data), "disabled = true") {
		t.Fatalf("rewritten monitor still contains disabled flag: %s", string(data))
	}
}
