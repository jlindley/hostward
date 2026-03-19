package service

import (
	"fmt"
	"time"

	"hostward/internal/config"
	"hostward/internal/history"
	"hostward/internal/logging"
	"hostward/internal/monitor"
	"hostward/internal/notify"
	"hostward/internal/runner"
	"hostward/internal/state"
)

type Service struct {
	paths    config.Paths
	notifier notify.Notifier
}

func New(paths config.Paths) Service {
	return Service{
		paths:    paths,
		notifier: notify.New(),
	}
}

func NewWithNotifier(paths config.Paths, notifier notify.Notifier) Service {
	return Service{
		paths:    paths,
		notifier: notifier,
	}
}

func (s Service) LoadBundle() (config.Bundle, error) {
	return config.Load(s.paths)
}

func (s Service) LoadStore() (state.Store, error) {
	return state.LoadStore(s.paths.RuntimeStatePath)
}

func (s Service) SyncSnapshot(bundle config.Bundle, store state.Store, now time.Time) (state.Snapshot, error) {
	snapshot := state.BuildSnapshot(bundle.Monitors, store, now)
	if err := state.WriteSnapshot(s.paths.CurrentStatePath, snapshot); err != nil {
		return state.Snapshot{}, err
	}

	return snapshot, nil
}

func (s Service) RunMonitor(id string) (state.MonitorSnapshot, error) {
	bundle, err := s.LoadBundle()
	if err != nil {
		return state.MonitorSnapshot{}, err
	}

	definition, err := findMonitor(bundle.Monitors, id)
	if err != nil {
		return state.MonitorSnapshot{}, err
	}
	if definition.Disabled {
		return state.MonitorSnapshot{}, fmt.Errorf("monitor %s is disabled", id)
	}
	if definition.Type != monitor.TypeScript {
		return state.MonitorSnapshot{}, fmt.Errorf("monitor %s is type %s; only script monitors can be run", id, definition.Type)
	}

	store, err := s.LoadStore()
	if err != nil {
		return state.MonitorSnapshot{}, err
	}

	previousRecord := store.Monitors[id]
	previousStatus, _ := state.ResolveStatus(definition, previousRecord, time.Now().UTC())

	result, err := runner.RunScript(s.paths.Home, definition)
	if err != nil {
		return state.MonitorSnapshot{}, err
	}

	record := previousRecord
	record.Status = result.Status
	record.Summary = result.Summary
	record.LastCheckAt = &result.FinishedAt
	if result.Status == monitor.StatusOK {
		record.LastSuccessAt = &result.FinishedAt
		record.FailureStdout = ""
		record.FailureStderr = ""
	} else {
		record.LastFailureAt = &result.FinishedAt
		record.FailureStdout = result.Stdout
		record.FailureStderr = result.Stderr
	}
	if previousStatus != result.Status || record.LastChangeAt == nil {
		record.LastChangeAt = &result.FinishedAt
	}

	if store.Monitors == nil {
		store.Monitors = map[string]state.Record{}
	}
	store.Monitors[id] = record
	store.UpdatedAt = result.FinishedAt

	if err := state.WriteStore(s.paths.RuntimeStatePath, store); err != nil {
		return state.MonitorSnapshot{}, err
	}

	snapshot, err := s.SyncSnapshot(bundle, store, result.FinishedAt)
	if err != nil {
		return state.MonitorSnapshot{}, err
	}

	if previousStatus != result.Status {
		if err := history.Append(s.paths.HistoryLogPath, history.Event{
			At:            result.FinishedAt,
			MonitorID:     definition.ID,
			MonitorName:   definition.DisplayName(),
			Status:        result.Status,
			Previous:      previousStatus,
			Summary:       result.Summary,
			Definition:    definition.SourcePath,
			FailureStdout: result.Stdout,
			FailureStderr: result.Stderr,
		}); err != nil {
			return state.MonitorSnapshot{}, err
		}
	}
	if err := s.notifyIfFailureStarted(bundle.Global, definition, previousStatus, result.Status, result.Summary); err != nil {
		return state.MonitorSnapshot{}, err
	}

	if err := s.logMonitorRun(bundle.Global, definition, result); err != nil {
		return state.MonitorSnapshot{}, err
	}
	if err := s.pruneArtifacts(bundle.Global, result.FinishedAt); err != nil {
		return state.MonitorSnapshot{}, err
	}

	return findSnapshotMonitor(snapshot, definition.ID)
}

func (s Service) ReconcileOnce(now time.Time) (state.Snapshot, error) {
	bundle, err := s.LoadBundle()
	if err != nil {
		return state.Snapshot{}, err
	}
	store, err := s.LoadStore()
	if err != nil {
		return state.Snapshot{}, err
	}
	if store.Monitors == nil {
		store.Monitors = map[string]state.Record{}
	}

	logger, err := s.logger(bundle.Global)
	if err != nil {
		return state.Snapshot{}, err
	}

	for _, definition := range bundle.Monitors {
		record := store.Monitors[definition.ID]
		previousStatus, _ := state.ResolveStatus(definition, record, now)

		switch definition.Type {
		case monitor.TypeScript:
			if !definition.Disabled && scriptDue(definition, record, now) {
				result, err := runner.RunScript(s.paths.Home, definition)
				if err != nil {
					return state.Snapshot{}, err
				}

				record.Status = result.Status
				record.Summary = result.Summary
				record.LastCheckAt = &result.FinishedAt
				if result.Status == monitor.StatusOK {
					record.LastSuccessAt = &result.FinishedAt
					record.FailureStdout = ""
					record.FailureStderr = ""
				} else {
					record.LastFailureAt = &result.FinishedAt
					record.FailureStdout = result.Stdout
					record.FailureStderr = result.Stderr
				}

				if err := s.logMonitorRun(bundle.Global, definition, result); err != nil {
					return state.Snapshot{}, err
				}
			}
		case monitor.TypeDeadman:
			record.LastCheckAt = &now
		}

		newStatus, newSummary := state.ResolveStatus(definition, record, now)
		record.Status = newStatus
		record.Summary = newSummary
		if previousStatus != newStatus || record.LastChangeAt == nil {
			record.LastChangeAt = &now
		}

		store.Monitors[definition.ID] = record

		if previousStatus != newStatus {
			if err := history.Append(s.paths.HistoryLogPath, history.Event{
				At:            now,
				MonitorID:     definition.ID,
				MonitorName:   definition.DisplayName(),
				Status:        newStatus,
				Previous:      previousStatus,
				Summary:       newSummary,
				Definition:    definition.SourcePath,
				FailureStdout: record.FailureStdout,
				FailureStderr: record.FailureStderr,
			}); err != nil {
				return state.Snapshot{}, err
			}
			if err := logger.Log(logging.LevelInfo, "monitor state changed", map[string]any{
				"monitor_id": definition.ID,
				"previous":   previousStatus,
				"status":     newStatus,
				"summary":    newSummary,
			}); err != nil {
				return state.Snapshot{}, err
			}
		}
		if err := s.notifyIfFailureStarted(bundle.Global, definition, previousStatus, newStatus, newSummary); err != nil {
			return state.Snapshot{}, err
		}
	}

	store.UpdatedAt = now
	if err := state.WriteStore(s.paths.RuntimeStatePath, store); err != nil {
		return state.Snapshot{}, err
	}

	snapshot, err := s.SyncSnapshot(bundle, store, now)
	if err != nil {
		return state.Snapshot{}, err
	}
	if err := s.pruneArtifacts(bundle.Global, now); err != nil {
		return state.Snapshot{}, err
	}

	return snapshot, nil
}

func (s Service) PokeMonitor(id string) (state.MonitorSnapshot, error) {
	bundle, err := s.LoadBundle()
	if err != nil {
		return state.MonitorSnapshot{}, err
	}

	definition, err := findMonitor(bundle.Monitors, id)
	if err != nil {
		return state.MonitorSnapshot{}, err
	}
	if definition.Disabled {
		return state.MonitorSnapshot{}, fmt.Errorf("monitor %s is disabled", id)
	}
	if definition.Type != monitor.TypeDeadman {
		return state.MonitorSnapshot{}, fmt.Errorf("monitor %s is type %s; only deadman monitors can be poked", id, definition.Type)
	}

	store, err := s.LoadStore()
	if err != nil {
		return state.MonitorSnapshot{}, err
	}
	if store.Monitors == nil {
		store.Monitors = map[string]state.Record{}
	}

	now := time.Now().UTC()
	previousRecord := store.Monitors[id]
	previousStatus, _ := state.ResolveStatus(definition, previousRecord, now)

	record := previousRecord
	record.Status = monitor.StatusOK
	record.Summary = "poke received"
	record.LastPokeAt = &now
	record.LastCheckAt = &now
	if previousStatus != monitor.StatusOK || record.LastChangeAt == nil {
		record.LastChangeAt = &now
	}
	record.LastSuccessAt = &now

	store.Monitors[id] = record
	store.UpdatedAt = now

	if err := state.WriteStore(s.paths.RuntimeStatePath, store); err != nil {
		return state.MonitorSnapshot{}, err
	}

	snapshot, err := s.SyncSnapshot(bundle, store, now)
	if err != nil {
		return state.MonitorSnapshot{}, err
	}

	newStatus, newSummary := state.ResolveStatus(definition, record, now)
	if previousStatus != newStatus {
		if err := history.Append(s.paths.HistoryLogPath, history.Event{
			At:          now,
			MonitorID:   definition.ID,
			MonitorName: definition.DisplayName(),
			Status:      newStatus,
			Previous:    previousStatus,
			Summary:     newSummary,
			Definition:  definition.SourcePath,
		}); err != nil {
			return state.MonitorSnapshot{}, err
		}
	}

	logger, err := s.logger(bundle.Global)
	if err != nil {
		return state.MonitorSnapshot{}, err
	}
	if err := logger.Log(logging.LevelInfo, "deadman poke received", map[string]any{
		"monitor_id": definition.ID,
		"status":     newStatus,
	}); err != nil {
		return state.MonitorSnapshot{}, err
	}
	if err := s.pruneArtifacts(bundle.Global, now); err != nil {
		return state.MonitorSnapshot{}, err
	}

	return findSnapshotMonitor(snapshot, definition.ID)
}

func (s Service) NotifyTest() error {
	bundle, err := s.LoadBundle()
	if err != nil {
		return err
	}

	return s.notifier.NotifyTest(bundle.Global)
}

func (s Service) Snapshot() (config.Bundle, state.Store, state.Snapshot, error) {
	bundle, err := s.LoadBundle()
	if err != nil {
		return config.Bundle{}, state.Store{}, state.Snapshot{}, err
	}
	store, err := s.LoadStore()
	if err != nil {
		return config.Bundle{}, state.Store{}, state.Snapshot{}, err
	}

	snapshot := state.BuildSnapshot(bundle.Monitors, store, time.Now().UTC())
	return bundle, store, snapshot, nil
}

func (s Service) SetMonitorDisabled(id string, disabled bool) (state.MonitorSnapshot, error) {
	beforeBundle, err := s.LoadBundle()
	if err != nil {
		return state.MonitorSnapshot{}, err
	}
	store, err := s.LoadStore()
	if err != nil {
		return state.MonitorSnapshot{}, err
	}
	if store.Monitors == nil {
		store.Monitors = map[string]state.Record{}
	}

	now := time.Now().UTC()
	record := store.Monitors[id]
	currentDefinition, err := findMonitor(beforeBundle.Monitors, id)
	if err != nil {
		return state.MonitorSnapshot{}, err
	}
	previousStatus, _ := state.ResolveStatus(currentDefinition, record, now)

	definition, err := config.SetMonitorDisabled(s.paths, id, disabled)
	if err != nil {
		return state.MonitorSnapshot{}, err
	}
	bundle, err := s.LoadBundle()
	if err != nil {
		return state.MonitorSnapshot{}, err
	}
	if disabled {
		record.Status = monitor.StatusDisabled
		record.Summary = "disabled"
	} else if definition.Type == monitor.TypeDeadman && record.LastPokeAt == nil {
		record.Status = monitor.StatusUnknown
		record.Summary = "awaiting first poke"
	} else if definition.Type == monitor.TypeScript && record.LastCheckAt == nil {
		record.Status = monitor.StatusUnknown
		record.Summary = "never run"
	}
	if previousStatus != record.Status || record.LastChangeAt == nil {
		record.LastChangeAt = &now
	}
	record.LastCheckAt = &now
	store.Monitors[id] = record
	store.UpdatedAt = now

	if err := state.WriteStore(s.paths.RuntimeStatePath, store); err != nil {
		return state.MonitorSnapshot{}, err
	}

	snapshot, err := s.SyncSnapshot(bundle, store, now)
	if err != nil {
		return state.MonitorSnapshot{}, err
	}

	newStatus, newSummary := state.ResolveStatus(definition, record, now)
	if previousStatus != newStatus {
		if err := history.Append(s.paths.HistoryLogPath, history.Event{
			At:          now,
			MonitorID:   definition.ID,
			MonitorName: definition.DisplayName(),
			Status:      newStatus,
			Previous:    previousStatus,
			Summary:     newSummary,
			Definition:  definition.SourcePath,
		}); err != nil {
			return state.MonitorSnapshot{}, err
		}
	}

	logger, err := s.logger(bundle.Global)
	if err != nil {
		return state.MonitorSnapshot{}, err
	}
	message := "monitor enabled"
	if disabled {
		message = "monitor disabled"
	}
	if err := logger.Log(logging.LevelInfo, message, map[string]any{
		"monitor_id": definition.ID,
		"status":     newStatus,
	}); err != nil {
		return state.MonitorSnapshot{}, err
	}

	return findSnapshotMonitor(snapshot, definition.ID)
}

func findMonitor(definitions []monitor.Definition, id string) (monitor.Definition, error) {
	for _, definition := range definitions {
		if definition.ID == id {
			return definition, nil
		}
	}

	return monitor.Definition{}, fmt.Errorf("unknown monitor %q", id)
}

func findSnapshotMonitor(snapshot state.Snapshot, id string) (state.MonitorSnapshot, error) {
	for _, monitorSnapshot := range snapshot.Monitors {
		if monitorSnapshot.ID == id {
			return monitorSnapshot, nil
		}
	}

	return state.MonitorSnapshot{}, fmt.Errorf("monitor %q not found in snapshot", id)
}

func scriptDue(definition monitor.Definition, record state.Record, now time.Time) bool {
	if record.LastCheckAt == nil {
		return true
	}

	return now.Sub(*record.LastCheckAt) >= definition.Every
}

func (s Service) logger(cfg config.Config) (logging.Logger, error) {
	level, err := logging.ParseLevel(cfg.LogLevel)
	if err != nil {
		return logging.Logger{}, err
	}

	return logging.Logger{
		Path:  s.paths.OperationalLogPath,
		Level: level,
	}, nil
}

func (s Service) logMonitorRun(cfg config.Config, definition monitor.Definition, result runner.Result) error {
	logger, err := s.logger(cfg)
	if err != nil {
		return err
	}

	return logger.Log(logging.LevelInfo, "monitor run completed", map[string]any{
		"monitor_id": definition.ID,
		"status":     result.Status,
		"duration":   result.FinishedAt.Sub(result.StartedAt).String(),
		"summary":    result.Summary,
	})
}

func (s Service) notifyIfFailureStarted(cfg config.Config, definition monitor.Definition, previousStatus, newStatus monitor.Status, summary string) error {
	if previousStatus == monitor.StatusFailing || newStatus != monitor.StatusFailing {
		return nil
	}

	if err := s.notifier.NotifyFailureStart(cfg, definition, summary); err != nil {
		logger, logErr := s.logger(cfg)
		if logErr == nil {
			_ = logger.Log(logging.LevelWarn, "notification delivery failed", map[string]any{
				"monitor_id": definition.ID,
				"error":      err.Error(),
			})
		}
		return nil
	}

	return nil
}

func (s Service) pruneArtifacts(cfg config.Config, now time.Time) error {
	if err := history.PruneOlderThan(s.paths.HistoryLogPath, now.Add(-cfg.HistoryRetention)); err != nil {
		return err
	}
	if err := logging.PruneOlderThan(s.paths.OperationalLogPath, now.Add(-cfg.LogRetention)); err != nil {
		return err
	}
	if err := logging.TrimToMaxBytes(s.paths.OperationalLogPath, cfg.LogMaxBytes); err != nil {
		return err
	}

	return nil
}
