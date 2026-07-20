package logs

import (
	"log/slog"
	"strings"
	"time"
)

const (
	selfBurstWindow    = 10 * time.Second
	selfBurstThreshold = 1000
	selfSuspension     = 5 * time.Minute
)

// selfBreaker pauses pgdozor's own logs on a burst, so they can't loop: storing
// a log on the monitored server can make Postgres log it, which gets stored again.
type selfBreaker struct {
	windowStart    time.Time
	count          int
	suspended      bool
	suspendedUntil time.Time
}

// admit reports whether the record may be collected; pgdozor's own logs are
// dropped while suspended, everything else always passes.
func (b *selfBreaker) admit(now time.Time, event ParsedLogEvent, logger *slog.Logger) bool {
	if !isPgdozorRecord(event) {
		return true
	}

	if b.suspended {
		if now.Before(b.suspendedUntil) {
			return false
		}

		b.suspended = false
		b.windowStart = now
		b.count = 1
		logger.Info("resuming collection of pgdozor logs after burst suspension")

		return true
	}

	if now.Sub(b.windowStart) >= selfBurstWindow {
		b.windowStart = now
		b.count = 0
	}

	b.count++
	if b.count > selfBurstThreshold {
		b.suspended = true
		b.suspendedUntil = now.Add(selfSuspension)
		logger.Error(
			"pgdozor log burst detected; suspending collection of pgdozor logs",
			"threshold", selfBurstThreshold,
			"window", selfBurstWindow,
			"suspension", selfSuspension,
		)

		return false
	}

	return true
}

func isPgdozorRecord(event ParsedLogEvent) bool {
	return containsPgdozor(event.Username) || containsPgdozor(event.DatabaseName)
}

func containsPgdozor(s string) bool {
	return strings.Contains(strings.ToLower(s), "pgdozor")
}
