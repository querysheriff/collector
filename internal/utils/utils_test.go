package utils_test

import (
	"maps"
	"testing"

	"github.com/pgdozor/collector/internal/utils"
)

func TestParseSQLTagsValid(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		sql      string
		wantTags map[string]string
		wantSQL  string
	}{
		{
			name:     "single field",
			sql:      "/* service=user-api */ SELECT 1;",
			wantTags: map[string]string{"service": "user-api"},
			wantSQL:  "SELECT 1;",
		},
		{
			name:     "single field no spaces",
			sql:      "/*service=user-api*/SELECT 1;",
			wantTags: map[string]string{"service": "user-api"},
			wantSQL:  "SELECT 1;",
		},
		{
			name: "route with colons and slashes",
			sql:  "/* service=user-api request_id=123 route=GET:/v1/users/:id */\nSELECT id, email FROM users WHERE id = $1;",
			wantTags: map[string]string{
				"service":    "user-api",
				"request_id": "123",
				"route":      "GET:/v1/users/:id",
			},
			wantSQL: "SELECT id, email FROM users WHERE id = $1;",
		},
		{
			name: "operation and job id",
			sql:  "/*service=worker operation=deliver_email job_id=8921 */\nUPDATE email_delivery SET status = 'sent' WHERE id = $1;",
			wantTags: map[string]string{
				"service":   "worker",
				"operation": "deliver_email",
				"job_id":    "8921",
			},
			wantSQL: "UPDATE email_delivery SET status = 'sent' WHERE id = $1;",
		},
		{
			name: "code location and trace id",
			sql:  "/* code=userService.ts:42 trace_id=abc-123 */\nSELECT * FROM users WHERE id = $1;",
			wantTags: map[string]string{
				"code":     "userService.ts:42",
				"trace_id": "abc-123",
			},
			wantSQL: "SELECT * FROM users WHERE id = $1;",
		},
		{
			name:     "leading whitespace before comment",
			sql:      "  \n\t/* service=api */ SELECT 1;",
			wantTags: map[string]string{"service": "api"},
			wantSQL:  "SELECT 1;",
		},
		{
			name:     "irregular inner whitespace",
			sql:      "/*   service=api    request_id=7  */SELECT 1;",
			wantTags: map[string]string{"service": "api", "request_id": "7"},
			wantSQL:  "SELECT 1;",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tags, sql := utils.ParseSQLTags(tc.sql)
			if !maps.Equal(tags, tc.wantTags) {
				t.Errorf("tags = %v, want %v", tags, tc.wantTags)
			}
			if sql != tc.wantSQL {
				t.Errorf("sql = %q, want %q", sql, tc.wantSQL)
			}
		})
	}
}

func TestParseSQLTagsStripsNonTagComment(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		sql  string
	}{
		{"value with space", "/* service=user api */\nSELECT 1;"},
		{"value with equals", "/* service=user-api request_id=a=b */\nSELECT 1;"},
		{"uppercase key", "/* Service=user-api */\nSELECT 1;"},
		{"duplicate key", "/* service=user-api service=other */\nSELECT 1;"},
		{"empty comment", "/* */\nSELECT 1;"},
		{"value with asterisk", "/* service=a*b */\nSELECT 1;"},
		{"value with quote", "/* service='api' */\nSELECT 1;"},
		{"missing value", "/* service= */\nSELECT 1;"},
		{"key only", "/* service */\nSELECT 1;"},
		{"digit-leading key", "/* 1service=api */\nSELECT 1;"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tags, sql := utils.ParseSQLTags(tc.sql)
			if tags != nil {
				t.Errorf("tags = %v, want nil", tags)
			}
			if sql != "SELECT 1;" {
				t.Errorf("sql = %q, want %q", sql, "SELECT 1;")
			}
		})
	}
}

func TestParseSQLTagsNoLeadingComment(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		sql  string
	}{
		{"comment after sql", "SELECT /* service=user-api */ 1;"},
		{"no comment", "SELECT 1;"},
		{"unterminated comment", "/* service=user-api \nSELECT 1;"},
		{"line comment", "-- service=api\nSELECT 1;"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tags, sql := utils.ParseSQLTags(tc.sql)
			if tags != nil {
				t.Errorf("tags = %v, want nil", tags)
			}
			if sql != tc.sql {
				t.Errorf("sql = %q, want unchanged %q", sql, tc.sql)
			}
		})
	}
}
