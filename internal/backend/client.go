package backend

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"buf.build/gen/go/querysheriff/backend/connectrpc/go/querysheriff/v1/querysheriffv1connect"
	querysheriffv1 "buf.build/gen/go/querysheriff/backend/protocolbuffers/go/querysheriff/v1"
	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/querysheriff/collector/internal/config"
	"github.com/querysheriff/collector/internal/logs"
	"github.com/querysheriff/collector/internal/postgres"
)

const requestTimeout = 10 * time.Second

type Client struct {
	activity  querysheriffv1connect.ActivityServiceClient
	statement querysheriffv1connect.StatementServiceClient
	log       querysheriffv1connect.LogServiceClient
	health    querysheriffv1connect.HealthServiceClient
}

func NewClient(cfg config.Config) *Client {
	httpClient := &http.Client{Timeout: requestTimeout}
	auth := connect.WithInterceptors(authInterceptor(cfg.Backend.Token))

	return &Client{
		activity:  querysheriffv1connect.NewActivityServiceClient(httpClient, cfg.Backend.URL, auth),
		statement: querysheriffv1connect.NewStatementServiceClient(httpClient, cfg.Backend.URL, auth),
		log:       querysheriffv1connect.NewLogServiceClient(httpClient, cfg.Backend.URL, auth),
		health:    querysheriffv1connect.NewHealthServiceClient(httpClient, cfg.Backend.URL, auth),
	}
}

func authInterceptor(token string) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			req.Header().Set("Authorization", "Bearer "+token)
			return next(ctx, req)
		}
	}
}

func (c *Client) ReportActivity(
	ctx context.Context,
	collectedAt time.Time,
	snapshots []postgres.ActivitySnapshot,
) error {
	req := connect.NewRequest(&querysheriffv1.ReportActivityRequest{
		CollectedAt:       timestamppb.New(collectedAt),
		ActivitySnapshots: activitySnapshotsToProto(snapshots),
	})

	if _, err := c.activity.ReportActivity(ctx, req); err != nil {
		return fmt.Errorf("report activity: %w", err)
	}

	return nil
}

func activitySnapshotsToProto(snapshots []postgres.ActivitySnapshot) []*querysheriffv1.ActivitySnapshot {
	out := make([]*querysheriffv1.ActivitySnapshot, 0, len(snapshots))
	for _, s := range snapshots {
		out = append(out, &querysheriffv1.ActivitySnapshot{
			Pid:             s.PID,
			BackendStart:    timestamppb.New(s.BackendStart),
			DatabaseName:    deref(s.DatabaseName),
			UserName:        deref(s.UserName),
			ApplicationName: deref(s.ApplicationName),
			State:           deref(s.State),
			WaitEventType:   deref(s.WaitEventType),
			WaitEvent:       deref(s.WaitEvent),
			XactStart:       timestampOrNil(s.XactStart),
			QueryStart:      timestampOrNil(s.QueryStart),
			QueryId:         deref(s.QueryID),
			Query:           deref(s.Query),
			QueryTags:       s.QueryTags,
			BlockedByPid:    s.BlockedByPID,
			LockWaitStart:   timestampOrNil(s.LockWaitStart),
			LockMode:        deref(s.LockMode),
		})
	}

	return out
}

func (c *Client) ReportStatements(
	ctx context.Context,
	collectedAt time.Time,
	deltas []postgres.StatementDelta,
) ([]postgres.StatementIdentity, error) {
	req := connect.NewRequest(&querysheriffv1.ReportStatementsRequest{
		CollectedAt:     timestamppb.New(collectedAt),
		StatementDeltas: statementDeltasToProto(deltas),
	})

	resp, err := c.statement.ReportStatements(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("report statements: %w", err)
	}

	unknown := resp.Msg.GetUnknownStatements()
	refs := make([]postgres.StatementIdentity, len(unknown))
	for i, s := range unknown {
		refs[i] = postgres.StatementIdentity{
			UserName:     s.GetUserName(),
			DatabaseName: s.GetDatabaseName(),
			QueryID:      s.GetQueryId(),
		}
	}

	return refs, nil
}

func statementDeltasToProto(deltas []postgres.StatementDelta) []*querysheriffv1.StatementDelta {
	out := make([]*querysheriffv1.StatementDelta, 0, len(deltas))
	for _, s := range deltas {
		out = append(out, &querysheriffv1.StatementDelta{
			UserName:      s.UserName,
			DatabaseName:  s.DatabaseName,
			QueryId:       s.QueryID,
			Calls:         s.Calls,
			Rows:          s.Rows,
			TotalExecTime: s.TotalExecTime,
			TotalIoTime:   s.TotalIOTime,
		})
	}

	return out
}

func (c *Client) ReportStatementTexts(ctx context.Context, texts []postgres.StatementText) error {
	req := connect.NewRequest(&querysheriffv1.ReportStatementTextsRequest{
		StatementTexts: statementTextsToProto(texts),
	})

	if _, err := c.statement.ReportStatementTexts(ctx, req); err != nil {
		return fmt.Errorf("report statement texts: %w", err)
	}

	return nil
}

func statementTextsToProto(texts []postgres.StatementText) []*querysheriffv1.StatementText {
	out := make([]*querysheriffv1.StatementText, 0, len(texts))
	for _, t := range texts {
		out = append(out, &querysheriffv1.StatementText{
			Identity: &querysheriffv1.StatementIdentity{
				UserName:     t.UserName,
				DatabaseName: t.DatabaseName,
				QueryId:      t.QueryID,
			},
			Query: t.Query,
		})
	}

	return out
}

func (c *Client) ReportLogs(
	ctx context.Context,
	collectedAt time.Time,
	logEvents []logs.AnalyzedLogEvent,
) error {
	req := connect.NewRequest(&querysheriffv1.ReportLogsRequest{
		CollectedAt: timestamppb.New(collectedAt),
		LogEvents:   logEventsToProto(logEvents),
	})

	if _, err := c.log.ReportLogs(ctx, req); err != nil {
		return fmt.Errorf("report logs: %w", err)
	}

	return nil
}

func logEventsToProto(events []logs.AnalyzedLogEvent) []*querysheriffv1.LogEvent {
	out := make([]*querysheriffv1.LogEvent, 0, len(events))
	for i := range events {
		e := &events[i] // don't copy the large event
		out = append(out, &querysheriffv1.LogEvent{
			OccurredAt:      timestampFromTime(e.OccurredAt),
			LogLevel:        e.LogLevel,
			Classification:  e.Classification,
			Message:         e.Message,
			Pid:             e.PID,
			Username:        e.Username,
			DatabaseName:    e.DatabaseName,
			ApplicationName: e.ApplicationName,
			Detail:          e.Detail,
			Hint:            e.Hint,
			Context:         e.Context,
			Statement:       e.Statement,
			QueryId:         e.QueryID,
			BackendType:     e.BackendType,
			StateCode:       e.StateCode,
			StatementSample: statementSampleToProto(e),
		})
	}

	return out
}

func statementSampleToProto(e *logs.AnalyzedLogEvent) *querysheriffv1.LogStatementSample {
	if e.StatementSample == nil {
		return nil
	}

	return &querysheriffv1.LogStatementSample{
		OccurredAt:      timestampFromTime(e.OccurredAt),
		Query:           e.StatementSample.Query,
		DurationMs:      e.StatementSample.DurationMs,
		Parameters:      e.StatementSample.Parameters,
		ExplainPlanJson: e.StatementSample.ExplainPlanJSON,
		Tags:            e.StatementSample.Tags,
	}
}

func (c *Client) ReportHealth(
	ctx context.Context,
	collectedAt time.Time,
	databases []string,
) error {
	req := connect.NewRequest(&querysheriffv1.ReportHealthRequest{
		CollectedAt: timestamppb.New(collectedAt),
		Databases:   databases,
	})

	if _, err := c.health.ReportHealth(ctx, req); err != nil {
		return fmt.Errorf("report health: %w", err)
	}

	return nil
}

func timestampFromTime(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}

	return timestamppb.New(t)
}

func timestampOrNil(t *time.Time) *timestamppb.Timestamp {
	if t == nil {
		return nil
	}

	return timestamppb.New(*t)
}

func deref[T any](v *T) T {
	if v == nil {
		var zero T
		return zero
	}

	return *v
}
