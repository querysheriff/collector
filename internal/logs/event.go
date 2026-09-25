package logs

import (
	"time"

	querysheriffv1 "github.com/querysheriff/collector/gen/querysheriff/v1"
)

type ParsedLogEvent struct {
	OccurredAt      time.Time
	LogLevel        querysheriffv1.LogEvent_LogLevel
	PID             int32
	Username        string
	DatabaseName    string
	ApplicationName string
	QueryID         int64
	BackendType     string
	StateCode       string
	Message         string
	Detail          string
	Hint            string
	Context         string
	Statement       string
}

type AnalyzedLogEvent struct {
	ParsedLogEvent

	Classification  querysheriffv1.LogEvent_LogClassification
	StatementSample *StatementSample
}

type StatementSample struct {
	Query           string
	Parameters      []string
	DurationMs      float64
	ExplainPlanJSON string
	Tags            map[string]string
}
