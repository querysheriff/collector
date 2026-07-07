package postgres

import "strings"

// IsNoiseStatement reports whether a statement is transaction-control or session-utility noise
// (BEGIN/COMMIT/SET/SHOW/...) that should be dropped from pg_stat_statements reporting.
func IsNoiseStatement(query string) bool {
	switch leadingKeyword(query) {
	case "BEGIN", "START", "COMMIT", "END", "ROLLBACK", "ABORT",
		"SAVEPOINT", "RELEASE", "DEALLOCATE",
		"SET", "RESET", "SHOW", "DISCARD",
		"LISTEN", "UNLISTEN", "NOTIFY", "LOAD":
		return true
	default:
		return false
	}
}

func leadingKeyword(query string) string {
	query = strings.TrimLeft(query, " \t\n\r\f\v")
	end := 0
	for end < len(query) {
		c := query[end]
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') {
			break
		}
		end++
	}
	return strings.ToUpper(query[:end])
}
