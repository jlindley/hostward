package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"hostward/internal/config"
	"hostward/internal/state"
)

func TestRunDoctorReportsLiveDiagnostics(t *testing.T) {
	runner, stdout, _, paths := testRunner(t)
	writeExecutable(t, filepath.Join(paths.Home, "bin", "launchctl"), `#!/bin/sh
if [ "$1" = "print" ]; then
  echo "service = com.hostward.scheduler"
  exit 0
fi
exit 0
`)
	writeExecutable(t, filepath.Join(paths.Home, "bin", "osascript"), "#!/bin/sh\nexit 0\n")

	if err := os.WriteFile(filepath.Join(paths.Home, ".zshrc"), []byte(`eval "$(hostward shell-init zsh)"`+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(.zshrc) error = %v", err)
	}
	if err := state.WriteSnapshot(paths.CurrentStatePath, state.Snapshot{
		GeneratedAt:  time.Now().UTC(),
		TotalCount:   1,
		FailingCount: 0,
	}); err != nil {
		t.Fatalf("WriteSnapshot() error = %v", err)
	}

	if err := runner.Run([]string{"doctor"}); err != nil {
		t.Fatalf("Run(doctor) error = %v", err)
	}

	text := stdout.String()
	assertContains(t, text, "config validation: ok")
	assertContains(t, text, "shell integration: advisory only; banner and prompt wiring are optional")
	assertContains(t, text, "shell zsh banner: advisory only: detected")
	assertContains(t, text, "shell zsh prompt env function: advisory only: detected")
	assertContains(t, text, "shell zsh prompt hook: advisory only: detected")
	assertContains(t, text, "shell bash banner: advisory only: not detected")
	assertContains(t, text, "shell bash prompt env function: advisory only: not detected")
	assertContains(t, text, "shell bash prompt hook: advisory only: not detected")
	assertContains(t, text, "snapshot cache: fresh")
	assertContains(t, text, "launchd loaded: true")
	assertContains(t, text, "notification adapter: osascript available at")
}

func TestRunDoctorReportsBannerOnlyShellSetup(t *testing.T) {
	runner, stdout, _, paths := testRunner(t)

	if err := os.WriteFile(filepath.Join(paths.Home, ".zshrc"), []byte(`if [[ -o interactive ]]; then
  "$HOME/.local/bin/hostward" banner 2>/dev/null
fi
`), 0o644); err != nil {
		t.Fatalf("WriteFile(.zshrc) error = %v", err)
	}

	if err := runner.Run([]string{"doctor"}); err != nil {
		t.Fatalf("Run(doctor) error = %v", err)
	}

	text := stdout.String()
	assertContains(t, text, "shell zsh banner: advisory only: detected")
	assertContains(t, text, "shell zsh prompt env function: advisory only: not detected")
	assertContains(t, text, "shell zsh prompt hook: advisory only: not detected")
}

func TestRunMonitorRunSucceedsWhenNotificationFails(t *testing.T) {
	runner, stdout, _, paths := testRunner(t)
	writeExecutable(t, filepath.Join(paths.Home, "bin", "osascript"), "#!/bin/sh\nexit 1\n")

	writeMonitor(t, paths.MonitorsDir, "script-check.toml", `type = "script"
every = "1m"
timeout = "5s"
command = ["sh", "-c", "echo fail >&2; exit 3"]
`)

	if err := runner.Run([]string{"monitor", "run", "script-check"}); err != nil {
		t.Fatalf("Run(monitor run) error = %v", err)
	}

	text := stdout.String()
	assertContains(t, text, "script-check: failing")
	assertContains(t, text, "command exited with status 3")

	logData, err := os.ReadFile(paths.OperationalLogPath)
	if err != nil {
		t.Fatalf("ReadFile(log) error = %v", err)
	}
	assertContains(t, string(logData), "notification delivery failed")
}

func TestRunStatusWritesSnapshot(t *testing.T) {
	runner, stdout, _, paths := testRunner(t)
	writeMonitor(t, paths.MonitorsDir, "backup.toml", `type = "deadman"
every = "1m"
grace = "24h"
`)

	if err := runner.Run([]string{"status"}); err != nil {
		t.Fatalf("Run(status) error = %v", err)
	}

	text := stdout.String()
	assertContains(t, text, "hostward: 1 unknown")
	assertContains(t, text, "configured monitors: 1")
	assertContains(t, text, "states: ok=0 failing=0 unknown=1 disabled=0")

	if _, err := os.Stat(paths.CurrentStatePath); err != nil {
		t.Fatalf("Stat(snapshot) error = %v", err)
	}
}

func TestRunShellInitPrintsSnippet(t *testing.T) {
	runner, stdout, _, _ := testRunner(t)

	if err := runner.Run([]string{"shell-init", "zsh"}); err != nil {
		t.Fatalf("Run(shell-init) error = %v", err)
	}

	text := stdout.String()
	assertContains(t, text, "__hostward_precmd")
	assertContains(t, text, "HOSTWARD_FAILING_COUNT")
}

func TestRunMonitorsListAlignsColumns(t *testing.T) {
	runner, stdout, _, paths := testRunner(t)

	writeMonitor(t, paths.MonitorsDir, "alpha.toml", `type = "script"
every = "1m"
timeout = "5s"
command = ["sh", "-c", "exit 0"]
name = "Alpha monitor"
`)
	writeMonitor(t, paths.MonitorsDir, "beta-long.toml", `type = "deadman"
every = "1m"
grace = "24h"
name = "Beta monitor"
`)

	if err := runner.Run([]string{"monitors", "list"}); err != nil {
		t.Fatalf("Run(monitors list) error = %v", err)
	}

	text := stdout.String()
	if strings.Contains(text, "\t") {
		t.Fatalf("monitors list output still contains tabs:\n%s", text)
	}
	assertMatches(t, text, `(?m)^alpha {2,}script {2,}unknown {2,}Alpha monitor$`)
	assertMatches(t, text, `(?m)^beta-long {2,}deadman {2,}unknown {2,}Beta monitor$`)
}

func testRunner(t *testing.T) (*Runner, *bytes.Buffer, *bytes.Buffer, config.Paths) {
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

	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(bin) error = %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	return &Runner{
		stdout: stdout,
		stderr: stderr,
		paths:  paths,
	}, stdout, stderr, paths
}

func writeMonitor(t *testing.T, dir, name, body string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", name, err)
	}
}

func writeExecutable(t *testing.T, path, body string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

func assertContains(t *testing.T, text, want string) {
	t.Helper()

	if !strings.Contains(text, want) {
		t.Fatalf("output missing %q:\n%s", want, text)
	}
}

func assertMatches(t *testing.T, text, pattern string) {
	t.Helper()

	if !regexp.MustCompile(pattern).MatchString(text) {
		t.Fatalf("output did not match %q:\n%s", pattern, text)
	}
}
