package schedule

import (
	"context"
	"sync"
	"time"
)

type task struct {
	interval time.Duration
	run      func(ctx context.Context, now time.Time)
}

type Scheduler struct {
	tasks []task
}

func (s *Scheduler) Add(interval time.Duration, run func(ctx context.Context, now time.Time)) {
	s.tasks = append(s.tasks, task{interval: interval, run: run})
}

func (s *Scheduler) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for _, t := range s.tasks {
		wg.Go(func() { t.loop(ctx) })
	}
	wg.Wait()
}

func (t task) loop(ctx context.Context) {
	for {
		now := time.Now().UTC()
		scheduled := NextTick(now, t.interval)
		timer := time.NewTimer(time.Until(scheduled))

		select {
		case <-ctx.Done():
			timer.Stop()
			return

		case <-timer.C:
			t.run(ctx, scheduled)
		}
	}
}

// NextTick example: NextTick(10:07:35, 5m) returns 10:10:00.
func NextTick(now time.Time, interval time.Duration) time.Time {
	return now.Truncate(interval).Add(interval)
}
