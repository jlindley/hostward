package monitor

import "time"

type Type string

const (
	TypeScript  Type = "script"
	TypeDeadman Type = "deadman"
)

type Status string

const (
	StatusOK       Status = "ok"
	StatusFailing  Status = "failing"
	StatusUnknown  Status = "unknown"
	StatusDisabled Status = "disabled"
)

func (s Status) IsAlerting() bool {
	return s == StatusFailing
}

type Definition struct {
	ID         string
	Name       string
	Type       Type
	Every      time.Duration
	Disabled   bool
	SourcePath string
	Script     *ScriptConfig
	Deadman    *DeadmanConfig
}

func (d Definition) DisplayName() string {
	if d.Name != "" {
		return d.Name
	}

	return d.ID
}

type ScriptConfig struct {
	Command        []string
	Timeout        time.Duration
	WorkingDir     string
	InheritEnv     bool
	MaxOutputBytes int
}

type DeadmanConfig struct {
	Grace time.Duration
}
