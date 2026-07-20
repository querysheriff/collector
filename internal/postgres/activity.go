package postgres

import (
	"context"
	"fmt"
	"time"

	sqltags "github.com/pgdozor/sqltags/go"
)

type ActivitySnapshot struct {
	PID             int32
	BackendStart    time.Time
	DatabaseName    *string
	UserName        *string
	ApplicationName *string
	State           *string
	WaitEventType   *string
	WaitEvent       *string
	XactStart       *time.Time
	QueryStart      *time.Time
	QueryID         *int64
	Query           *string
	BlockingPids    []int32
	LockWaitStart   *time.Time
	LockMode        *string

	// BlockedByPID is derived from BlockingPids by ResolveBlockedBy, not scanned.
	BlockedByPID int32

	// QueryTags are parsed from Query's leading comment, which is stripped from Query.
	QueryTags map[string]string
}

const activitySQL = `
SELECT a.pid,
       a.backend_start,
       a.datname AS database_name,
       a.usename AS user_name,
       a.application_name,
       a.state,
       a.wait_event_type,
       a.wait_event,
       a.xact_start,
       a.query_start,
       a.query_id,
       a.query,
       CASE WHEN l.pid IS NOT NULL OR a.wait_event_type = 'Lock' THEN pg_catalog.pg_blocking_pids(a.pid) ELSE ARRAY []::integer[] END AS blocking_pids,
       l.waitstart AS lock_wait_start,
       l.mode AS lock_mode
FROM pg_catalog.pg_stat_activity a
LEFT JOIN pg_catalog.pg_locks l ON l.pid = a.pid AND l.granted = FALSE
WHERE a.backend_type = 'client backend'
  AND a.state IN ('active', 'idle in transaction', 'idle in transaction (aborted)')
  AND a.xact_start IS NOT NULL
  AND a.xact_start <= clock_timestamp() - interval '1 second'
ORDER BY a.pid;
`

func (c *Client) CollectActivitySnapshots(ctx context.Context) ([]ActivitySnapshot, error) {
	rows, err := c.pool.Query(ctx, activitySQL)
	if err != nil {
		return nil, fmt.Errorf("query pg_stat_activity: %w", err)
	}
	defer rows.Close()

	var snapshots []ActivitySnapshot
	for rows.Next() {
		var s ActivitySnapshot
		if scanErr := rows.Scan(
			&s.PID, &s.BackendStart, &s.DatabaseName, &s.UserName, &s.ApplicationName,
			&s.State, &s.WaitEventType, &s.WaitEvent, &s.XactStart, &s.QueryStart,
			&s.QueryID, &s.Query, &s.BlockingPids, &s.LockWaitStart, &s.LockMode,
		); scanErr != nil {
			return nil, fmt.Errorf("scan pg_stat_activity row: %w", scanErr)
		}
		if s.Query != nil {
			stripped, tags := sqltags.Untag(*s.Query)
			s.QueryTags = tags
			s.Query = &stripped
		}
		snapshots = append(snapshots, s)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, fmt.Errorf("read pg_stat_activity rows: %w", rowsErr)
	}

	ResolveBlockedBy(snapshots)

	return snapshots, nil
}
