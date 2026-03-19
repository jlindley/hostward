package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"hostward/internal/config"
	"hostward/internal/launchd"
	"hostward/internal/state"
)

const doctorSnapshotFreshThreshold = 2 * time.Minute

type doctorLine struct {
	label string
	value string
}

func buildDoctorReport(paths config.Paths, now time.Time) (string, error) {
	lines := []doctorLine{
		{label: "config", value: fmt.Sprintf("%s (%s)", paths.GlobalConfigPath, presence(paths.GlobalConfigPath))},
		{label: "monitors", value: fmt.Sprintf("%s (%s)", paths.MonitorsDir, presence(paths.MonitorsDir))},
		{label: "runtime state", value: fmt.Sprintf("%s (%s)", paths.RuntimeStatePath, presence(paths.RuntimeStatePath))},
		{label: "current state", value: fmt.Sprintf("%s (%s)", paths.CurrentStatePath, presence(paths.CurrentStatePath))},
		{label: "history log", value: fmt.Sprintf("%s (%s)", paths.HistoryLogPath, presence(paths.HistoryLogPath))},
		{label: "operational log", value: fmt.Sprintf("%s (%s)", paths.OperationalLogPath, presence(paths.OperationalLogPath))},
		{label: "launch agent", value: fmt.Sprintf("%s (%s)", paths.LaunchAgentPath, presence(paths.LaunchAgentPath))},
	}

	bundle, err := config.Load(paths)
	if err != nil {
		lines = append(lines, doctorLine{label: "config validation", value: "invalid"})
		lines = append(lines, doctorLine{label: "configuration error", value: err.Error()})
		return formatDoctorReport(lines), fmt.Errorf("configuration invalid: %w", err)
	}

	lines = append(lines,
		doctorLine{label: "config validation", value: "ok"},
		doctorLine{label: "banner mode", value: bundle.Global.BannerMode},
		doctorLine{label: "log level", value: bundle.Global.LogLevel},
		doctorLine{label: "log retention", value: bundle.Global.LogRetention.String()},
		doctorLine{label: "log max bytes", value: fmt.Sprintf("%d", bundle.Global.LogMaxBytes)},
		doctorLine{label: "monitors loaded", value: fmt.Sprintf("%d", len(bundle.Monitors))},
		doctorLine{label: "notifications enabled", value: fmt.Sprintf("%t", bundle.Global.Notifications.Enabled)},
		doctorLine{label: "notification mode", value: bundle.Global.Notifications.Mode},
		doctorLine{label: "shell init zsh", value: shellInitStatus(filepath.Join(paths.Home, ".zshrc"), "zsh")},
		doctorLine{label: "shell init bash", value: shellInitStatus(filepath.Join(paths.Home, ".bashrc"), "bash")},
	)

	lines = append(lines, doctorLine{label: "snapshot cache", value: snapshotStatus(paths.CurrentStatePath, now)})

	launchdStatus, launchdErr := launchd.LoadStatus(paths, launchd.Label)
	switch {
	case launchdErr != nil:
		lines = append(lines, doctorLine{label: "launchd status", value: "error: " + launchdErr.Error()})
	default:
		lines = append(lines, doctorLine{label: "launchd loaded", value: fmt.Sprintf("%t", launchdStatus.Loaded)})
		if launchdStatus.Details != "" {
			lines = append(lines, doctorLine{label: "launchd detail", value: launchdStatus.Details})
		}
	}

	lines = append(lines, doctorLine{label: "notification adapter", value: notificationAdapterStatus(bundle.Global)})

	return formatDoctorReport(lines), nil
}

func formatDoctorReport(lines []doctorLine) string {
	var builder strings.Builder
	builder.WriteString("hostward doctor\n")
	for _, line := range lines {
		builder.WriteString(line.label)
		builder.WriteString(": ")
		builder.WriteString(line.value)
		builder.WriteByte('\n')
	}
	return builder.String()
}

func shellInitStatus(path, target string) string {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return fmt.Sprintf("not detected (%s missing)", path)
	}
	if err != nil {
		return fmt.Sprintf("unreadable (%s)", path)
	}

	text := string(data)
	markers := []string{
		fmt.Sprintf("hostward shell-init %s", target),
		"HOSTWARD_FAILING_COUNT",
	}
	switch target {
	case "zsh":
		markers = append(markers, "__hostward_precmd")
	case "bash":
		markers = append(markers, "__hostward_prompt_command")
	}

	for _, marker := range markers {
		if strings.Contains(text, marker) {
			return fmt.Sprintf("configured in %s", path)
		}
	}

	return fmt.Sprintf("not detected in %s", path)
}

func snapshotStatus(path string, now time.Time) string {
	snapshot, err := state.LoadSnapshot(path)
	if err != nil {
		return "unreadable: " + err.Error()
	}
	if snapshot.GeneratedAt.IsZero() {
		if presence(path) == "present" {
			return "invalid (missing generated_at)"
		}
		return "missing"
	}

	age := now.Sub(snapshot.GeneratedAt).Round(time.Second)
	if age <= doctorSnapshotFreshThreshold {
		return fmt.Sprintf("fresh (%s old)", age)
	}

	return fmt.Sprintf("stale (%s old; last generated %s)", age, snapshot.GeneratedAt.Format(time.RFC3339))
}

func notificationAdapterStatus(cfg config.Config) string {
	if !cfg.Notifications.Enabled {
		return "disabled in config"
	}

	path, err := exec.LookPath("osascript")
	if err != nil {
		return "osascript not found in PATH"
	}

	return fmt.Sprintf("osascript available at %s", path)
}
