package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hostward/internal/config"
)

func TestAddScriptWritesMonitorFile(t *testing.T) {
	root := t.TempDir()
	paths := config.Paths{MonitorsDir: filepath.Join(root, "monitors")}

	path, err := AddScript(paths, ScriptOptions{
		ID:          "backup",
		DisplayName: "Nightly Backup",
		Every:       "5m",
		Command:     []string{"sh", "-c", "exit 0"},
	})
	if err != nil {
		t.Fatalf("AddScript() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	text := string(data)
	if !strings.Contains(text, `name = "Nightly Backup"`) {
		t.Fatalf("script scaffold missing display name: %s", text)
	}
	if !strings.Contains(text, `command = ["sh", "-c", "exit 0"]`) {
		t.Fatalf("script scaffold missing command: %s", text)
	}
}

func TestAddDeadmanWritesMonitorFile(t *testing.T) {
	root := t.TempDir()
	paths := config.Paths{MonitorsDir: filepath.Join(root, "monitors")}

	path, err := AddDeadman(paths, DeadmanOptions{
		ID:    "backup",
		Every: "5m",
		Grace: "24h",
	})
	if err != nil {
		t.Fatalf("AddDeadman() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	text := string(data)
	if !strings.Contains(text, `type = "deadman"`) || !strings.Contains(text, `grace = "24h"`) {
		t.Fatalf("deadman scaffold missing fields: %s", text)
	}
}

func TestAddFileExistsWritesTestCommand(t *testing.T) {
	root := t.TempDir()
	paths := config.Paths{MonitorsDir: filepath.Join(root, "monitors")}

	path, err := AddFileExists(paths, FileExistsOptions{
		ID:    "marker",
		Every: "5m",
		Path:  "/tmp/marker file",
	})
	if err != nil {
		t.Fatalf("AddFileExists() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(data), `test -e '/tmp/marker file'`) {
		t.Fatalf("file exists scaffold missing test command: %s", string(data))
	}
}

func TestAddFileFreshnessWritesStatCommand(t *testing.T) {
	root := t.TempDir()
	paths := config.Paths{MonitorsDir: filepath.Join(root, "monitors")}

	path, err := AddFileFreshness(paths, FileFreshnessOptions{
		ID:     "marker-fresh",
		Every:  "5m",
		Path:   "/tmp/marker",
		MaxAge: "4h",
	})
	if err != nil {
		t.Fatalf("AddFileFreshness() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(data), `stat -f %m '/tmp/marker'`) {
		t.Fatalf("file freshness scaffold missing stat command: %s", string(data))
	}
}

func TestAddFreeSpaceWritesDfCommand(t *testing.T) {
	root := t.TempDir()
	paths := config.Paths{MonitorsDir: filepath.Join(root, "monitors")}

	path, err := AddFreeSpace(paths, FreeSpaceOptions{
		ID:             "disk-free",
		Every:          "5m",
		Path:           "/",
		MinFreePercent: 10,
	})
	if err != nil {
		t.Fatalf("AddFreeSpace() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(data), `df -Pk '/'`) {
		t.Fatalf("free space scaffold missing df command: %s", string(data))
	}
}
