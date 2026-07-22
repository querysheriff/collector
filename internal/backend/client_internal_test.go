package backend

import (
	"testing"
	"time"

	"github.com/querysheriff/collector/internal/logs"
	"github.com/querysheriff/collector/internal/postgres"
)

// A fully-populated activity snapshot maps every field onto its named proto field.
func TestActivitySnapshotsToProto(t *testing.T) {
	t.Parallel()

	backendStart := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	xactStart := time.Date(2026, 1, 2, 3, 4, 6, 0, time.UTC)
	queryStart := time.Date(2026, 1, 2, 3, 4, 7, 0, time.UTC)
	lockWaitStart := time.Date(2026, 1, 2, 3, 4, 8, 0, time.UTC)

	snap := postgres.ActivitySnapshot{
		PID:             4242,
		BackendStart:    backendStart,
		DatabaseName:    new("app_prod"),
		UserName:        new("app_user"),
		ApplicationName: new("web"),
		State:           new("active"),
		WaitEventType:   new("Lock"),
		WaitEvent:       new("relation"),
		XactStart:       &xactStart,
		QueryStart:      &queryStart,
		QueryID:         new(int64(987654321)),
		Query:           new("SELECT 1"),
		LockWaitStart:   &lockWaitStart,
		LockMode:        new("ExclusiveLock"),
		BlockedByPID:    99,
		QueryTags:       map[string]string{"app": "web"},
	}

	got := activitySnapshotsToProto([]postgres.ActivitySnapshot{snap})
	if len(got) != 1 {
		t.Fatalf("got %d rows, want 1", len(got))
	}
	p := got[0]

	if p.GetPid() != 4242 {
		t.Errorf("Pid = %d, want 4242", p.GetPid())
	}
	if !p.GetBackendStart().AsTime().Equal(backendStart) {
		t.Errorf("BackendStart = %v, want %v", p.GetBackendStart().AsTime(), backendStart)
	}
	if p.GetDatabaseName() != "app_prod" {
		t.Errorf("DatabaseName = %q, want app_prod", p.GetDatabaseName())
	}
	if p.GetUserName() != "app_user" {
		t.Errorf("UserName = %q, want app_user", p.GetUserName())
	}
	if p.GetApplicationName() != "web" {
		t.Errorf("ApplicationName = %q, want web", p.GetApplicationName())
	}
	if p.GetState() != "active" {
		t.Errorf("State = %q, want active", p.GetState())
	}
	if p.GetWaitEventType() != "Lock" {
		t.Errorf("WaitEventType = %q, want Lock", p.GetWaitEventType())
	}
	if p.GetWaitEvent() != "relation" {
		t.Errorf("WaitEvent = %q, want relation", p.GetWaitEvent())
	}
	if !p.GetXactStart().AsTime().Equal(xactStart) {
		t.Errorf("XactStart = %v, want %v", p.GetXactStart().AsTime(), xactStart)
	}
	if !p.GetQueryStart().AsTime().Equal(queryStart) {
		t.Errorf("QueryStart = %v, want %v", p.GetQueryStart().AsTime(), queryStart)
	}
	if p.GetQueryId() != 987654321 {
		t.Errorf("QueryId = %d, want 987654321", p.GetQueryId())
	}
	if p.GetQuery() != "SELECT 1" {
		t.Errorf("Query = %q, want SELECT 1", p.GetQuery())
	}
	if !p.GetLockWaitStart().AsTime().Equal(lockWaitStart) {
		t.Errorf("LockWaitStart = %v, want %v", p.GetLockWaitStart().AsTime(), lockWaitStart)
	}
	if p.GetLockMode() != "ExclusiveLock" {
		t.Errorf("LockMode = %q, want ExclusiveLock", p.GetLockMode())
	}
	if p.GetBlockedByPid() != 99 {
		t.Errorf("BlockedByPid = %d, want 99", p.GetBlockedByPid())
	}
	if p.GetQueryTags()["app"] != "web" {
		t.Errorf("QueryTags = %v, want app=web", p.GetQueryTags())
	}
}

// Nil optional fields deref to their zero value, and absent timestamps map to nil (not epoch 0).
func TestActivitySnapshotsToProtoNilOptionals(t *testing.T) {
	t.Parallel()

	got := activitySnapshotsToProto([]postgres.ActivitySnapshot{{PID: 1}})
	p := got[0]

	if p.GetDatabaseName() != "" || p.GetUserName() != "" || p.GetQuery() != "" {
		t.Errorf("nil string pointers should deref to empty, got db=%q user=%q query=%q",
			p.GetDatabaseName(), p.GetUserName(), p.GetQuery())
	}
	if p.GetQueryId() != 0 {
		t.Errorf("nil QueryID should deref to 0, got %d", p.GetQueryId())
	}
	if p.GetXactStart() != nil || p.GetQueryStart() != nil || p.GetLockWaitStart() != nil {
		t.Errorf("nil *time.Time should map to nil timestamp, got xact=%v query=%v lock=%v",
			p.GetXactStart(), p.GetQueryStart(), p.GetLockWaitStart())
	}
}

// Each statement-delta field lands in the matching proto field.
func TestStatementDeltasToProto(t *testing.T) {
	t.Parallel()

	delta := postgres.StatementDelta{
		StatementIdentity: postgres.StatementIdentity{
			UserName:     "app_user",
			DatabaseName: "app_prod",
			QueryID:      123,
		},
		Calls:         10,
		Rows:          20,
		TotalExecTime: 30.5,
		TotalIOTime:   40.5,
	}

	p := statementDeltasToProto([]postgres.StatementDelta{delta})[0]

	switch {
	case p.GetUserName() != "app_user":
		t.Errorf("UserName = %q", p.GetUserName())
	case p.GetDatabaseName() != "app_prod":
		t.Errorf("DatabaseName = %q", p.GetDatabaseName())
	case p.GetQueryId() != 123:
		t.Errorf("QueryId = %d", p.GetQueryId())
	case p.GetCalls() != 10:
		t.Errorf("Calls = %d", p.GetCalls())
	case p.GetRows() != 20:
		t.Errorf("Rows = %d", p.GetRows())
	case p.GetTotalExecTime() != 30.5:
		t.Errorf("TotalExecTime = %v", p.GetTotalExecTime())
	case p.GetTotalIoTime() != 40.5:
		t.Errorf("TotalIoTime = %v", p.GetTotalIoTime())
	}
}

func TestStatementTextsToProto(t *testing.T) {
	t.Parallel()

	text := postgres.StatementText{
		StatementIdentity: postgres.StatementIdentity{
			UserName:     "app_user",
			DatabaseName: "app_prod",
			QueryID:      123,
		},
		Query: "SELECT * FROM users WHERE id = $1",
	}

	p := statementTextsToProto([]postgres.StatementText{text})[0]

	switch {
	case p.GetIdentity().GetUserName() != "app_user":
		t.Errorf("UserName = %q", p.GetIdentity().GetUserName())
	case p.GetIdentity().GetDatabaseName() != "app_prod":
		t.Errorf("DatabaseName = %q", p.GetIdentity().GetDatabaseName())
	case p.GetIdentity().GetQueryId() != 123:
		t.Errorf("QueryId = %d", p.GetIdentity().GetQueryId())
	case p.GetQuery() != "SELECT * FROM users WHERE id = $1":
		t.Errorf("Query = %q", p.GetQuery())
	}
}

// A log event without a sample maps StatementSample to nil; with one, the sample reuses the
// event's OccurredAt. A zero OccurredAt maps to a nil timestamp rather than epoch 0.
func TestLogEventsToProtoStatementSample(t *testing.T) {
	t.Parallel()

	occurred := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	noSample := logs.AnalyzedLogEvent{ParsedLogEvent: logs.ParsedLogEvent{
		OccurredAt:      occurred,
		PID:             7,
		DatabaseName:    "app_prod",
		ApplicationName: "web",
	}}
	withSample := logs.AnalyzedLogEvent{
		ParsedLogEvent:  logs.ParsedLogEvent{OccurredAt: occurred},
		StatementSample: &logs.StatementSample{Query: "SELECT 3", DurationMs: 1.5},
	}
	zeroTime := logs.AnalyzedLogEvent{ParsedLogEvent: logs.ParsedLogEvent{Message: "x"}}

	got := logEventsToProto([]logs.AnalyzedLogEvent{noSample, withSample, zeroTime})

	if got[0].GetPid() != 7 || got[0].GetDatabaseName() != "app_prod" || got[0].GetApplicationName() != "web" {
		t.Errorf("field mapping mismatch: pid=%d db=%q app=%q",
			got[0].GetPid(), got[0].GetDatabaseName(), got[0].GetApplicationName())
	}
	if got[0].GetStatementSample() != nil {
		t.Errorf("event without a sample should map to nil StatementSample")
	}
	if got[1].GetStatementSample() == nil {
		t.Fatalf("event with a sample should not map to nil StatementSample")
	}
	if !got[1].GetStatementSample().GetOccurredAt().AsTime().Equal(occurred) {
		t.Errorf("sample OccurredAt = %v, want the event's %v",
			got[1].GetStatementSample().GetOccurredAt().AsTime(), occurred)
	}
	if got[2].GetOccurredAt() != nil {
		t.Errorf("zero OccurredAt should map to nil timestamp, got %v", got[2].GetOccurredAt())
	}
}

func TestPrimitiveHelpers(t *testing.T) {
	t.Parallel()

	if deref((*string)(nil)) != "" {
		t.Errorf("deref(nil *string) should be empty")
	}
	if deref(new("v")) != "v" {
		t.Errorf("deref should return the pointed-to value")
	}
	if timestampOrNil(nil) != nil {
		t.Errorf("timestampOrNil(nil) should be nil")
	}
	if timestampFromTime(time.Time{}) != nil {
		t.Errorf("timestampFromTime(zero) should be nil")
	}
	if got := timestampFromTime(time.Unix(1, 0)); got == nil {
		t.Errorf("timestampFromTime(non-zero) should not be nil")
	}
}
