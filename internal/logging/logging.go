package logging

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"hostward/internal/fileio"
)

type Level string

const (
	LevelError Level = "error"
	LevelWarn  Level = "warn"
	LevelInfo  Level = "info"
	LevelDebug Level = "debug"
)

type Entry struct {
	At      time.Time      `json:"at"`
	Level   Level          `json:"level"`
	Message string         `json:"message"`
	Fields  map[string]any `json:"fields,omitempty"`
}

type Logger struct {
	Path  string
	Level Level
}

func ParseLevel(input string) (Level, error) {
	level := Level(input)
	switch level {
	case LevelError, LevelWarn, LevelInfo, LevelDebug:
		return level, nil
	default:
		return "", fmt.Errorf("unsupported log level %q", input)
	}
}

func (l Logger) Log(level Level, message string, fields map[string]any) error {
	if !enabled(level, l.Level) {
		return nil
	}

	entry := Entry{
		At:      time.Now().UTC(),
		Level:   level,
		Message: message,
		Fields:  fields,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal log entry: %w", err)
	}

	data = append(data, '\n')
	return fileio.AppendLine(l.Path, data, 0o644)
}

func PruneOlderThan(path string, cutoff time.Time) error {
	input, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read log %s: %w", path, err)
	}

	var output bytes.Buffer
	for _, rawLine := range bytes.Split(input, []byte{'\n'}) {
		line := bytes.TrimSpace(rawLine)
		if len(line) == 0 {
			continue
		}

		var entry Entry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		if entry.At.Before(cutoff) {
			continue
		}

		output.Write(line)
		output.WriteByte('\n')
	}

	return fileio.AtomicWriteFile(path, output.Bytes(), 0o644)
}

func TrimToMaxBytes(path string, maxBytes int64) error {
	input, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read log %s: %w", path, err)
	}
	if int64(len(input)) <= maxBytes {
		return nil
	}

	lines := bytes.Split(input, []byte{'\n'})
	if len(lines) > 0 && len(lines[len(lines)-1]) == 0 {
		lines = lines[:len(lines)-1]
	}

	kept := make([][]byte, 0, len(lines))
	var size int64
	for i := len(lines) - 1; i >= 0; i-- {
		line := bytes.TrimSpace(lines[i])
		if len(line) == 0 {
			continue
		}

		lineSize := int64(len(line) + 1)
		if lineSize > maxBytes {
			continue
		}
		if size > 0 && size+lineSize > maxBytes {
			break
		}

		kept = append(kept, line)
		size += lineSize
	}

	var output bytes.Buffer
	for i := len(kept) - 1; i >= 0; i-- {
		output.Write(kept[i])
		output.WriteByte('\n')
	}

	return fileio.AtomicWriteFile(path, output.Bytes(), 0o644)
}

func enabled(level, threshold Level) bool {
	return levelRank(level) <= levelRank(threshold)
}

func levelRank(level Level) int {
	switch level {
	case LevelError:
		return 0
	case LevelWarn:
		return 1
	case LevelInfo:
		return 2
	case LevelDebug:
		return 3
	default:
		return 99
	}
}
