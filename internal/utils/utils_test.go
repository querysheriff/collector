package utils_test

import (
	"testing"

	"github.com/pgdozor/collector/internal/utils"
)

func TestStripLeadingComments(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		sql  string
		want string
	}{
		{"no comment", "SELECT 1;", "SELECT 1;"},
		{"single block", "/* service=api */ SELECT 1;", "SELECT 1;"},
		{"block no spaces", "/*service=api*/SELECT 1;", "SELECT 1;"},
		{"multiple blocks", "/* a */ /* b */ SELECT 1;", "SELECT 1;"},
		{"line comment", "-- note\nSELECT 1;", "SELECT 1;"},
		{"block then line", "/* a */ -- note\nSELECT 1;", "SELECT 1;"},
		{"line then block", "-- note\n/* a */ SELECT 1;", "SELECT 1;"},
		{"leading whitespace", "  \n\t/* a */ SELECT 1;", "SELECT 1;"},
		{"comment after sql untouched", "SELECT /* x */ 1;", "SELECT /* x */ 1;"},
		{"unterminated block", "/* foo SELECT 1;", "/* foo SELECT 1;"},
		{"line comment to end", "-- just a note", "-- just a note"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := utils.StripLeadingComments(tc.sql); got != tc.want {
				t.Errorf("StripLeadingComments(%q) = %q, want %q", tc.sql, got, tc.want)
			}
		})
	}
}
