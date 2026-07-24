package logs

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

const (
	logStreamBufferLen = 500
	maxLogLineBytes    = 1 << 20
)

type Reporter interface {
	ReportLogs(ctx context.Context, collectedAt time.Time, logEvents []AnalyzedLogEvent) error
}

type PipelineConfig struct {
	LogTimezone           *time.Location
	DeleteRotatedFiles    bool
	DeleteRotatedJSONOnly bool
	LogFilename           string
	Path                  string
	PollInterval          time.Duration
}

type Pipeline struct {
	cfg      PipelineConfig
	logger   *slog.Logger
	tailer   *Tailer
	parser   *Parser
	analyzer *Analyzer
	reporter Reporter
	breaker  selfBreaker
	now      func() time.Time
}

func NewPipeline(
	cfg PipelineConfig,
	logger *slog.Logger,
	reporter Reporter,
) *Pipeline {
	return &Pipeline{
		cfg:      cfg,
		logger:   logger,
		tailer:   NewTailer(logger, cfg.LogFilename, cfg.DeleteRotatedFiles, cfg.DeleteRotatedJSONOnly),
		parser:   NewParser(cfg.LogTimezone),
		analyzer: NewAnalyzer(),
		reporter: reporter,
		now:      time.Now,
	}
}

// Run wires the stages tail -> process -> batch and blocks until the context is cancelled.
// Each stage owns its output channel and closes it when its input drains, so cancellation propagates down the chain to the batch sink.
func (p *Pipeline) Run(ctx context.Context) error {
	raw, err := p.tailer.Tail(ctx, p.cfg.Path)
	if err != nil {
		return fmt.Errorf("tail %q: %w", p.cfg.Path, err)
	}

	p.batch(ctx, p.process(ctx, raw))

	return nil
}

// process parses and classifies each jsonlog line.
func (p *Pipeline) process(ctx context.Context, raw <-chan []byte) <-chan AnalyzedLogEvent {
	out := make(chan AnalyzedLogEvent, logStreamBufferLen)

	go func() {
		defer close(out)

		for {
			select {
			case <-ctx.Done():
				return
			case line, ok := <-raw:
				if !ok {
					return
				}

				event, emit := p.processLine(line)
				if !emit {
					continue
				}

				select {
				case out <- event:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return out
}

func (p *Pipeline) processLine(raw []byte) (AnalyzedLogEvent, bool) {
	if len(raw) > maxLogLineBytes {
		p.logger.Warn("dropping oversized log line", "bytes", len(raw), "limit", maxLogLineBytes)

		return AnalyzedLogEvent{}, false
	}

	parsed, ok := p.parser.Parse(raw)
	if !ok {
		return AnalyzedLogEvent{}, false
	}

	if !p.breaker.admit(p.now(), parsed, p.logger) {
		return AnalyzedLogEvent{}, false
	}

	return p.analyzer.Analyze(parsed), true
}

// batch accumulates analyzed events and, on each interval tick, sends them to the backend.
func (p *Pipeline) batch(ctx context.Context, events <-chan AnalyzedLogEvent) {
	ticker := time.NewTicker(p.cfg.PollInterval)
	defer ticker.Stop()

	var pending []AnalyzedLogEvent

	flush := func() {
		if len(pending) == 0 {
			return
		}

		if err := p.reporter.ReportLogs(ctx, time.Now().UTC(), pending); err != nil {
			p.logger.Error("log sending failed (discarding lines)", "error", err)
		}

		pending = nil
	}

	for {
		select {
		case event, ok := <-events:
			if !ok {
				flush()
				return
			}
			pending = append(pending, event)
		case <-ticker.C:
			flush()
		}
	}
}
