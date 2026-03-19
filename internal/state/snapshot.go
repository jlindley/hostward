package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"hostward/internal/fileio"
	"hostward/internal/monitor"
)

type Snapshot struct {
	GeneratedAt  time.Time         `json:"generated_at,omitempty"`
	TotalCount   int               `json:"total_count"`
	FailingCount int               `json:"failing_count"`
	UnknownCount int               `json:"unknown_count,omitempty"`
	StatusCounts StatusCounts      `json:"status_counts,omitempty"`
	Failing      []string          `json:"failing,omitempty"`
	Monitors     []MonitorSnapshot `json:"monitors,omitempty"`
}

type StatusCounts struct {
	OK       int `json:"ok,omitempty"`
	Failing  int `json:"failing,omitempty"`
	Unknown  int `json:"unknown,omitempty"`
	Disabled int `json:"disabled,omitempty"`
}

type MonitorSnapshot struct {
	ID            string         `json:"id"`
	Name          string         `json:"name,omitempty"`
	Status        monitor.Status `json:"status"`
	Summary       string         `json:"summary,omitempty"`
	LastCheckAt   *time.Time     `json:"last_check_at,omitempty"`
	LastChangeAt  *time.Time     `json:"last_change_at,omitempty"`
	LastSuccessAt *time.Time     `json:"last_success_at,omitempty"`
	LastFailureAt *time.Time     `json:"last_failure_at,omitempty"`
}

func LoadSnapshot(path string) (Snapshot, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Snapshot{}, nil
	}
	if err != nil {
		return Snapshot{}, err
	}

	var snapshot Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return Snapshot{}, err
	}

	snapshot.Normalize()

	return snapshot, nil
}

func WriteSnapshot(path string, snapshot Snapshot) error {
	snapshot.Normalize()
	data, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}

	return fileio.AtomicWriteFile(path, data, 0o644)
}

func (s *Snapshot) Normalize() {
	if len(s.Monitors) > 0 {
		var counts StatusCounts
		failing := make([]string, 0)
		total := 0

		for i := range s.Monitors {
			name := s.Monitors[i].Name
			if name == "" {
				name = s.Monitors[i].ID
			}

			switch s.Monitors[i].Status {
			case monitor.StatusOK:
				counts.OK++
				total++
			case monitor.StatusFailing:
				counts.Failing++
				total++
				failing = append(failing, name)
			case monitor.StatusUnknown:
				counts.Unknown++
				total++
			case monitor.StatusDisabled:
				counts.Disabled++
			}
		}

		s.StatusCounts = counts
		s.TotalCount = total
		s.FailingCount = counts.Failing
		s.UnknownCount = counts.Unknown
		s.Failing = failing
		return
	}

	if s.FailingCount == 0 && len(s.Failing) > 0 {
		s.FailingCount = len(s.Failing)
	}
	if s.TotalCount < s.FailingCount+s.UnknownCount {
		s.TotalCount = s.FailingCount + s.UnknownCount
	}
	if s.StatusCounts.Failing == 0 && s.FailingCount > 0 {
		s.StatusCounts.Failing = s.FailingCount
	}
	if s.StatusCounts.Unknown == 0 && s.UnknownCount > 0 {
		s.StatusCounts.Unknown = s.UnknownCount
	}
	if s.StatusCounts.OK == 0 && s.TotalCount > s.FailingCount+s.UnknownCount {
		s.StatusCounts.OK = s.TotalCount - s.FailingCount - s.UnknownCount
	}
}
