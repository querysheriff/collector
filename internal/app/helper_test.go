//go:build integration

package app_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	querysheriffv1 "github.com/querysheriff/collector/gen/querysheriff/v1"
	"github.com/querysheriff/collector/gen/querysheriff/v1/querysheriffv1connect"
	"github.com/querysheriff/collector/internal/app"
	"github.com/querysheriff/collector/internal/config"
	"github.com/querysheriff/collector/internal/testdb"
)

const (
	collectorToken = "qsc_integration"
	waitTimeout    = 30 * time.Second
	pollInterval   = 100 * time.Millisecond
)

// forEachPostgres runs test on PostgreSQL 15, the oldest supported, and 17, the first with split pg_stat_statements IO columns.
func forEachPostgres(t *testing.T, test func(t *testing.T, image string)) {
	t.Helper()

	for _, image := range []string{"postgres:15", "postgres:17"} {
		t.Run(image, func(t *testing.T) {
			t.Parallel()
			test(t, image)
		})
	}
}

// fakeBackend records what the collector reports and, like the real backend, rejects a wrong token.
type fakeBackend struct {
	querysheriffv1connect.UnimplementedActivityServiceHandler
	querysheriffv1connect.UnimplementedStatementServiceHandler
	querysheriffv1connect.UnimplementedLogServiceHandler
	querysheriffv1connect.UnimplementedHealthServiceHandler

	mu              sync.Mutex
	activityReports int
	activity        []*querysheriffv1.ActivitySnapshot
	deltas          []*querysheriffv1.StatementDelta
	texts           []*querysheriffv1.StatementText
	knownTexts      map[string]bool
	logs            []*querysheriffv1.LogEvent
	health          [][]string
}

func newFakeBackend(t *testing.T) (*fakeBackend, string) {
	t.Helper()

	backend := &fakeBackend{knownTexts: map[string]bool{}}
	auth := connect.WithInterceptors(connect.UnaryInterceptorFunc(
		func(next connect.UnaryFunc) connect.UnaryFunc {
			return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
				if req.Header().Get("Authorization") != "Bearer "+collectorToken {
					return nil, connect.NewError(connect.CodeUnauthenticated, nil)
				}

				return next(ctx, req)
			}
		},
	))

	mux := http.NewServeMux()
	mux.Handle(querysheriffv1connect.NewActivityServiceHandler(backend, auth))
	mux.Handle(querysheriffv1connect.NewStatementServiceHandler(backend, auth))
	mux.Handle(querysheriffv1connect.NewLogServiceHandler(backend, auth))
	mux.Handle(querysheriffv1connect.NewHealthServiceHandler(backend, auth))

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return backend, server.URL
}

func (b *fakeBackend) ReportActivity(
	_ context.Context,
	req *connect.Request[querysheriffv1.ReportActivityRequest],
) (*connect.Response[querysheriffv1.ReportActivityResponse], error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.activityReports++
	b.activity = append(b.activity, req.Msg.GetActivitySnapshots()...)

	return connect.NewResponse(&querysheriffv1.ReportActivityResponse{}), nil
}

func (b *fakeBackend) ReportStatements(
	_ context.Context,
	req *connect.Request[querysheriffv1.ReportStatementsRequest],
) (*connect.Response[querysheriffv1.ReportStatementsResponse], error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	var unknown []*querysheriffv1.StatementIdentity
	for _, delta := range req.Msg.GetStatementDeltas() {
		b.deltas = append(b.deltas, delta)
		identity := &querysheriffv1.StatementIdentity{
			UserName: delta.GetUserName(), DatabaseName: delta.GetDatabaseName(), QueryId: delta.GetQueryId(),
		}
		if !b.knownTexts[identity.String()] {
			unknown = append(unknown, identity)
		}
	}

	return connect.NewResponse(&querysheriffv1.ReportStatementsResponse{UnknownStatements: unknown}), nil
}

func (b *fakeBackend) ReportStatementTexts(
	_ context.Context,
	req *connect.Request[querysheriffv1.ReportStatementTextsRequest],
) (*connect.Response[querysheriffv1.ReportStatementTextsResponse], error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, text := range req.Msg.GetStatementTexts() {
		b.texts = append(b.texts, text)
		b.knownTexts[text.GetIdentity().String()] = true
	}

	return connect.NewResponse(&querysheriffv1.ReportStatementTextsResponse{}), nil
}

func (b *fakeBackend) ReportLogs(
	_ context.Context,
	req *connect.Request[querysheriffv1.ReportLogsRequest],
) (*connect.Response[querysheriffv1.ReportLogsResponse], error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.logs = append(b.logs, req.Msg.GetLogEvents()...)

	return connect.NewResponse(&querysheriffv1.ReportLogsResponse{}), nil
}

func (b *fakeBackend) ReportHealth(
	_ context.Context,
	req *connect.Request[querysheriffv1.ReportHealthRequest],
) (*connect.Response[querysheriffv1.ReportHealthResponse], error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.health = append(b.health, req.Msg.GetDatabases())

	return connect.NewResponse(&querysheriffv1.ReportHealthResponse{}), nil
}

// inspect runs read with the recorded reports locked.
func (b *fakeBackend) inspect(read func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	read()
}

func (b *fakeBackend) hasActivity(match func(*querysheriffv1.ActivitySnapshot) bool) bool {
	var found bool
	b.inspect(func() { found = slices.ContainsFunc(b.activity, match) })

	return found
}

func (b *fakeBackend) hasLog(match func(*querysheriffv1.LogEvent) bool) bool {
	var found bool
	b.inspect(func() { found = slices.ContainsFunc(b.logs, match) })

	return found
}

func (b *fakeBackend) activityReportCount() int {
	var count int
	b.inspect(func() { count = b.activityReports })

	return count
}

type collector struct {
	*fakeBackend

	exited chan struct{}
	err    error
}

// runCollector runs the whole collector against server and a fake backend until the test ends. An empty logDir turns off log collection.
func runCollector(t *testing.T, server *testdb.Server, logDir string) *collector {
	t.Helper()

	backend, backendURL := newFakeBackend(t)
	cfg := config.Config{
		Backend:  config.BackendConfig{URL: backendURL, Token: collectorToken},
		Postgres: server.CollectorDSN,
		Logs:     config.LogsConfig{Path: logDir},
	}
	intervals := app.Intervals{
		Activity:   250 * time.Millisecond,
		Statements: 500 * time.Millisecond,
		Logs:       250 * time.Millisecond,
		Health:     500 * time.Millisecond,
	}
	logger := slog.New(slog.NewTextHandler(t.Output(), nil))

	ctx, cancel := context.WithCancel(context.Background())
	c := &collector{fakeBackend: backend, exited: make(chan struct{})}
	go func() {
		defer close(c.exited)
		c.err = app.Run(ctx, cfg, intervals, logger)
	}()

	t.Cleanup(func() {
		cancel()
		<-c.exited
		if c.err != nil {
			t.Errorf("collector failed: %v", c.err)
		}
	})

	return c
}

func waitFor(t *testing.T, what string, done func() bool) {
	t.Helper()

	deadline := time.Now().Add(waitTimeout)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out after %s waiting for %s", waitTimeout, what)
		}
		time.Sleep(pollInterval)
	}
}

func connectPostgres(t *testing.T, dsn string) *pgx.Conn {
	t.Helper()

	conn, err := pgx.Connect(t.Context(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })

	return conn
}

func mustExec(t *testing.T, conn *pgx.Conn, sql string) {
	t.Helper()

	if _, err := conn.Exec(t.Context(), sql); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}
