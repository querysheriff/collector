package backend

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"buf.build/gen/go/pgdozor/backend/connectrpc/go/pgdozor/v1/pgdozorv1connect"
	pgdozorv1 "buf.build/gen/go/pgdozor/backend/protocolbuffers/go/pgdozor/v1"
	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/pgdozor/collector/internal/config"
	"github.com/pgdozor/collector/internal/logs"
	"github.com/pgdozor/collector/internal/postgres"
)

const requestTimeout = 10 * time.Second

type Client struct {
	activity  pgdozorv1connect.ActivityServiceClient
	statement pgdozorv1connect.StatementServiceClient
	log       pgdozorv1connect.LogServiceClient
	health    pgdozorv1connect.HealthServiceClient
}

func NewClient(cfg config.Config) *Client {
	httpClient := &http.Client{Timeout: requestTimeout}
	auth := connect.WithInterceptors(authInterceptor(cfg.Backend.Token))

	return &Client{
		activity:  pgdozorv1connect.NewActivityServiceClient(httpClient, cfg.Backend.URL, auth),
		statement: pgdozorv1connect.NewStatementServiceClient(httpClient, cfg.Backend.URL, auth),
		log:       pgdozorv1connect.NewLogServiceClient(httpClient, cfg.Backend.URL, auth),
		health:    pgdozorv1connect.NewHealthServiceClient(httpClient, cfg.Backend.URL, auth),
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
	req := connect.NewRequest(&pgdozorv1.ReportActivityRequest{
		CollectedAt:       timestamppb.New(collectedAt),
		ActivitySnapshots: activitySnapshotsToProto(snapshots),
	})

	if _, err := c.activity.ReportActivity(ctx, req); err != nil {
		return fmt.Errorf("report activity: %w", err)
	}

	return nil
}

func activitySnapshotsToProto(snapshots []postgres.ActivitySnapshot) []*pgdozorv1.ActivitySnapshot {
	out := make([]*pgdozorv1.ActivitySnapshot, 0, len(snapshots))
	for _, s := range snapshots {
		out = append(out, &pgdozorv1.ActivitySnapshot{
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
	req := connect.NewRequest(&pgdozorv1.ReportStatementsRequest{
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

func statementDeltasToProto(deltas []postgres.StatementDelta) []*pgdozorv1.StatementDelta {
	out := make([]*pgdozorv1.StatementDelta, 0, len(deltas))
	for _, s := range deltas {
		out = append(out, &pgdozorv1.StatementDelta{
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
	req := connect.NewRequest(&pgdozorv1.ReportStatementTextsRequest{
		StatementTexts: statementTextsToProto(texts),
	})

	if _, err := c.statement.ReportStatementTexts(ctx, req); err != nil {
		return fmt.Errorf("report statement texts: %w", err)
	}

	return nil
}

func statementTextsToProto(texts []postgres.StatementText) []*pgdozorv1.StatementText {
	out := make([]*pgdozorv1.StatementText, 0, len(texts))
	for _, t := range texts {
		out = append(out, &pgdozorv1.StatementText{
			Identity: &pgdozorv1.StatementIdentity{
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
	req := connect.NewRequest(&pgdozorv1.ReportLogsRequest{
		CollectedAt: timestamppb.New(collectedAt),
		LogEvents:   logEventsToProto(logEvents),
	})

	if _, err := c.log.ReportLogs(ctx, req); err != nil {
		return fmt.Errorf("report logs: %w", err)
	}

	return nil
}

func logEventsToProto(events []logs.AnalyzedLogEvent) []*pgdozorv1.LogEvent {
	out := make([]*pgdozorv1.LogEvent, 0, len(events))
	for i := range events {
		e := &events[i] // don't copy the large event
		out = append(out, &pgdozorv1.LogEvent{
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

func statementSampleToProto(e *logs.AnalyzedLogEvent) *pgdozorv1.LogStatementSample {
	if e.StatementSample == nil {
		return nil
	}

	return &pgdozorv1.LogStatementSample{
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
	req := connect.NewRequest(&pgdozorv1.ReportHealthRequest{
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
