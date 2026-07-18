package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	minVersionNum      = 150000
	maxVersionNum      = 190000
	versionNumPerMajor = 10000
	connectTimeout     = 10 * time.Second
	maxPoolConns       = 5
)

type Client struct {
	pool *pgxpool.Pool

	// server_version_num
	VersionNum int
	// log_timezone
	LogTimezone *time.Location

	// cumulative pg_stat_statements counters from the previous collection, for computing deltas
	statementsState map[StatementIdentity]statementCounters
}

func Connect(ctx context.Context, dsn string) (*Client, error) {
	poolCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse postgres dsn: %w", err)
	}

	poolCfg.ConnConfig.ConnectTimeout = connectTimeout
	poolCfg.MinConns = 0
	poolCfg.MaxConns = maxPoolConns

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create postgres pool: %w", err)
	}

	var versionNum int
	err = pool.QueryRow(ctx, "SELECT current_setting('server_version_num')::int").Scan(&versionNum)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("query server version: %w", err)
	}

	if versionNum < minVersionNum || versionNum >= maxVersionNum {
		pool.Close()
		return nil, fmt.Errorf(
			"unsupported PostgreSQL version %d (supported: %d-%d)",
			versionNum/versionNumPerMajor,
			minVersionNum/versionNumPerMajor,
			maxVersionNum/versionNumPerMajor-1,
		)
	}

	var logTimezoneName string
	err = pool.QueryRow(ctx, "SELECT current_setting('log_timezone')").Scan(&logTimezoneName)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("query log_timezone: %w", err)
	}

	logTimezone, err := time.LoadLocation(logTimezoneName)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("load log timezone %q: %w", logTimezoneName, err)
	}

	return &Client{
		pool:        pool,
		VersionNum:  versionNum,
		LogTimezone: logTimezone,
	}, nil
}

func (c *Client) Close() {
	c.pool.Close()
}
