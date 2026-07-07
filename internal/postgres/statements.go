package postgres

import (
	"context"
	"fmt"

	"github.com/pgdozor/collector/internal/utils"
)

type StatementDelta struct {
	UserName        string
	DatabaseName    string
	QueryID         int64
	Query           string
	Calls           int64
	Rows            int64
	TotalExecTime   float64
	SharedBlksRead  int64
	TempBlksWritten int64
}

type statementKey struct {
	databaseName string
	userName     string
	queryID      int64
}

type statementCounters struct {
	calls           int64
	rows            int64
	totalExecTime   float64
	sharedBlksRead  int64
	tempBlksWritten int64
}

const statementsSQL = `
SELECT r.rolname AS user_name,
       d.datname AS database_name,
       s.queryid,
       s.query,
       s.calls,
       s.rows,
       s.total_exec_time,
       s.shared_blks_read,
       s.temp_blks_written
FROM pg_stat_statements s
JOIN pg_database d ON d.oid = s.dbid
JOIN pg_roles r ON r.oid = s.userid
WHERE r.rolname NOT ILIKE '%pgdozor%'
  AND d.datname NOT ILIKE '%pgdozor%'
  AND s.toplevel
  AND s.queryid IS NOT NULL
  AND s.query IS NOT NULL;
`

func (c *Client) CollectStatementDeltas(ctx context.Context) ([]StatementDelta, error) {
	rows, err := c.pool.Query(ctx, statementsSQL)
	if err != nil {
		return nil, fmt.Errorf("query pg_stat_statements: %w", err)
	}
	defer rows.Close()

	var cumulative []StatementDelta
	for rows.Next() {
		var s StatementDelta
		if scanErr := rows.Scan(
			&s.UserName, &s.DatabaseName, &s.QueryID, &s.Query, &s.Calls, &s.Rows, &s.TotalExecTime,
			&s.SharedBlksRead, &s.TempBlksWritten,
		); scanErr != nil {
			return nil, fmt.Errorf("scan pg_stat_statements row: %w", scanErr)
		}
		_, s.Query = utils.ParseSQLTags(s.Query)
		if IsNoiseStatement(s.Query) {
			continue
		}
		cumulative = append(cumulative, s)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, fmt.Errorf("read pg_stat_statements rows: %w", rowsErr)
	}

	newState := make(map[statementKey]statementCounters, len(cumulative))
	for _, s := range cumulative {
		newState[statementKey{s.DatabaseName, s.UserName, s.QueryID}] = statementCounters{
			calls:           s.Calls,
			rows:            s.Rows,
			totalExecTime:   s.TotalExecTime,
			sharedBlksRead:  s.SharedBlksRead,
			tempBlksWritten: s.TempBlksWritten,
		}
	}

	prevState := c.statementsState
	c.statementsState = newState

	if prevState == nil || countersDecreased(prevState, newState) {
		return nil, nil
	}

	var deltas []StatementDelta
	for _, s := range cumulative {
		prev := prevState[statementKey{s.DatabaseName, s.UserName, s.QueryID}]
		s.Calls -= prev.calls
		s.Rows -= prev.rows
		s.TotalExecTime -= prev.totalExecTime
		s.SharedBlksRead -= prev.sharedBlksRead
		s.TempBlksWritten -= prev.tempBlksWritten
		if s.Calls > 0 {
			deltas = append(deltas, s)
		}
	}

	return deltas, nil
}

func countersDecreased(prev, cur map[statementKey]statementCounters) bool {
	for key, c := range cur {
		p, ok := prev[key]
		if !ok {
			continue
		}
		if c.calls < p.calls || c.rows < p.rows || c.totalExecTime < p.totalExecTime ||
			c.sharedBlksRead < p.sharedBlksRead || c.tempBlksWritten < p.tempBlksWritten {
			return true
		}
	}

	return false
}
