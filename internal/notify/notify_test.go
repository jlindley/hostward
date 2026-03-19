package notify

import (
	"strings"
	"testing"

	"hostward/internal/config"
	"hostward/internal/monitor"
)

func TestNotifyFailureStartRunsOsascript(t *testing.T) {
	var name string
	var args []string
	notifier := NewWithRunner(func(cmd string, cmdArgs ...string) error {
		name = cmd
		args = cmdArgs
		return nil
	})

	err := notifier.NotifyFailureStart(config.DefaultConfig(), monitor.Definition{
		ID:   "backup",
		Name: "Nightly Backup",
	}, `backup overdue "badly"`)
	if err != nil {
		t.Fatalf("NotifyFailureStart() error = %v", err)
	}

	if name != "osascript" {
		t.Fatalf("cmd = %q, want osascript", name)
	}
	if len(args) != 2 || args[0] != "-e" {
		t.Fatalf("args = %#v, want -e <script>", args)
	}
	if !strings.Contains(args[1], `with title "Hostward failure"`) {
		t.Fatalf("script missing title: %s", args[1])
	}
	if !strings.Contains(args[1], `subtitle "Nightly Backup"`) {
		t.Fatalf("script missing subtitle: %s", args[1])
	}
}

func TestNotifyDisabledSkips(t *testing.T) {
	called := false
	notifier := NewWithRunner(func(cmd string, args ...string) error {
		called = true
		return nil
	})

	cfg := config.DefaultConfig()
	cfg.Notifications.Enabled = false
	if err := notifier.NotifyTest(cfg); err != nil {
		t.Fatalf("NotifyTest() error = %v", err)
	}
	if called {
		t.Fatalf("runner called when notifications disabled")
	}
}
