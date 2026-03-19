package notify

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"hostward/internal/config"
	"hostward/internal/monitor"
)

type RunnerFunc func(name string, args ...string) error

type Notifier struct {
	run RunnerFunc
}

func New() Notifier {
	return Notifier{
		run: func(name string, args ...string) error {
			return exec.Command(name, args...).Run()
		},
	}
}

func NewWithRunner(run RunnerFunc) Notifier {
	return Notifier{run: run}
}

func (n Notifier) NotifyFailureStart(cfg config.Config, definition monitor.Definition, summary string) error {
	if !cfg.Notifications.Enabled || cfg.Notifications.Mode != "failure-start" {
		return nil
	}

	body := summary
	if body == "" {
		body = "monitor failing"
	}

	return n.run("osascript", "-e", appleScript("Hostward failure", definition.DisplayName(), body))
}

func (n Notifier) NotifyTest(cfg config.Config) error {
	if !cfg.Notifications.Enabled || cfg.Notifications.Mode != "failure-start" {
		return nil
	}

	body := fmt.Sprintf("test notification at %s", time.Now().UTC().Format(time.RFC3339))
	return n.run("osascript", "-e", appleScript("Hostward", "Test notification", body))
}

func appleScript(title, subtitle, body string) string {
	parts := []string{
		fmt.Sprintf("display notification %s with title %s", quoteAppleScript(body), quoteAppleScript(title)),
	}
	if subtitle != "" {
		parts = append(parts, "subtitle "+quoteAppleScript(subtitle))
	}

	return strings.Join(parts, " ")
}

func quoteAppleScript(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + replacer.Replace(value) + `"`
}
