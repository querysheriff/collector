package postgres

import (
	"context"
	"fmt"
)

const databasesSQL = `
SELECT datname
FROM pg_catalog.pg_database
WHERE datistemplate = false
ORDER BY datname;
`

func (c *Client) CollectDatabaseNames(ctx context.Context) ([]string, error) {
	rows, err := c.pool.Query(ctx, databasesSQL)
	if err != nil {
		return nil, fmt.Errorf("query pg_database: %w", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if scanErr := rows.Scan(&name); scanErr != nil {
			return nil, fmt.Errorf("scan pg_database row: %w", scanErr)
		}
		names = append(names, name)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, fmt.Errorf("read pg_database rows: %w", rowsErr)
	}

	return names, nil
}
