package scheduler

import (
	"context"
	"time"

	"hostward/internal/service"
	"hostward/internal/state"
)

type Runner struct {
	Service service.Service
	Tick    time.Duration
}

func (r Runner) RunOnce(now time.Time) (state.Snapshot, error) {
	return r.Service.ReconcileOnce(now)
}

func (r Runner) RunLoop(ctx context.Context) error {
	tick := r.Tick
	if tick <= 0 {
		tick = time.Second
	}

	if _, err := r.RunOnce(time.Now().UTC()); err != nil {
		return err
	}

	ticker := time.NewTicker(tick)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case now := <-ticker.C:
			if _, err := r.RunOnce(now.UTC()); err != nil {
				return err
			}
		}
	}
}
