package logging

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoggerRespectsLevel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hostward.jsonl")
	logger := Logger{Path: path, Level: LevelWarn}

	if err := logger.Log(LevelInfo, "ignored", nil); err != nil {
		t.Fatalf("Log(info) error = %v", err)
	}
	if err := logger.Log(LevelError, "kept", nil); err != nil {
		t.Fatalf("Log(error) error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	text := string(data)
	if strings.Contains(text, "ignored") {
		t.Fatalf("log contains ignored entry: %s", text)
	}
	if !strings.Contains(text, "kept") {
		t.Fatalf("log missing kept entry: %s", text)
	}
}

func TestPruneOlderThan(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hostward.jsonl")
	old := Entry{At: time.Now().UTC().Add(-72 * time.Hour), Level: LevelInfo, Message: "old"}
	newEntry := Entry{At: time.Now().UTC(), Level: LevelInfo, Message: "new"}

	write := func(entry Entry) {
		data, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			t.Fatalf("ReadFile() error = %v", err)
		}
		payload := append(data, mustMarshal(t, entry)...)
		payload = append(payload, '\n')
		if err := os.WriteFile(path, payload, 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
	}

	write(old)
	write(newEntry)

	if err := PruneOlderThan(path, time.Now().UTC().Add(-24*time.Hour)); err != nil {
		t.Fatalf("PruneOlderThan() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	text := string(data)
	if strings.Contains(text, "\"old\"") {
		t.Fatalf("pruned log still contains old entry: %s", text)
	}
	if !strings.Contains(text, "\"new\"") {
		t.Fatalf("pruned log lost new entry: %s", text)
	}
}

func TestPruneOlderThanSkipsMalformedLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hostward.jsonl")
	good := Entry{At: time.Now().UTC(), Level: LevelInfo, Message: "kept"}

	payload := append([]byte("not json\n"), mustMarshal(t, good)...)
	payload = append(payload, '\n')
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := PruneOlderThan(path, time.Now().UTC().Add(-time.Hour)); err != nil {
		t.Fatalf("PruneOlderThan() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	text := string(data)
	if strings.Contains(text, "not json") {
		t.Fatalf("pruned log still contains malformed line: %s", text)
	}
	if !strings.Contains(text, "\"kept\"") {
		t.Fatalf("pruned log lost good entry: %s", text)
	}
}

func TestTrimToMaxBytesKeepsNewestEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hostward.jsonl")
	lines := []string{
		string(mustMarshal(t, Entry{At: time.Now().UTC(), Level: LevelInfo, Message: "old"})),
		string(mustMarshal(t, Entry{At: time.Now().UTC(), Level: LevelInfo, Message: "middle"})),
		string(mustMarshal(t, Entry{At: time.Now().UTC(), Level: LevelInfo, Message: "new"})),
	}
	payload := []byte(strings.Join(lines, "\n") + "\n")
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	maxBytes := int64(len(lines[1]) + len(lines[2]) + 2)
	if err := TrimToMaxBytes(path, maxBytes); err != nil {
		t.Fatalf("TrimToMaxBytes() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	text := string(data)
	if strings.Contains(text, "\"old\"") {
		t.Fatalf("trimmed log still contains old entry: %s", text)
	}
	if !strings.Contains(text, "\"middle\"") || !strings.Contains(text, "\"new\"") {
		t.Fatalf("trimmed log lost recent entries: %s", text)
	}
	if int64(len(data)) > maxBytes {
		t.Fatalf("trimmed log length = %d, want <= %d", len(data), maxBytes)
	}
}

func mustMarshal(t *testing.T, entry Entry) []byte {
	t.Helper()

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	return data
}
