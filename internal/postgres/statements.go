package postgres

import (
	"context"
	"fmt"
)

type StatementIdentity struct {
	UserName     string
	DatabaseName string
	QueryID      int64
}

type StatementDelta struct {
	StatementIdentity

	Calls         int64
	Rows          int64
	TotalExecTime float64
	TotalIOTime   float64
}

type StatementText struct {
	StatementIdentity

	Query string
}

type statementCounters struct {
	calls         int64
	rows          int64
	totalExecTime float64
	totalIOTime   float64
}

// blkTimeSplitVersionNum is the first server_version_num (PostgreSQL 17) where
// pg_stat_statements splits block-IO timing into shared/local/temp columns.
const blkTimeSplitVersionNum = 170000

func statementsSQL(versionNum int) string {
	ioTime := "(s.blk_read_time + s.blk_write_time)"
	if versionNum >= blkTimeSplitVersionNum {
		ioTime = "(s.shared_blk_read_time + s.shared_blk_write_time + " +
			"s.local_blk_read_time + s.local_blk_write_time + " +
			"s.temp_blk_read_time + s.temp_blk_write_time)"
	}

	return `
SELECT r.rolname AS user_name,
       d.datname AS database_name,
       s.queryid,
       s.calls,
       s.rows,
       s.total_exec_time,
       ` + ioTime + ` AS total_io_time
FROM pg_stat_statements s
JOIN pg_database d ON d.oid = s.dbid
JOIN pg_roles r ON r.oid = s.userid
WHERE r.rolname NOT ILIKE '%pgdozor%'
  AND d.datname NOT ILIKE '%pgdozor%'
  AND s.toplevel
  AND s.queryid IS NOT NULL
  AND s.query IS NOT NULL;
`
}

func (c *Client) CollectStatementDeltas(ctx context.Context) ([]StatementDelta, error) {
	rows, err := c.pool.Query(ctx, statementsSQL(c.VersionNum))
	if err != nil {
		return nil, fmt.Errorf("query pg_stat_statements: %w", err)
	}
	defer rows.Close()

	var cumulative []StatementDelta
	for rows.Next() {
		var s StatementDelta
		if scanErr := rows.Scan(
			&s.UserName, &s.DatabaseName, &s.QueryID, &s.Calls, &s.Rows, &s.TotalExecTime,
			&s.TotalIOTime,
		); scanErr != nil {
			return nil, fmt.Errorf("scan pg_stat_statements row: %w", scanErr)
		}
		cumulative = append(cumulative, s)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, fmt.Errorf("read pg_stat_statements rows: %w", rowsErr)
	}

	newState := make(map[StatementIdentity]statementCounters, len(cumulative))
	for _, s := range cumulative {
		newState[s.StatementIdentity] = statementCounters{
			calls:         s.Calls,
			rows:          s.Rows,
			totalExecTime: s.TotalExecTime,
			totalIOTime:   s.TotalIOTime,
		}
	}

	prevState := c.statementsState
	c.statementsState = newState

	if prevState == nil || countersDecreased(prevState, newState) {
		return nil, nil
	}

	var deltas []StatementDelta
	for _, s := range cumulative {
		prev := prevState[s.StatementIdentity]
		s.Calls -= prev.calls
		s.Rows -= prev.rows
		s.TotalExecTime -= prev.totalExecTime
		s.TotalIOTime -= prev.totalIOTime
		if s.Calls > 0 {
			deltas = append(deltas, s)
		}
	}

	return deltas, nil
}

const statementTextsSQL = `
SELECT r.rolname AS user_name,
       d.datname AS database_name,
       s.queryid,
       s.query
FROM pg_stat_statements s
JOIN pg_database d ON d.oid = s.dbid
JOIN pg_roles r ON r.oid = s.userid
JOIN unnest($1::text[], $2::text[], $3::bigint[]) AS req(user_name, database_name, query_id)
  ON r.rolname = req.user_name
 AND d.datname = req.database_name
 AND s.queryid = req.query_id
WHERE s.toplevel;
`

func (c *Client) CollectStatementTexts(ctx context.Context, refs []StatementIdentity) ([]StatementText, error) {
	userNames := make([]string, len(refs))
	databaseNames := make([]string, len(refs))
	queryIDs := make([]int64, len(refs))
	for i, ref := range refs {
		userNames[i] = ref.UserName
		databaseNames[i] = ref.DatabaseName
		queryIDs[i] = ref.QueryID
	}

	rows, err := c.pool.Query(ctx, statementTextsSQL, userNames, databaseNames, queryIDs)
	if err != nil {
		return nil, fmt.Errorf("query statement texts: %w", err)
	}
	defer rows.Close()

	var texts []StatementText
	for rows.Next() {
		var t StatementText
		if scanErr := rows.Scan(&t.UserName, &t.DatabaseName, &t.QueryID, &t.Query); scanErr != nil {
			return nil, fmt.Errorf("scan statement text row: %w", scanErr)
		}
		texts = append(texts, t)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, fmt.Errorf("read statement text rows: %w", rowsErr)
	}

	return texts, nil
}

func countersDecreased(prev, cur map[StatementIdentity]statementCounters) bool {
	for key, c := range cur {
		p, ok := prev[key]
		if !ok {
			continue
		}
		if c.calls < p.calls || c.rows < p.rows || c.totalExecTime < p.totalExecTime ||
			c.totalIOTime < p.totalIOTime {
			return true
		}
	}

	return false
}
