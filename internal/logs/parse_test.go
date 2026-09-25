package logs_test

import (
	"testing"
	"time"

	querysheriffv1 "github.com/querysheriff/collector/gen/querysheriff/v1"

	"github.com/querysheriff/collector/internal/logs"
)

func TestParseFields(t *testing.T) {
	t.Parallel()

	p := logs.NewParser(time.UTC)
	line := `{"timestamp":"2026-06-14 12:00:00.123 UTC","user":"app_user","dbname":"app_prod",` +
		`"pid":15328,"error_severity":"ERROR","state_code":"23505",` +
		`"application_name":"svc","backend_type":"client backend","query_id":123456789,` +
		`"message":"duplicate key value","detail":"Key (id)=(1) already exists.",` +
		`"hint":"check the key","context":"SQL function foo","statement":"INSERT INTO t VALUES (1)"}`

	got, ok := p.Parse([]byte(line))
	if !ok {
		t.Fatalf("Parse ok=false for a valid jsonlog line")
	}

	wantTime := time.Date(2026, 6, 14, 12, 0, 0, 123_000_000, time.UTC)
	if !got.OccurredAt.Equal(wantTime) {
		t.Errorf("OccurredAt = %v, want %v", got.OccurredAt, wantTime)
	}
	checks := []struct {
		name string
		got  any
		want any
	}{
		{"Pid", got.PID, int32(15328)},
		{"Username", got.Username, "app_user"},
		{"Database", got.DatabaseName, "app_prod"},
		{"Application", got.ApplicationName, "svc"},
		{"BackendType", got.BackendType, "client backend"},
		{"StateCode", got.StateCode, "23505"},
		{"QueryID", got.QueryID, int64(123456789)},
		{"LogLevel", got.LogLevel, querysheriffv1.LogEvent_LOG_LEVEL_ERROR},
		{"Message", got.Message, "duplicate key value"},
		{"Detail", got.Detail, "Key (id)=(1) already exists."},
		{"Hint", got.Hint, "check the key"},
		{"Context", got.Context, "SQL function foo"},
		{"Statement", got.Statement, "INSERT INTO t VALUES (1)"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}

func TestParseSeverityMapping(t *testing.T) {
	t.Parallel()

	p := logs.NewParser(time.UTC)
	cases := map[string]querysheriffv1.LogEvent_LogLevel{
		"DEBUG1":  querysheriffv1.LogEvent_LOG_LEVEL_DEBUG,
		"DEBUG5":  querysheriffv1.LogEvent_LOG_LEVEL_DEBUG,
		"LOG":     querysheriffv1.LogEvent_LOG_LEVEL_LOG,
		"INFO":    querysheriffv1.LogEvent_LOG_LEVEL_INFO,
		"NOTICE":  querysheriffv1.LogEvent_LOG_LEVEL_NOTICE,
		"WARNING": querysheriffv1.LogEvent_LOG_LEVEL_WARNING,
		"ERROR":   querysheriffv1.LogEvent_LOG_LEVEL_ERROR,
		"FATAL":   querysheriffv1.LogEvent_LOG_LEVEL_FATAL,
		"PANIC":   querysheriffv1.LogEvent_LOG_LEVEL_PANIC,
		"BOGUS":   querysheriffv1.LogEvent_LOG_LEVEL_UNSPECIFIED,
	}
	for severity, want := range cases {
		line := `{"timestamp":"2026-06-14 12:00:00.000 UTC","pid":1,"error_severity":"` + severity + `","message":"x"}`
		got, ok := p.Parse([]byte(line))
		if !ok {
			t.Errorf("Parse ok=false for severity %q", severity)

			continue
		}
		if got.LogLevel != want {
			t.Errorf("severity %q → level %v, want %v", severity, got.LogLevel, want)
		}
	}
}

func TestParseUsesConfiguredTimezone(t *testing.T) {
	t.Parallel()

	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("timezone data unavailable: %v", err)
	}

	p := logs.NewParser(loc)
	line := `{"timestamp":"2026-06-14 12:00:00.000 UTC","pid":1,"error_severity":"LOG","message":"x"}`

	got, ok := p.Parse([]byte(line))
	if !ok {
		t.Fatalf("Parse ok=false")
	}

	// The printed "UTC" is dropped; the wall-clock time is read in loc.
	want := time.Date(2026, 6, 14, 12, 0, 0, 0, loc)
	if !got.OccurredAt.Equal(want) {
		t.Errorf("OccurredAt = %v, want %v", got.OccurredAt, want)
	}
}

func TestParseInvalidJSON(t *testing.T) {
	t.Parallel()

	p := logs.NewParser(time.UTC)
	if _, ok := p.Parse([]byte("this is not json")); ok {
		t.Errorf("Parse ok=true for a non-JSON line")
	}
	if _, ok := p.Parse([]byte(`{"timestamp":"2026-06-14 12:00:00.000 UTC"`)); ok {
		t.Errorf("Parse ok=true for a truncated JSON line")
	}
}

func TestParseMissingTimestamp(t *testing.T) {
	t.Parallel()

	p := logs.NewParser(time.UTC)
	got, ok := p.Parse([]byte(`{"pid":1,"error_severity":"LOG","message":"no timestamp"}`))
	if !ok {
		t.Fatalf("Parse ok=false")
	}
	if !got.OccurredAt.IsZero() {
		t.Errorf("OccurredAt = %v, want zero time", got.OccurredAt)
	}
}
