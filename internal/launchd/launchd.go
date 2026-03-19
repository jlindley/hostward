package launchd

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"hostward/internal/config"
	"hostward/internal/fileio"
)

const Label = "com.hostward.scheduler"

type Agent struct {
	Label            string
	ProgramArguments []string
	WorkingDirectory string
	RunAtLoad        bool
	KeepAlive        bool
}

type Status struct {
	PlistExists bool
	Loaded      bool
	Details     string
}

func DefaultAgent(paths config.Paths, binary string, tick time.Duration) Agent {
	return Agent{
		Label:            Label,
		ProgramArguments: []string{binary, "scheduler", "run", "--tick", tick.String()},
		WorkingDirectory: paths.Home,
		RunAtLoad:        true,
		KeepAlive:        true,
	}
}

func Render(agent Agent) (string, error) {
	var buf bytes.Buffer
	if err := plistTemplate.Execute(&buf, agent); err != nil {
		return "", fmt.Errorf("render launchd plist: %w", err)
	}

	return buf.String(), nil
}

func Write(paths config.Paths, agent Agent) error {
	rendered, err := Render(agent)
	if err != nil {
		return err
	}

	return fileio.AtomicWriteFile(paths.LaunchAgentPath, []byte(rendered), 0o644)
}

func Install(paths config.Paths, agent Agent) error {
	if err := Write(paths, agent); err != nil {
		return err
	}

	domain := guiDomain()
	_ = exec.Command("launchctl", "bootout", domain+"/"+agent.Label).Run()

	if output, err := exec.Command("launchctl", "bootstrap", domain, paths.LaunchAgentPath).CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl bootstrap failed: %w: %s", err, bytes.TrimSpace(output))
	}
	if output, err := exec.Command("launchctl", "kickstart", "-k", domain+"/"+agent.Label).CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl kickstart failed: %w: %s", err, bytes.TrimSpace(output))
	}

	return nil
}

func Uninstall(paths config.Paths, label string) error {
	domain := guiDomain()
	_ = exec.Command("launchctl", "bootout", domain+"/"+label).Run()

	if err := os.Remove(paths.LaunchAgentPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove launch agent plist %s: %w", paths.LaunchAgentPath, err)
	}

	return nil
}

func LoadStatus(paths config.Paths, label string) (Status, error) {
	status := Status{}
	if _, err := os.Stat(paths.LaunchAgentPath); err == nil {
		status.PlistExists = true
	} else if !os.IsNotExist(err) {
		return Status{}, fmt.Errorf("stat launch agent plist %s: %w", paths.LaunchAgentPath, err)
	}

	domain := guiDomain()
	output, err := exec.Command("launchctl", "print", domain+"/"+label).CombinedOutput()
	status.Details = string(bytes.TrimSpace(output))
	if err == nil {
		status.Loaded = true
		return status, nil
	}
	if strings.Contains(status.Details, "Could not find service") {
		status.Details = ""
	}

	return status, nil
}

func guiDomain() string {
	return fmt.Sprintf("gui/%d", os.Getuid())
}

func ExecutablePath(explicit string) (string, error) {
	if explicit != "" {
		return filepath.Abs(explicit)
	}

	return os.Executable()
}

func LooksEphemeralBinary(path string) bool {
	return strings.Contains(path, "go-build") || strings.Contains(path, "/tmp/") || strings.Contains(path, "/var/folders/")
}

var plistTemplate = template.Must(template.New("launchd").Parse(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>{{.Label}}</string>
  <key>ProgramArguments</key>
  <array>
  {{- range .ProgramArguments }}
    <string>{{ . }}</string>
  {{- end }}
  </array>
  <key>WorkingDirectory</key>
  <string>{{.WorkingDirectory}}</string>
  <key>RunAtLoad</key>
  {{- if .RunAtLoad }}<true/>{{ else }}<false/>{{ end }}
  <key>KeepAlive</key>
  {{- if .KeepAlive }}<true/>{{ else }}<false/>{{ end }}
</dict>
</plist>
`))
