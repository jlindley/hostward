package history

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"hostward/internal/fileio"
	"hostward/internal/monitor"
)

type Event struct {
	At            time.Time      `json:"at"`
	MonitorID     string         `json:"monitor_id"`
	MonitorName   string         `json:"monitor_name,omitempty"`
	Status        monitor.Status `json:"status"`
	Previous      monitor.Status `json:"previous_status,omitempty"`
	Summary       string         `json:"summary,omitempty"`
	Definition    string         `json:"definition_path,omitempty"`
	FailureStdout string         `json:"failure_stdout,omitempty"`
	FailureStderr string         `json:"failure_stderr,omitempty"`
}

func Append(path string, event Event) error {
	if event.At.IsZero() {
		event.At = time.Now().UTC()
	}

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal history event: %w", err)
	}

	data = append(data, '\n')
	return fileio.AppendLine(path, data, 0o644)
}

func PruneOlderThan(path string, cutoff time.Time) error {
	input, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read history log %s: %w", path, err)
	}

	var output bytes.Buffer
	for _, rawLine := range bytes.Split(input, []byte{'\n'}) {
		line := bytes.TrimSpace(rawLine)
		if len(line) == 0 {
			continue
		}

		var event Event
		if err := json.Unmarshal(line, &event); err != nil {
			continue
		}
		if event.At.Before(cutoff) {
			continue
		}

		output.Write(line)
		output.WriteByte('\n')
	}

	return fileio.AtomicWriteFile(path, output.Bytes(), 0o644)
}
