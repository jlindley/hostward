package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"hostward/internal/fileio"
	"hostward/internal/monitor"
)

const (
	DefaultHistoryRetention = 31 * 24 * time.Hour
	DefaultLogRetention     = 31 * 24 * time.Hour
	DefaultLogMaxBytes      = 10 * 1024 * 1024
	DefaultScriptTimeout    = 30 * time.Second
	DefaultMaxOutputBytes   = 64 * 1024
)

type Config struct {
	BannerMode       string
	HistoryRetention time.Duration
	LogLevel         string
	LogRetention     time.Duration
	LogMaxBytes      int64
	Notifications    NotificationsConfig
}

type NotificationsConfig struct {
	Enabled bool
	Mode    string
}

type Bundle struct {
	Global   Config
	Monitors []monitor.Definition
	Paths    Paths
}

func DefaultConfig() Config {
	return Config{
		BannerMode:       "count",
		HistoryRetention: DefaultHistoryRetention,
		LogLevel:         "info",
		LogRetention:     DefaultLogRetention,
		LogMaxBytes:      DefaultLogMaxBytes,
		Notifications: NotificationsConfig{
			Enabled: true,
			Mode:    "failure-start",
		},
	}
}

func Load(paths Paths) (Bundle, error) {
	cfg, err := loadGlobalConfig(paths.GlobalConfigPath)
	if err != nil {
		return Bundle{}, err
	}

	monitors, err := loadMonitorDefinitions(paths.MonitorsDir)
	if err != nil {
		return Bundle{}, err
	}

	return Bundle{
		Global:   cfg,
		Monitors: monitors,
		Paths:    paths,
	}, nil
}

func SetMonitorDisabled(paths Paths, id string, disabled bool) (monitor.Definition, error) {
	path := filepath.Join(paths.MonitorsDir, id+".toml")
	definition, err := loadMonitorDefinition(path)
	if err != nil {
		return monitor.Definition{}, err
	}

	definition.Disabled = disabled
	if err := writeMonitorDefinition(path, definition); err != nil {
		return monitor.Definition{}, err
	}

	return loadMonitorDefinition(path)
}

func loadGlobalConfig(path string) (Config, error) {
	cfg := DefaultConfig()

	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	} else if err != nil {
		return Config{}, fmt.Errorf("stat global config %s: %w", path, err)
	}

	var raw rawConfig
	meta, err := toml.DecodeFile(path, &raw)
	if err != nil {
		return Config{}, fmt.Errorf("decode global config %s: %w", path, err)
	}
	if undecoded := meta.Undecoded(); len(undecoded) > 0 {
		return Config{}, fmt.Errorf("global config %s has unknown keys: %s", path, joinTomlKeys(undecoded))
	}

	if raw.BannerMode != "" {
		cfg.BannerMode = raw.BannerMode
	}
	if raw.HistoryRetention != "" {
		cfg.HistoryRetention, err = ParseDuration(raw.HistoryRetention)
		if err != nil {
			return Config{}, fmt.Errorf("parse history_retention in %s: %w", path, err)
		}
	}
	if raw.LogLevel != "" {
		cfg.LogLevel = raw.LogLevel
	}
	if raw.LogRetention != "" {
		cfg.LogRetention, err = ParseDuration(raw.LogRetention)
		if err != nil {
			return Config{}, fmt.Errorf("parse log_retention in %s: %w", path, err)
		}
	}
	if raw.LogMaxBytes > 0 {
		cfg.LogMaxBytes = raw.LogMaxBytes
	}
	if raw.Notifications.Enabled != nil {
		cfg.Notifications.Enabled = *raw.Notifications.Enabled
	}
	if raw.Notifications.Mode != "" {
		cfg.Notifications.Mode = raw.Notifications.Mode
	}

	if err := validateGlobalConfig(cfg); err != nil {
		return Config{}, fmt.Errorf("validate global config %s: %w", path, err)
	}

	return cfg, nil
}

func loadMonitorDefinitions(dir string) ([]monitor.Definition, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read monitors dir %s: %w", dir, err)
	}

	var fileNames []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) != ".toml" {
			continue
		}
		fileNames = append(fileNames, entry.Name())
	}
	slices.Sort(fileNames)

	definitions := make([]monitor.Definition, 0, len(fileNames))
	for _, fileName := range fileNames {
		path := filepath.Join(dir, fileName)
		definition, err := loadMonitorDefinition(path)
		if err != nil {
			return nil, err
		}
		definitions = append(definitions, definition)
	}

	return definitions, nil
}

func loadMonitorDefinition(path string) (monitor.Definition, error) {
	var raw rawMonitorConfig
	meta, err := toml.DecodeFile(path, &raw)
	if err != nil {
		return monitor.Definition{}, fmt.Errorf("decode monitor %s: %w", path, err)
	}
	if undecoded := meta.Undecoded(); len(undecoded) > 0 {
		return monitor.Definition{}, fmt.Errorf("monitor %s has unknown keys: %s", path, joinTomlKeys(undecoded))
	}

	id := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if id == "" {
		return monitor.Definition{}, fmt.Errorf("monitor %s has empty id derived from file name", path)
	}

	every, err := ParseDuration(raw.Every)
	if err != nil {
		return monitor.Definition{}, fmt.Errorf("parse every in %s: %w", path, err)
	}

	definition := monitor.Definition{
		ID:         id,
		Name:       raw.Name,
		Type:       monitor.Type(raw.Type),
		Every:      every,
		Disabled:   raw.Disabled,
		SourcePath: path,
	}

	switch definition.Type {
	case monitor.TypeScript:
		timeout := DefaultScriptTimeout
		if raw.Timeout != "" {
			timeout, err = ParseDuration(raw.Timeout)
			if err != nil {
				return monitor.Definition{}, fmt.Errorf("parse timeout in %s: %w", path, err)
			}
		}

		definition.Script = &monitor.ScriptConfig{
			Command:        slices.Clone(raw.Command),
			Timeout:        timeout,
			WorkingDir:     raw.WorkingDir,
			InheritEnv:     !raw.NoInheritEnv,
			MaxOutputBytes: firstPositive(raw.MaxOutputBytes, DefaultMaxOutputBytes),
		}
	case monitor.TypeDeadman:
		grace, err := ParseDuration(raw.Grace)
		if err != nil {
			return monitor.Definition{}, fmt.Errorf("parse grace in %s: %w", path, err)
		}

		definition.Deadman = &monitor.DeadmanConfig{
			Grace: grace,
		}
	default:
		return monitor.Definition{}, fmt.Errorf("monitor %s has unsupported type %q", path, raw.Type)
	}

	if err := validateMonitorDefinition(definition); err != nil {
		return monitor.Definition{}, fmt.Errorf("validate monitor %s: %w", path, err)
	}

	return definition, nil
}

func validateGlobalConfig(cfg Config) error {
	var problems []error

	switch cfg.BannerMode {
	case "count", "list":
	default:
		problems = append(problems, fmt.Errorf("banner_mode must be \"count\" or \"list\""))
	}

	switch cfg.LogLevel {
	case "error", "warn", "info", "debug":
	default:
		problems = append(problems, fmt.Errorf("log_level must be one of error, warn, info, debug"))
	}

	if cfg.HistoryRetention <= 0 {
		problems = append(problems, fmt.Errorf("history_retention must be positive"))
	}
	if cfg.LogRetention <= 0 {
		problems = append(problems, fmt.Errorf("log_retention must be positive"))
	}
	if cfg.LogMaxBytes <= 0 {
		problems = append(problems, fmt.Errorf("log_max_bytes must be positive"))
	}

	switch cfg.Notifications.Mode {
	case "failure-start":
	default:
		problems = append(problems, fmt.Errorf("notifications.mode must currently be \"failure-start\""))
	}

	return errors.Join(problems...)
}

func validateMonitorDefinition(definition monitor.Definition) error {
	var problems []error

	if definition.Every <= 0 {
		problems = append(problems, fmt.Errorf("every must be positive"))
	}

	switch definition.Type {
	case monitor.TypeScript:
		if definition.Script == nil {
			problems = append(problems, fmt.Errorf("script config is required"))
			break
		}
		if len(definition.Script.Command) == 0 {
			problems = append(problems, fmt.Errorf("script monitor requires a non-empty command"))
		}
		if definition.Script.Timeout <= 0 {
			problems = append(problems, fmt.Errorf("script timeout must be positive"))
		}
		if definition.Script.MaxOutputBytes <= 0 {
			problems = append(problems, fmt.Errorf("max_output_bytes must be positive"))
		}
	case monitor.TypeDeadman:
		if definition.Deadman == nil {
			problems = append(problems, fmt.Errorf("deadman config is required"))
			break
		}
		if definition.Deadman.Grace <= 0 {
			problems = append(problems, fmt.Errorf("deadman grace must be positive"))
		}
	default:
		problems = append(problems, fmt.Errorf("unsupported type %q", definition.Type))
	}

	return errors.Join(problems...)
}

func joinTomlKeys(keys []toml.Key) string {
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key.String())
	}
	return strings.Join(parts, ", ")
}

func firstPositive(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func writeMonitorDefinition(path string, definition monitor.Definition) error {
	var lines []string

	lines = append(lines, fmt.Sprintf("type = %s", strconv.Quote(string(definition.Type))))
	lines = append(lines, fmt.Sprintf("every = %s", strconv.Quote(definition.Every.String())))
	if definition.Name != "" {
		lines = append(lines, fmt.Sprintf("name = %s", strconv.Quote(definition.Name)))
	}
	if definition.Disabled {
		lines = append(lines, "disabled = true")
	}

	switch definition.Type {
	case monitor.TypeScript:
		if definition.Script == nil {
			return fmt.Errorf("script monitor %s missing script config", definition.ID)
		}
		lines = append(lines, fmt.Sprintf("timeout = %s", strconv.Quote(definition.Script.Timeout.String())))
		lines = append(lines, fmt.Sprintf("command = [%s]", joinQuoted(definition.Script.Command)))
		if definition.Script.WorkingDir != "" {
			lines = append(lines, fmt.Sprintf("working_dir = %s", strconv.Quote(definition.Script.WorkingDir)))
		}
		if !definition.Script.InheritEnv {
			lines = append(lines, "no_inherit_env = true")
		}
		lines = append(lines, fmt.Sprintf("max_output_bytes = %d", definition.Script.MaxOutputBytes))
	case monitor.TypeDeadman:
		if definition.Deadman == nil {
			return fmt.Errorf("deadman monitor %s missing deadman config", definition.ID)
		}
		lines = append(lines, fmt.Sprintf("grace = %s", strconv.Quote(definition.Deadman.Grace.String())))
	default:
		return fmt.Errorf("unsupported type %q", definition.Type)
	}

	return fileio.AtomicWriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

func joinQuoted(values []string) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, strconv.Quote(value))
	}
	return strings.Join(parts, ", ")
}

type rawConfig struct {
	BannerMode       string              `toml:"banner_mode"`
	HistoryRetention string              `toml:"history_retention"`
	LogLevel         string              `toml:"log_level"`
	LogRetention     string              `toml:"log_retention"`
	LogMaxBytes      int64               `toml:"log_max_bytes"`
	Notifications    rawNotificationPart `toml:"notifications"`
}

type rawNotificationPart struct {
	Enabled *bool  `toml:"enabled"`
	Mode    string `toml:"mode"`
}

type rawMonitorConfig struct {
	Name           string   `toml:"name"`
	Type           string   `toml:"type"`
	Every          string   `toml:"every"`
	Disabled       bool     `toml:"disabled"`
	Timeout        string   `toml:"timeout"`
	Command        []string `toml:"command"`
	WorkingDir     string   `toml:"working_dir"`
	NoInheritEnv   bool     `toml:"no_inherit_env"`
	MaxOutputBytes int      `toml:"max_output_bytes"`
	Grace          string   `toml:"grace"`
}
