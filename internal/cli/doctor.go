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

type shellCheck struct {
	label      string
	configured bool
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
		doctorLine{label: "shell integration", value: "advisory only; banner and prompt wiring are optional"},
	)
	lines = append(lines, shellInitLines(filepath.Join(paths.Home, ".zshrc"), "zsh")...)
	lines = append(lines, shellInitLines(filepath.Join(paths.Home, ".bashrc"), "bash")...)

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

func shellInitLines(path, target string) []doctorLine {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return missingShellInitLines(path, target)
	}
	if err != nil {
		return []doctorLine{
			{label: shellCheckLabel(target, "banner"), value: fmt.Sprintf("advisory only: unreadable (%s)", path)},
			{label: shellCheckLabel(target, "prompt env function"), value: fmt.Sprintf("advisory only: unreadable (%s)", path)},
			{label: shellCheckLabel(target, "prompt hook"), value: fmt.Sprintf("advisory only: unreadable (%s)", path)},
		}
	}

	text := string(data)
	checks := shellChecks(text, target)
	lines := make([]doctorLine, 0, len(checks))
	for _, check := range checks {
		value := fmt.Sprintf("advisory only: not detected in %s", path)
		if check.configured {
			value = fmt.Sprintf("advisory only: detected in %s", path)
		}
		lines = append(lines, doctorLine{
			label: shellCheckLabel(target, check.label),
			value: value,
		})
	}
	return lines
}

func missingShellInitLines(path, target string) []doctorLine {
	return []doctorLine{
		{label: shellCheckLabel(target, "banner"), value: fmt.Sprintf("advisory only: not detected (%s missing)", path)},
		{label: shellCheckLabel(target, "prompt env function"), value: fmt.Sprintf("advisory only: not detected (%s missing)", path)},
		{label: shellCheckLabel(target, "prompt hook"), value: fmt.Sprintf("advisory only: not detected (%s missing)", path)},
	}
}

func shellChecks(text, target string) []shellCheck {
	if containsAll(text, "shell-init "+target, "hostward") {
		return []shellCheck{
			{label: "banner", configured: true},
			{label: "prompt env function", configured: true},
			{label: "prompt hook", configured: true},
		}
	}

	switch target {
	case "zsh":
		return []shellCheck{
			{label: "banner", configured: containsAll(text, "banner", "hostward")},
			{label: "prompt env function", configured: strings.Contains(text, "__hostward_precmd") || strings.Contains(text, "HOSTWARD_FAILING_COUNT")},
			{label: "prompt hook", configured: strings.Contains(text, "precmd_functions") && strings.Contains(text, "__hostward_precmd")},
		}
	case "bash":
		return []shellCheck{
			{label: "banner", configured: containsAll(text, "banner", "hostward")},
			{label: "prompt env function", configured: strings.Contains(text, "__hostward_prompt_command") || strings.Contains(text, "HOSTWARD_FAILING_COUNT")},
			{label: "prompt hook", configured: strings.Contains(text, "PROMPT_COMMAND") && strings.Contains(text, "__hostward_prompt_command")},
		}
	default:
		return []shellCheck{
			{label: "banner", configured: false},
			{label: "prompt env function", configured: false},
			{label: "prompt hook", configured: false},
		}
	}
}

func containsAll(text string, patterns ...string) bool {
	for _, pattern := range patterns {
		if !strings.Contains(text, pattern) {
			return false
		}
	}
	return true
}

func shellCheckLabel(target, component string) string {
	return fmt.Sprintf("shell %s %s", target, component)
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
