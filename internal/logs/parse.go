package logs

import (
	"encoding/json"
	"strings"
	"time"

	pgdozorv1 "buf.build/gen/go/pgdozor/backend/protocolbuffers/go/pgdozor/v1"
)

// jsonLogRecord mirrors the subset of jsonlog fields we consume.
// Fields that are absent or empty are simply omitted by PostgreSQL and decode to zero values here.
// See https://www.postgresql.org/docs/15/runtime-config-logging.html#RUNTIME-CONFIG-LOGGING-JSONLOG-KEYS-VALUES.
type jsonLogRecord struct {
	Timestamp       string `json:"timestamp"`
	User            string `json:"user"`
	Database        string `json:"dbname"`
	Pid             int32  `json:"pid"`
	ErrorSeverity   string `json:"error_severity"`
	StateCode       string `json:"state_code"`
	Message         string `json:"message"`
	Detail          string `json:"detail"`
	Hint            string `json:"hint"`
	Context         string `json:"context"`
	Statement       string `json:"statement"`
	ApplicationName string `json:"application_name"`
	BackendType     string `json:"backend_type"`
	QueryID         int64  `json:"query_id"`
}

type Parser struct {
	tz *time.Location
}

func NewParser(tz *time.Location) *Parser {
	if tz == nil {
		tz = time.UTC
	}

	return &Parser{tz: tz}
}

// Parse decodes one jsonlog line into a ParsedLogEvent.
// ok is false when the line is not valid JSON (e.g. a truncated final line).
func (p *Parser) Parse(line []byte) (ParsedLogEvent, bool) {
	var rec jsonLogRecord
	if err := json.Unmarshal(line, &rec); err != nil {
		return ParsedLogEvent{}, false
	}

	return ParsedLogEvent{
		OccurredAt:      parseTimestamp(rec.Timestamp, p.tz),
		LogLevel:        mapSeverity(rec.ErrorSeverity),
		PID:             rec.Pid,
		Username:        rec.User,
		DatabaseName:    rec.Database,
		ApplicationName: rec.ApplicationName,
		QueryID:         rec.QueryID,
		BackendType:     rec.BackendType,
		StateCode:       rec.StateCode,
		Message:         rec.Message,
		Detail:          rec.Detail,
		Hint:            rec.Hint,
		Context:         rec.Context,
		Statement:       rec.Statement,
	}, true
}

// mapSeverity maps a jsonlog error_severity to the wire log level.
// DEBUG1–DEBUG5 collapse to DEBUG; any unrecognized severity becomes UNSPECIFIED.
func mapSeverity(severity string) pgdozorv1.LogEvent_LogLevel {
	if strings.HasPrefix(severity, "DEBUG") {
		return pgdozorv1.LogEvent_LOG_LEVEL_DEBUG
	}

	switch severity {
	case "INFO":
		return pgdozorv1.LogEvent_LOG_LEVEL_INFO
	case "NOTICE":
		return pgdozorv1.LogEvent_LOG_LEVEL_NOTICE
	case "WARNING":
		return pgdozorv1.LogEvent_LOG_LEVEL_WARNING
	case "ERROR":
		return pgdozorv1.LogEvent_LOG_LEVEL_ERROR
	case "LOG":
		return pgdozorv1.LogEvent_LOG_LEVEL_LOG
	case "FATAL":
		return pgdozorv1.LogEvent_LOG_LEVEL_FATAL
	case "PANIC":
		return pgdozorv1.LogEvent_LOG_LEVEL_PANIC
	default:
		return pgdozorv1.LogEvent_LOG_LEVEL_UNSPECIFIED
	}
}

// parseTimestamp parses a jsonlog "timestamp" (e.g. "2026-06-14 12:00:00.123 UTC") using the known log timezone.
func parseTimestamp(value string, tz *time.Location) time.Time {
	lastSpace := strings.LastIndex(value, " ")
	if lastSpace == -1 {
		return time.Time{}
	}

	// Drop the printed zone (for example "UTC" or "CEST") and interpret the wall-clock part in the known log timezone.
	ts, err := time.ParseInLocation("2006-01-02 15:04:05.999", value[:lastSpace], tz)
	if err != nil {
		return time.Time{}
	}

	return ts
}
