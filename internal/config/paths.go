package config

import (
	"os"
	"path/filepath"
)

type Paths struct {
	Home               string
	ConfigDir          string
	MonitorsDir        string
	GlobalConfigPath   string
	StateDir           string
	RuntimeStatePath   string
	CurrentStatePath   string
	HistoryLogPath     string
	CacheDir           string
	LogDir             string
	OperationalLogPath string
	LaunchAgentPath    string
}

func DefaultPaths() (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, err
	}

	configBase := envOrDefault("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	stateBase := envOrDefault("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	cacheBase := envOrDefault("XDG_CACHE_HOME", filepath.Join(home, ".cache"))

	configDir := filepath.Join(configBase, "hostward")
	stateDir := filepath.Join(stateBase, "hostward")
	cacheDir := filepath.Join(cacheBase, "hostward")
	logDir := filepath.Join(home, "Library", "Logs", "hostward")

	return Paths{
		Home:               home,
		ConfigDir:          configDir,
		MonitorsDir:        filepath.Join(configDir, "monitors"),
		GlobalConfigPath:   filepath.Join(configDir, "config.toml"),
		StateDir:           stateDir,
		RuntimeStatePath:   filepath.Join(stateDir, "monitor-state.json"),
		CurrentStatePath:   filepath.Join(cacheDir, "current-state.json"),
		HistoryLogPath:     filepath.Join(stateDir, "history.jsonl"),
		CacheDir:           cacheDir,
		LogDir:             logDir,
		OperationalLogPath: filepath.Join(logDir, "hostward.jsonl"),
		LaunchAgentPath:    filepath.Join(home, "Library", "LaunchAgents", "com.hostward.scheduler.plist"),
	}, nil
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}
