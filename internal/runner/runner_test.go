package runner

import (
	"strings"
	"testing"
	"time"

	"hostward/internal/monitor"
)

func TestRunScriptSuccess(t *testing.T) {
	result, err := RunScript(t.TempDir(), monitor.Definition{
		ID:   "ok",
		Type: monitor.TypeScript,
		Script: &monitor.ScriptConfig{
			Command:        []string{"sh", "-c", "echo ok"},
			Timeout:        5 * time.Second,
			InheritEnv:     true,
			MaxOutputBytes: 1024,
		},
	})
	if err != nil {
		t.Fatalf("RunScript() error = %v", err)
	}
	if result.Status != monitor.StatusOK {
		t.Fatalf("Status = %q, want ok", result.Status)
	}
	if result.Stdout != "ok" {
		t.Fatalf("Stdout = %q, want ok", result.Stdout)
	}
}

func TestRunScriptFailure(t *testing.T) {
	result, err := RunScript(t.TempDir(), monitor.Definition{
		ID:   "bad",
		Type: monitor.TypeScript,
		Script: &monitor.ScriptConfig{
			Command:        []string{"sh", "-c", "echo nope >&2; exit 7"},
			Timeout:        5 * time.Second,
			InheritEnv:     true,
			MaxOutputBytes: 1024,
		},
	})
	if err != nil {
		t.Fatalf("RunScript() error = %v", err)
	}
	if result.Status != monitor.StatusFailing {
		t.Fatalf("Status = %q, want failing", result.Status)
	}
	if !strings.Contains(result.Summary, "status 7") {
		t.Fatalf("Summary = %q, want status 7", result.Summary)
	}
	if result.Stderr != "nope" {
		t.Fatalf("Stderr = %q, want nope", result.Stderr)
	}
}

func TestRunScriptTimeout(t *testing.T) {
	result, err := RunScript(t.TempDir(), monitor.Definition{
		ID:   "slow",
		Type: monitor.TypeScript,
		Script: &monitor.ScriptConfig{
			Command:        []string{"sh", "-c", "sleep 1"},
			Timeout:        50 * time.Millisecond,
			InheritEnv:     true,
			MaxOutputBytes: 1024,
		},
	})
	if err != nil {
		t.Fatalf("RunScript() error = %v", err)
	}
	if result.Status != monitor.StatusFailing {
		t.Fatalf("Status = %q, want failing", result.Status)
	}
	if !strings.Contains(result.Summary, "timed out") {
		t.Fatalf("Summary = %q, want timeout", result.Summary)
	}
}
