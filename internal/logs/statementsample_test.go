package logs_test

import (
	"maps"
	"slices"
	"strings"
	"testing"

	pgdozorv1 "buf.build/gen/go/pgdozor/backend/protocolbuffers/go/pgdozor/v1"

	"github.com/pgdozor/collector/internal/logs"
)

// A log_min_duration_statement line yields a statement sample with its duration.
func TestStatementSampleLogMinDuration(t *testing.T) {
	t.Parallel()

	got := logs.NewAnalyzer().Analyze(logs.ParsedLogEvent{Message: "duration: 1234.5 ms  statement: SELECT 1"})

	if got.Classification != pgdozorv1.LogEvent_LOG_CLASSIFICATION_STATEMENT_DURATION {
		t.Fatalf("classification = %v, want STATEMENT_DURATION", got.Classification)
	}
	if got.StatementSample == nil {
		t.Fatalf("StatementSample = nil, want a sample")
	}
	if got.StatementSample.Query != "SELECT 1" || got.StatementSample.DurationMs != 1234.5 {
		t.Errorf("sample = {Query:%q DurationMs:%v}, want {SELECT 1, 1234.5}",
			got.StatementSample.Query, got.StatementSample.DurationMs)
	}
}

// Bind parameters from the DETAIL line are attached to the sample; NULL becomes the empty string.
func TestStatementSampleBindParameters(t *testing.T) {
	t.Parallel()

	got := logs.NewAnalyzer().Analyze(logs.ParsedLogEvent{
		Message: "duration: 2.5 ms  execute <unnamed>: SELECT $1, $2",
		Detail:  "parameters: $1 = 'foo', $2 = NULL",
	})

	if got.StatementSample == nil {
		t.Fatalf("StatementSample = nil, want a sample")
	}
	if got.StatementSample.Query != "SELECT $1, $2" {
		t.Errorf("Query = %q, want %q", got.StatementSample.Query, "SELECT $1, $2")
	}
	want := []string{"foo", ""}
	if len(got.StatementSample.Parameters) != len(want) ||
		got.StatementSample.Parameters[0] != want[0] || got.StatementSample.Parameters[1] != want[1] {
		t.Errorf("Parameters = %q, want %q", got.StatementSample.Parameters, want)
	}
}

func TestStatementSampleUnquotesParameters(t *testing.T) {
	t.Parallel()

	want := []string{"O'Brien", "'"}

	detail := logs.NewAnalyzer().Analyze(logs.ParsedLogEvent{
		Message: "duration: 2.5 ms  execute <unnamed>: SELECT $1, $2",
		Detail:  "parameters: $1 = 'O''Brien', $2 = ''''",
	})
	if detail.StatementSample == nil || !slices.Equal(detail.StatementSample.Parameters, want) {
		t.Errorf("DETAIL path Parameters = %q, want %q", params(detail), want)
	}

	plan := "{\n" +
		`  "Query Text": "SELECT $1, $2",` + "\n" +
		`  "Query Parameters": "$1 = 'O''Brien', $2 = ''''",` + "\n" +
		`  "Plan": {"Node Type": "Result"}` + "\n" +
		"}"
	explain := logs.NewAnalyzer().Analyze(logs.ParsedLogEvent{Message: "duration: 5.0 ms  plan:\n" + plan})
	if explain.StatementSample == nil || !slices.Equal(explain.StatementSample.Parameters, want) {
		t.Errorf("auto_explain path Parameters = %q, want %q", params(explain), want)
	}
}

func params(e logs.AnalyzedLogEvent) []string {
	if e.StatementSample == nil {
		return nil
	}

	return e.StatementSample.Parameters
}

// The bind/parse steps of the extended query protocol are not executions, so they carry no sample.
func TestStatementSampleSkipsBindStep(t *testing.T) {
	t.Parallel()

	got := logs.NewAnalyzer().Analyze(logs.ParsedLogEvent{Message: "duration: 1.0 ms  bind foo: SELECT $1"})

	if got.Classification != pgdozorv1.LogEvent_LOG_CLASSIFICATION_STATEMENT_DURATION {
		t.Errorf("classification = %v, want STATEMENT_DURATION", got.Classification)
	}
	if got.StatementSample != nil {
		t.Errorf("StatementSample = %+v, want nil for a bind step", got.StatementSample)
	}
}

// auto_explain JSON output is classified and preserved verbatim in the sample.
func TestStatementSampleAutoExplainJSON(t *testing.T) {
	t.Parallel()

	plan := `{"Query Text": "SELECT 1", "Plan": {"Node Type": "Result"}}`
	got := logs.NewAnalyzer().Analyze(logs.ParsedLogEvent{Message: "duration: 5.234 ms  plan:\n" + plan})

	if got.Classification != pgdozorv1.LogEvent_LOG_CLASSIFICATION_STATEMENT_AUTO_EXPLAIN {
		t.Fatalf("classification = %v, want STATEMENT_AUTO_EXPLAIN", got.Classification)
	}
	if got.StatementSample == nil {
		t.Fatalf("StatementSample = nil, want a sample")
	}
	if got.StatementSample.Query != "SELECT 1" || got.StatementSample.DurationMs != 5.234 {
		t.Errorf("sample = {Query:%q DurationMs:%v}, want {SELECT 1, 5.234}",
			got.StatementSample.Query, got.StatementSample.DurationMs)
	}
	if !strings.Contains(got.StatementSample.ExplainPlanJSON, "Query Text") {
		t.Errorf("ExplainPlanJSON = %q, want the raw plan preserved", got.StatementSample.ExplainPlanJSON)
	}
}

// auto_explain text output is parsed into query text plus the preserved plan body.
func TestStatementSampleAutoExplainText(t *testing.T) {
	t.Parallel()

	plan := "duration: 5.234 ms  plan:\nQuery Text: SELECT 1\n" +
		"Result  (cost=0.00..0.01 rows=1 width=4) (actual time=0.001..0.002 rows=1 loops=1)"
	got := logs.NewAnalyzer().Analyze(logs.ParsedLogEvent{Message: plan})

	if got.Classification != pgdozorv1.LogEvent_LOG_CLASSIFICATION_STATEMENT_AUTO_EXPLAIN {
		t.Fatalf("classification = %v, want STATEMENT_AUTO_EXPLAIN", got.Classification)
	}
	if got.StatementSample == nil {
		t.Fatalf("StatementSample = nil, want a sample")
	}
	if got.StatementSample.Query != "SELECT 1" {
		t.Errorf("Query = %q, want %q", got.StatementSample.Query, "SELECT 1")
	}
	if !strings.Contains(got.StatementSample.ExplainPlanJSON, "cost=") {
		t.Errorf("ExplainPlanJSON = %q, want the preserved plan body", got.StatementSample.ExplainPlanJSON)
	}
}

// A slow extended-protocol execute with a multi-line query keeps the whole query text and its bind params.
func TestStatementSampleMultilineExecuteWithParams(t *testing.T) {
	t.Parallel()

	query := "SELECT\n" +
		"\ta.user_id,\n" +
		"\tu.name,\n" +
		"\tcount(*) AS article_count,\n" +
		"\tavg(a.view_count) AS avg_views\n" +
		"FROM articles a JOIN users u ON u.id = a.user_id\n" +
		"WHERE a.status = $1 AND a.topic = $2\n" +
		"GROUP BY a.user_id, u.name\n" +
		"ORDER BY article_count DESC\n" +
		"LIMIT $5::int"
	got := logs.NewAnalyzer().Analyze(logs.ParsedLogEvent{
		Message: "duration: 9652.087 ms  execute <unnamed>: " + query + "\n",
		Detail:  "Parameters: $1 = 'draft', $2 = 'tech', $3 = '100', $4 = '2', $5 = '50'",
	})

	if got.Classification != pgdozorv1.LogEvent_LOG_CLASSIFICATION_STATEMENT_DURATION {
		t.Fatalf("classification = %v, want STATEMENT_DURATION", got.Classification)
	}
	if got.StatementSample == nil {
		t.Fatalf("StatementSample = nil, want a sample")
	}
	if got.StatementSample.Query != query {
		t.Errorf("Query = %q, want the full multi-line statement %q", got.StatementSample.Query, query)
	}
	if got.StatementSample.DurationMs != 9652.087 {
		t.Errorf("DurationMs = %v, want 9652.087", got.StatementSample.DurationMs)
	}
	want := []string{"draft", "tech", "100", "2", "50"}
	if !slices.Equal(got.StatementSample.Parameters, want) {
		t.Errorf("Parameters = %q, want %q", got.StatementSample.Parameters, want)
	}
}

// A slow auto_explain JSON line with a multi-line query keeps the query text, params, and raw plan.
func TestStatementSampleMultilineAutoExplainJSON(t *testing.T) {
	t.Parallel()

	plan := "{\n" +
		`  "Query Text": "SELECT\n\ta.user_id,\n\tcount(*)\nFROM articles a\nGROUP BY a.user_id",` + "\n" +
		`  "Query Parameters": "$1 = 'draft'",` + "\n" +
		`  "Plan": {"Node Type": "Aggregate"}` + "\n" +
		"}"
	got := logs.NewAnalyzer().Analyze(logs.ParsedLogEvent{Message: "duration: 9652.075 ms  plan:\n" + plan})

	if got.Classification != pgdozorv1.LogEvent_LOG_CLASSIFICATION_STATEMENT_AUTO_EXPLAIN {
		t.Fatalf("classification = %v, want STATEMENT_AUTO_EXPLAIN", got.Classification)
	}
	if got.StatementSample == nil {
		t.Fatalf("StatementSample = nil, want a sample")
	}
	wantQuery := "SELECT\n\ta.user_id,\n\tcount(*)\nFROM articles a\nGROUP BY a.user_id"
	if got.StatementSample.Query != wantQuery {
		t.Errorf("Query = %q, want %q", got.StatementSample.Query, wantQuery)
	}
	if got.StatementSample.DurationMs != 9652.075 {
		t.Errorf("DurationMs = %v, want 9652.075", got.StatementSample.DurationMs)
	}
	if !slices.Equal(got.StatementSample.Parameters, []string{"draft"}) {
		t.Errorf("Parameters = %q, want [draft]", got.StatementSample.Parameters)
	}
	if got.StatementSample.ExplainPlanJSON != plan {
		t.Errorf("ExplainPlanJSON = %q, want the raw plan preserved verbatim", got.StatementSample.ExplainPlanJSON)
	}
}

// A slow simple-protocol statement with a multi-line query keeps the whole query text and has no params.
func TestStatementSampleMultilineStatement(t *testing.T) {
	t.Parallel()

	query := "SELECT\n" +
		"\ta.user_id,\n" +
		"\tu.name,\n" +
		"\tcount(*) AS article_count,\n" +
		"\tavg(a.view_count) AS avg_views\n" +
		"FROM articles a JOIN users u ON u.id = a.user_id\n" +
		"ORDER BY article_count DESC\n" +
		"LIMIT 50\n" +
		";"
	got := logs.NewAnalyzer().Analyze(logs.ParsedLogEvent{Message: "duration: 9657.875 ms  statement: " + query})

	if got.Classification != pgdozorv1.LogEvent_LOG_CLASSIFICATION_STATEMENT_DURATION {
		t.Fatalf("classification = %v, want STATEMENT_DURATION", got.Classification)
	}
	if got.StatementSample == nil {
		t.Fatalf("StatementSample = nil, want a sample")
	}
	if got.StatementSample.Query != query {
		t.Errorf("Query = %q, want the full multi-line statement %q", got.StatementSample.Query, query)
	}
	if got.StatementSample.DurationMs != 9657.875 {
		t.Errorf("DurationMs = %v, want 9657.875", got.StatementSample.DurationMs)
	}
	if len(got.StatementSample.Parameters) != 0 {
		t.Errorf("Parameters = %q, want none", got.StatementSample.Parameters)
	}
}

// A leading metadata comment on a log_min_duration statement is parsed into tags
// and stripped from the reported query.
func TestStatementSampleLogMinDurationTags(t *testing.T) {
	t.Parallel()

	got := logs.NewAnalyzer().Analyze(logs.ParsedLogEvent{
		Message: "duration: 12.5 ms  statement: /* service=user-api request_id=123 */ SELECT 1",
	})

	if got.StatementSample == nil {
		t.Fatalf("StatementSample = nil, want a sample")
	}
	if got.StatementSample.Query != "SELECT 1" {
		t.Errorf("Query = %q, want %q", got.StatementSample.Query, "SELECT 1")
	}
	want := map[string]string{"service": "user-api", "request_id": "123"}
	if !maps.Equal(got.StatementSample.Tags, want) {
		t.Errorf("Tags = %v, want %v", got.StatementSample.Tags, want)
	}
}

// auto_explain output carries its tags too, while the raw plan JSON is preserved verbatim.
func TestStatementSampleAutoExplainTags(t *testing.T) {
	t.Parallel()

	plan := `{"Query Text": "/* service=worker job_id=8921 */ SELECT 1", "Plan": {"Node Type": "Result"}}`
	got := logs.NewAnalyzer().Analyze(logs.ParsedLogEvent{Message: "duration: 5.0 ms  plan:\n" + plan})

	if got.StatementSample == nil {
		t.Fatalf("StatementSample = nil, want a sample")
	}
	if got.StatementSample.Query != "SELECT 1" {
		t.Errorf("Query = %q, want %q", got.StatementSample.Query, "SELECT 1")
	}
	want := map[string]string{"service": "worker", "job_id": "8921"}
	if !maps.Equal(got.StatementSample.Tags, want) {
		t.Errorf("Tags = %v, want %v", got.StatementSample.Tags, want)
	}
	if !strings.Contains(got.StatementSample.ExplainPlanJSON, "/* service=worker") {
		t.Errorf("ExplainPlanJSON = %q, want the raw plan preserved", got.StatementSample.ExplainPlanJSON)
	}
}

// A query with no metadata comment yields no tags and is reported unchanged.
func TestStatementSampleNoTags(t *testing.T) {
	t.Parallel()

	got := logs.NewAnalyzer().Analyze(logs.ParsedLogEvent{Message: "duration: 1.0 ms  statement: SELECT 1"})

	if got.StatementSample == nil {
		t.Fatalf("StatementSample = nil, want a sample")
	}
	if got.StatementSample.Query != "SELECT 1" {
		t.Errorf("Query = %q, want %q", got.StatementSample.Query, "SELECT 1")
	}
	if got.StatementSample.Tags != nil {
		t.Errorf("Tags = %v, want nil", got.StatementSample.Tags)
	}
}
