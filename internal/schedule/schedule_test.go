package schedule_test

import (
	"context"
	"testing"
	"time"

	"github.com/querysheriff/collector/internal/schedule"
)

func TestNextTick(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		now      time.Time
		interval time.Duration
		want     time.Time
	}{
		{
			name:     "mid interval rounds up to next boundary",
			now:      time.Date(2026, 6, 21, 10, 0, 30, 0, time.UTC),
			interval: time.Minute,
			want:     time.Date(2026, 6, 21, 10, 1, 0, 0, time.UTC),
		},
		{
			name:     "exactly on a boundary advances to the following one",
			now:      time.Date(2026, 6, 21, 10, 1, 0, 0, time.UTC),
			interval: time.Minute,
			want:     time.Date(2026, 6, 21, 10, 2, 0, 0, time.UTC),
		},
		{
			name:     "ten second interval",
			now:      time.Date(2026, 6, 21, 10, 0, 5, 0, time.UTC),
			interval: 10 * time.Second,
			want:     time.Date(2026, 6, 21, 10, 0, 10, 0, time.UTC),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := schedule.NextTick(tc.now, tc.interval); !got.Equal(tc.want) {
				t.Errorf("NextTick(%v, %v) = %v, want %v", tc.now, tc.interval, got, tc.want)
			}
		})
	}
}

// A task stuck in a long run must not delay another task's ticks.
func TestRunTasksDoNotBlockEachOther(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	slowStarted := make(chan struct{}, 1)
	release := make(chan struct{})
	fast := make(chan struct{}, 64)

	var s schedule.Scheduler
	s.Add(20*time.Millisecond, func(ctx context.Context, _ time.Time) {
		select {
		case slowStarted <- struct{}{}:
		default:
		}
		select {
		case <-release:
		case <-ctx.Done():
		}
	})
	s.Add(20*time.Millisecond, func(context.Context, time.Time) {
		select {
		case fast <- struct{}{}:
		default:
		}
	})

	done := make(chan struct{})
	go func() {
		s.Run(ctx)
		close(done)
	}()

	select {
	case <-slowStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("slow task never started")
	}

	// The fast task keeps firing while the slow task is still parked in its run.
	for range 3 {
		select {
		case <-fast:
		case <-time.After(2 * time.Second):
			t.Fatal("fast task was blocked by the slow task")
		}
	}

	close(release)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after context cancellation")
	}
}

func TestRunStopsOnCancel(t *testing.T) {
	t.Parallel()

	fired := make(chan struct{}, 1)
	var s schedule.Scheduler
	s.Add(20*time.Millisecond, func(context.Context, time.Time) {
		select {
		case fired <- struct{}{}:
		default:
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		s.Run(ctx)
		close(done)
	}()

	select {
	case <-fired:
	case <-time.After(2 * time.Second):
		t.Fatal("task did not fire")
	}

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after context cancellation")
	}
}
