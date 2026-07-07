package postgres_test

import (
	"testing"

	"github.com/pgdozor/collector/internal/postgres"
)

func TestIsNoiseStatement(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		query string
		want  bool
	}{
		{"begin", "BEGIN", true},
		{"commit lowercase", "commit", true},
		{"set with args", "SET client_encoding TO $1", true},
		{"leading whitespace", "  \t\nROLLBACK", true},
		{"savepoint", "SAVEPOINT sp1", true},
		{"discard all", "DISCARD ALL", true},
		{"release", "RELEASE SAVEPOINT sp1", true},
		{"select not noise", "SELECT * FROM users", false},
		{"insert not noise", "INSERT INTO t VALUES ($1)", false},
		{"update not noise", "UPDATE t SET x = 1", false},
		{"empty", "", false},
		{"keyword is only a prefix", "BEGINNER SELECT 1", false},
		{"leading non-letter", "/* tag */ COMMIT", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := postgres.IsNoiseStatement(tc.query); got != tc.want {
				t.Errorf("IsNoiseStatement(%q) = %v, want %v", tc.query, got, tc.want)
			}
		})
	}
}
