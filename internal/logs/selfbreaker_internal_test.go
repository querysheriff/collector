package logs

import (
	"log/slog"
	"testing"
	"time"
)

func TestSelfBreakerSuspendsAndResumes(t *testing.T) {
	t.Parallel()

	var b selfBreaker
	logger := slog.New(slog.DiscardHandler)
	line := ParsedLogEvent{Username: "pgdozor", DatabaseName: "pgdozor"}
	base := time.Unix(0, 0)

	for i := range selfBurstThreshold {
		if !b.admit(base, line, logger) {
			t.Fatalf("record %d should be admitted before the burst trips", i)
		}
	}
	if b.admit(base, line, logger) {
		t.Fatal("record that trips the breaker should be dropped")
	}
	if b.admit(base.Add(time.Second), line, logger) {
		t.Fatal("record during suspension should be dropped")
	}
	if !b.admit(base.Add(selfSuspension+time.Second), line, logger) {
		t.Fatal("record after the cooldown should resume")
	}
}

func TestSelfBreakerIgnoresSlowDrip(t *testing.T) {
	t.Parallel()

	var b selfBreaker
	logger := slog.New(slog.DiscardHandler)
	line := ParsedLogEvent{Username: "pgdozor", DatabaseName: "pgdozor"}
	base := time.Unix(0, 0)

	for i := range selfBurstThreshold + 1 {
		at := base.Add(time.Duration(i) * (selfBurstWindow + time.Second))
		if !b.admit(at, line, logger) {
			t.Fatalf("record %d should be admitted", i)
		}
	}
}

func TestSelfBreakerPassesNonPgdozor(t *testing.T) {
	t.Parallel()

	var b selfBreaker
	logger := slog.New(slog.DiscardHandler)
	line := ParsedLogEvent{Username: "app", DatabaseName: "app"}
	base := time.Unix(0, 0)

	for range selfBurstThreshold + 1 {
		if !b.admit(base, line, logger) {
			t.Fatal("non-pgdozor record must always be admitted")
		}
	}
}
