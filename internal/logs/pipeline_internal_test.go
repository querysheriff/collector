package logs

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
)

type nopReporter struct{}

func (nopReporter) ReportLogs(context.Context, time.Time, []AnalyzedLogEvent) error { return nil }

func newTestPipeline() *Pipeline {
	return NewPipeline(
		PipelineConfig{},
		slog.New(slog.DiscardHandler),
		nopReporter{},
	)
}

func TestProcessLinePassesRecords(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		line     string
		wantEmit bool
	}{
		{
			"genuine record emitted",
			`{"user":"app","dbname":"app","error_severity":"LOG","message":"statement: SELECT 1"}`,
			true,
		},
		{
			"pgdozor role now collected",
			`{"user":"pgdozor","dbname":"app","error_severity":"LOG","message":"statement: SELECT 1"}`,
			true,
		},
		{
			"pgdozor database now collected",
			`{"user":"app","dbname":"pgdozor","error_severity":"LOG","message":"statement: SELECT 1"}`,
			true,
		},
		{
			"invalid json dropped",
			"this is not json",
			false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			p := newTestPipeline()
			if _, emit := p.processLine([]byte(c.line)); emit != c.wantEmit {
				t.Errorf("processLine emit = %v, want %v", emit, c.wantEmit)
			}
		})
	}
}

func TestProcessLineDropsOversized(t *testing.T) {
	t.Parallel()

	p := newTestPipeline()
	huge := `{"user":"app","message":"` + strings.Repeat("x", maxLogLineBytes) + `"}`

	if _, emit := p.processLine([]byte(huge)); emit {
		t.Errorf("oversized line should be dropped")
	}
}
