package scaffold

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"hostward/internal/config"
)

var validID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

type ScriptOptions struct {
	ID             string
	DisplayName    string
	Every          string
	Timeout        string
	Command        []string
	WorkingDir     string
	NoInheritEnv   bool
	MaxOutputBytes int
}

type DeadmanOptions struct {
	ID          string
	DisplayName string
	Every       string
	Grace       string
}

type FileExistsOptions struct {
	ID          string
	DisplayName string
	Every       string
	Timeout     string
	Path        string
}

type DirectoryExistsOptions struct {
	ID          string
	DisplayName string
	Every       string
	Timeout     string
	Path        string
}

type ProcessRunningOptions struct {
	ID          string
	DisplayName string
	Every       string
	Timeout     string
	Match       string
}

type FileFreshnessOptions struct {
	ID          string
	DisplayName string
	Every       string
	Timeout     string
	Path        string
	MaxAge      string
}

type FreeSpaceOptions struct {
	ID             string
	DisplayName    string
	Every          string
	Timeout        string
	Path           string
	MinFreePercent int
}

type DirectorySizeOptions struct {
	ID          string
	DisplayName string
	Every       string
	Timeout     string
	Path        string
	MaxBytes    int64
}

func AddScript(paths config.Paths, options ScriptOptions) (string, error) {
	if err := validateID(options.ID); err != nil {
		return "", err
	}
	if options.Every == "" {
		return "", fmt.Errorf("every is required")
	}
	if len(options.Command) == 0 {
		return "", fmt.Errorf("command is required")
	}
	if options.Timeout == "" {
		options.Timeout = config.DefaultScriptTimeout.String()
	}
	if options.MaxOutputBytes <= 0 {
		options.MaxOutputBytes = config.DefaultMaxOutputBytes
	}

	var lines []string
	lines = append(lines, `type = "script"`)
	lines = append(lines, fmt.Sprintf("every = %s", strconv.Quote(options.Every)))
	if options.DisplayName != "" {
		lines = append(lines, fmt.Sprintf("name = %s", strconv.Quote(options.DisplayName)))
	}
	lines = append(lines, fmt.Sprintf("timeout = %s", strconv.Quote(options.Timeout)))
	lines = append(lines, fmt.Sprintf("command = [%s]", joinQuoted(options.Command)))
	if options.WorkingDir != "" {
		lines = append(lines, fmt.Sprintf("working_dir = %s", strconv.Quote(options.WorkingDir)))
	}
	if options.NoInheritEnv {
		lines = append(lines, "no_inherit_env = true")
	}
	lines = append(lines, fmt.Sprintf("max_output_bytes = %d", options.MaxOutputBytes))

	return writeMonitorFile(paths.MonitorsDir, options.ID, strings.Join(lines, "\n")+"\n")
}

func AddDeadman(paths config.Paths, options DeadmanOptions) (string, error) {
	if err := validateID(options.ID); err != nil {
		return "", err
	}
	if options.Every == "" {
		return "", fmt.Errorf("every is required")
	}
	if options.Grace == "" {
		return "", fmt.Errorf("grace is required")
	}

	var lines []string
	lines = append(lines, `type = "deadman"`)
	lines = append(lines, fmt.Sprintf("every = %s", strconv.Quote(options.Every)))
	lines = append(lines, fmt.Sprintf("grace = %s", strconv.Quote(options.Grace)))
	if options.DisplayName != "" {
		lines = append(lines, fmt.Sprintf("name = %s", strconv.Quote(options.DisplayName)))
	}

	return writeMonitorFile(paths.MonitorsDir, options.ID, strings.Join(lines, "\n")+"\n")
}

func AddFileExists(paths config.Paths, options FileExistsOptions) (string, error) {
	if options.Path == "" {
		return "", fmt.Errorf("path is required")
	}

	return AddScript(paths, ScriptOptions{
		ID:          options.ID,
		DisplayName: options.DisplayName,
		Every:       options.Every,
		Timeout:     options.Timeout,
		Command: []string{
			"sh", "-c", "test -e " + shellQuote(options.Path),
		},
	})
}

func AddDirectoryExists(paths config.Paths, options DirectoryExistsOptions) (string, error) {
	if options.Path == "" {
		return "", fmt.Errorf("path is required")
	}

	return AddScript(paths, ScriptOptions{
		ID:          options.ID,
		DisplayName: options.DisplayName,
		Every:       options.Every,
		Timeout:     options.Timeout,
		Command: []string{
			"sh", "-c", "test -d " + shellQuote(options.Path),
		},
	})
}

func AddProcessRunning(paths config.Paths, options ProcessRunningOptions) (string, error) {
	if options.Match == "" {
		return "", fmt.Errorf("match is required")
	}

	return AddScript(paths, ScriptOptions{
		ID:          options.ID,
		DisplayName: options.DisplayName,
		Every:       options.Every,
		Timeout:     options.Timeout,
		Command: []string{
			"sh", "-c", "pgrep -f -- " + shellQuote(options.Match) + " >/dev/null",
		},
	})
}

func AddFileFreshness(paths config.Paths, options FileFreshnessOptions) (string, error) {
	if options.Path == "" {
		return "", fmt.Errorf("path is required")
	}
	if options.MaxAge == "" {
		return "", fmt.Errorf("max-age is required")
	}

	duration, err := config.ParseDuration(options.MaxAge)
	if err != nil {
		return "", fmt.Errorf("parse max-age: %w", err)
	}

	return AddScript(paths, ScriptOptions{
		ID:          options.ID,
		DisplayName: options.DisplayName,
		Every:       options.Every,
		Timeout:     options.Timeout,
		Command: []string{
			"sh", "-c", fmt.Sprintf("test $(( $(date +%%s) - $(stat -f %%m %s) )) -le %d", shellQuote(options.Path), int(duration.Seconds())),
		},
	})
}

func AddFreeSpace(paths config.Paths, options FreeSpaceOptions) (string, error) {
	if options.Path == "" {
		return "", fmt.Errorf("path is required")
	}
	if options.MinFreePercent <= 0 || options.MinFreePercent > 100 {
		return "", fmt.Errorf("min-free-percent must be between 1 and 100")
	}

	return AddScript(paths, ScriptOptions{
		ID:          options.ID,
		DisplayName: options.DisplayName,
		Every:       options.Every,
		Timeout:     options.Timeout,
		Command: []string{
			"sh", "-c", fmt.Sprintf("test \"$((100 - $(df -Pk %s | awk 'NR==2 {gsub(/%%/, \"\", $5); print $5}')))\" -ge %d", shellQuote(options.Path), options.MinFreePercent),
		},
	})
}

func AddDirectorySize(paths config.Paths, options DirectorySizeOptions) (string, error) {
	if options.Path == "" {
		return "", fmt.Errorf("path is required")
	}
	if options.MaxBytes <= 0 {
		return "", fmt.Errorf("max-bytes must be positive")
	}

	return AddScript(paths, ScriptOptions{
		ID:          options.ID,
		DisplayName: options.DisplayName,
		Every:       options.Every,
		Timeout:     options.Timeout,
		Command: []string{
			"sh", "-c", fmt.Sprintf("test \"$(( $(du -sk %s | awk '{print $1}') * 1024 ))\" -le %d", shellQuote(options.Path), options.MaxBytes),
		},
	})
}

func writeMonitorFile(monitorsDir, id, body string) (string, error) {
	if err := os.MkdirAll(monitorsDir, 0o755); err != nil {
		return "", fmt.Errorf("create monitors dir %s: %w", monitorsDir, err)
	}

	path := filepath.Join(monitorsDir, id+".toml")
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("monitor %s already exists", id)
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("stat monitor file %s: %w", path, err)
	}

	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return "", fmt.Errorf("write monitor file %s: %w", path, err)
	}

	return path, nil
}

func joinQuoted(values []string) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, strconv.Quote(value))
	}
	return strings.Join(parts, ", ")
}

func validateID(id string) error {
	if !validID.MatchString(id) {
		return fmt.Errorf("monitor id %q is invalid; use letters, numbers, dot, underscore, or hyphen", id)
	}
	return nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}
