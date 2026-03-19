package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"hostward/internal/monitor"
)

type Result struct {
	Status     monitor.Status
	Summary    string
	Stdout     string
	Stderr     string
	StartedAt  time.Time
	FinishedAt time.Time
}

func RunScript(home string, definition monitor.Definition) (Result, error) {
	if definition.Script == nil {
		return Result{}, fmt.Errorf("monitor %s is not a script monitor", definition.ID)
	}
	if len(definition.Script.Command) == 0 {
		return Result{}, fmt.Errorf("monitor %s has empty command", definition.ID)
	}

	startedAt := time.Now().UTC()
	ctx, cancel := context.WithTimeout(context.Background(), definition.Script.Timeout)
	defer cancel()

	command := exec.CommandContext(ctx, definition.Script.Command[0], definition.Script.Command[1:]...)
	if definition.Script.WorkingDir != "" {
		command.Dir = definition.Script.WorkingDir
	} else {
		command.Dir = home
	}
	if !definition.Script.InheritEnv {
		command.Env = []string{}
	} else {
		command.Env = os.Environ()
	}

	var stdout boundedBuffer
	var stderr boundedBuffer
	stdout.limit = definition.Script.MaxOutputBytes
	stderr.limit = definition.Script.MaxOutputBytes
	command.Stdout = &stdout
	command.Stderr = &stderr

	runErr := command.Run()
	finishedAt := time.Now().UTC()

	result := Result{
		Stdout:     stdout.String(),
		Stderr:     stderr.String(),
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
	}

	switch {
	case runErr == nil:
		result.Status = monitor.StatusOK
		result.Summary = "command succeeded"
		return result, nil
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		result.Status = monitor.StatusFailing
		result.Summary = fmt.Sprintf("command timed out after %s", definition.Script.Timeout)
		return result, nil
	default:
		result.Status = monitor.StatusFailing
		result.Summary = summarizeRunError(runErr)
		return result, nil
	}
}

type boundedBuffer struct {
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.limit <= 0 {
		return len(p), nil
	}

	remaining := b.limit - b.buf.Len()
	if remaining > 0 {
		if len(p) > remaining {
			_, _ = b.buf.Write(p[:remaining])
			b.truncated = true
		} else {
			_, _ = b.buf.Write(p)
		}
	} else {
		b.truncated = true
	}

	return len(p), nil
}

func (b *boundedBuffer) String() string {
	text := strings.TrimSpace(b.buf.String())
	if !b.truncated {
		return text
	}
	if text == "" {
		return "[output truncated]"
	}
	return text + "\n[output truncated]"
}

func summarizeRunError(err error) string {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
			return fmt.Sprintf("command exited with status %d", status.ExitStatus())
		}
		return "command exited with failure"
	}

	return err.Error()
}
