package app

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/querysheriff/collector/internal/backend"
	"github.com/querysheriff/collector/internal/config"
	"github.com/querysheriff/collector/internal/logs"
	"github.com/querysheriff/collector/internal/postgres"
	"github.com/querysheriff/collector/internal/schedule"
)

const (
	activityInterval         = 1 * time.Second
	statementsInterval       = 1 * time.Minute
	logsInterval             = 10 * time.Second
	healthInterval           = 1 * time.Minute
	logPipelineRetryInterval = 10 * time.Second
)

func Run(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	pg, err := postgres.Connect(ctx, cfg.Postgres)
	if err != nil {
		return err
	}
	defer pg.Close()

	logger.Info("querysheriff collector started",
		"postgres_version_num", pg.VersionNum,
	)

	bc := backend.NewClient(cfg)

	var wg sync.WaitGroup
	if cfg.Logs.Path != "" {
		startLogPipeline(ctx, &wg, cfg, bc, logger, pg)
	}

	var sched schedule.Scheduler
	sched.Add(activityInterval, func(ctx context.Context, now time.Time) {
		if err := collectAndSendActivity(ctx, bc, pg, now); err != nil {
			logger.Error("activity snapshot failed", "error", err)
		}
	})
	sched.Add(statementsInterval, func(ctx context.Context, now time.Time) {
		if err := collectAndSendStatements(ctx, bc, pg, now); err != nil {
			logger.Error("statement delta failed", "error", err)
		}
	})
	sched.Add(healthInterval, func(ctx context.Context, now time.Time) {
		if err := collectAndSendHealth(ctx, bc, pg, now); err != nil {
			logger.Error("health check failed", "error", err)
		}
	})

	sched.Run(ctx)

	wg.Wait()
	logger.Info("collector stopped")
	return nil
}

func startLogPipeline(
	ctx context.Context,
	wg *sync.WaitGroup,
	cfg config.Config,
	bc *backend.Client,
	logger *slog.Logger,
	pg *postgres.Client,
) {
	pipeline := logs.NewPipeline(
		logs.PipelineConfig{
			LogTimezone:        pg.LogTimezone,
			DeleteRotatedFiles: cfg.Logs.DeleteRotated,
			Path:               cfg.Logs.Path,
			PollInterval:       logsInterval,
		},
		logger,
		bc,
	)

	wg.Go(func() {
		runLogPipeline(ctx, pipeline, logger)
	})

	logger.Info("log collection started",
		"path", cfg.Logs.Path,
	)
}

// runLogPipeline supervises the log pipeline. pipeline.Run returns nil only on shutdown
// (ctx cancelled) and non-nil when tail setup fails, so a setup failure is retried rather than
// permanently and silently disabling log collection while the other streams keep flowing.
func runLogPipeline(ctx context.Context, pipeline *logs.Pipeline, logger *slog.Logger) {
	for {
		if err := pipeline.Run(ctx); err != nil {
			logger.Error("log pipeline stopped, retrying", "error", err, "retry_in", logPipelineRetryInterval)
		} else {
			return
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(logPipelineRetryInterval):
		}
	}
}

func collectAndSendActivity(
	ctx context.Context,
	bc *backend.Client,
	pg *postgres.Client,
	collectedAt time.Time,
) error {
	snapshots, err := pg.CollectActivitySnapshots(ctx)
	if err != nil {
		return err
	}

	if reportErr := bc.ReportActivity(ctx, collectedAt, snapshots); reportErr != nil {
		return reportErr
	}

	return nil
}

func collectAndSendStatements(
	ctx context.Context,
	bc *backend.Client,
	pg *postgres.Client,
	collectedAt time.Time,
) error {
	deltas, err := pg.CollectStatementDeltas(ctx)
	if err != nil {
		return err
	}

	unknown, err := bc.ReportStatements(ctx, collectedAt, deltas)
	if err != nil {
		return err
	}

	if len(unknown) == 0 {
		return nil
	}

	texts, err := pg.CollectStatementTexts(ctx, unknown)
	if err != nil {
		return err
	}

	if len(texts) == 0 {
		return nil
	}

	return bc.ReportStatementTexts(ctx, texts)
}

func collectAndSendHealth(
	ctx context.Context,
	bc *backend.Client,
	pg *postgres.Client,
	collectedAt time.Time,
) error {
	databases, err := pg.CollectDatabaseNames(ctx)
	if err != nil {
		return err
	}

	if reportErr := bc.ReportHealth(ctx, collectedAt, databases); reportErr != nil {
		return reportErr
	}

	return nil
}
