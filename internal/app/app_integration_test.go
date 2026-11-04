//go:build integration

package app_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	querysheriffv1 "github.com/querysheriff/collector/gen/querysheriff/v1"
	"github.com/querysheriff/collector/internal/testdb"
)

const (
	activityProbeUpdate = "UPDATE activity_probe SET n = n + 1 WHERE id = 1"
	statementsProbe     = "SELECT count(*) FROM statements_probe"
	duplicateDatabase   = "42P04"
	outage              = 3 * time.Second
	inFlightTick        = 500 * time.Millisecond
)

func TestCollectorReportsBlockedSessionAndTaggedTransaction(t *testing.T) {
	t.Parallel()

	forEachPostgres(t, func(t *testing.T, image string) {
		server := testdb.Postgres(t, image)
		backend := runCollector(t, server, "")

		admin := connectPostgres(t, server.AdminDSN)
		mustExec(t, admin, "CREATE TABLE IF NOT EXISTS activity_probe (id int PRIMARY KEY, n int)")
		mustExec(t, admin, "INSERT INTO activity_probe VALUES (1, 0) ON CONFLICT DO NOTHING")

		holder := connectPostgres(t, server.AdminDSN)
		holderPID := int32(holder.PgConn().PID())
		mustExec(t, holder, "BEGIN")
		mustExec(t, holder, "/* service=checkout */ "+activityProbeUpdate)

		waiter := connectPostgres(t, server.AdminDSN)
		waiterPID := int32(waiter.PgConn().PID())
		waited := make(chan error, 1)
		go func() {
			_, err := waiter.Exec(t.Context(), activityProbeUpdate)
			waited <- err
		}()

		waitFor(t, "the waiter reported as blocked by the holder", func() bool {
			return backend.hasActivity(func(s *querysheriffv1.ActivitySnapshot) bool {
				return s.GetPid() == waiterPID && s.GetBlockedByPid() == holderPID &&
					s.GetWaitEventType() == "Lock" && s.GetLockMode() == "ShareLock" && s.GetLockWaitStart() != nil
			})
		})
		waitFor(t, "the holder reported idle in transaction with its tags stripped", func() bool {
			return backend.hasActivity(func(s *querysheriffv1.ActivitySnapshot) bool {
				return s.GetPid() == holderPID && s.GetState() == "idle in transaction" &&
					s.GetQuery() == activityProbeUpdate && s.GetQueryTags()["service"] == "checkout"
			})
		})

		mustExec(t, holder, "ROLLBACK")
		if err := <-waited; err != nil {
			t.Fatalf("waiter update: %v", err)
		}
	})
}

func TestCollectorReportsStatementDeltasAndTexts(t *testing.T) {
	t.Parallel()

	forEachPostgres(t, func(t *testing.T, image string) {
		server := testdb.Postgres(t, image)
		backend := runCollector(t, server, "")

		admin := connectPostgres(t, server.AdminDSN)
		mustExec(t, admin, "CREATE TABLE IF NOT EXISTS statements_probe (id int)")

		waitFor(t, "the probe's text and a delta of its calls", func() bool {
			mustExec(t, admin, statementsProbe)

			var found bool
			backend.inspect(func() {
				i := slices.IndexFunc(backend.texts, func(text *querysheriffv1.StatementText) bool {
					return text.GetQuery() == statementsProbe
				})
				if i < 0 {
					return
				}
				identity := backend.texts[i].GetIdentity()
				found = slices.ContainsFunc(backend.deltas, func(d *querysheriffv1.StatementDelta) bool {
					return d.GetQueryId() == identity.GetQueryId() && d.GetUserName() == "postgres" &&
						d.GetDatabaseName() == "postgres" && d.GetCalls() > 0 && d.GetTotalExecTime() > 0
				})
			})

			return found
		})
	})
}

func TestCollectorReportsDatabases(t *testing.T) {
	t.Parallel()

	forEachPostgres(t, func(t *testing.T, image string) {
		server := testdb.Postgres(t, image)
		admin := connectPostgres(t, server.AdminDSN)

		var pgErr *pgconn.PgError
		if _, err := admin.Exec(t.Context(), "CREATE DATABASE health_probe"); err != nil &&
			(!errors.As(err, &pgErr) || pgErr.Code != duplicateDatabase) {
			t.Fatalf("create database: %v", err)
		}

		backend := runCollector(t, server, "")

		var databases []string
		waitFor(t, "a health report", func() bool {
			backend.inspect(func() {
				if len(backend.health) > 0 {
					databases = backend.health[0]
				}
			})

			return databases != nil
		})

		if !slices.Contains(databases, "health_probe") || !slices.Contains(databases, "postgres") ||
			slices.Contains(databases, "template0") || slices.Contains(databases, "template1") {
			t.Fatalf("databases = %v, want health_probe and postgres without templates", databases)
		}
	})
}

func TestCollectorReportsLogErrorsAndSlowStatements(t *testing.T) {
	t.Parallel()

	forEachPostgres(t, func(t *testing.T, image string) {
		server := testdb.Postgres(t, image)
		backend := runCollector(t, server, server.LogDir)

		session := connectPostgres(t, server.AdminDSN+"&application_name=logs_probe")
		mustExec(t, session, "SET log_min_duration_statement = 0")

		// The tailer follows only lines written after it opens the file, so keep writing until both arrive.
		waitFor(t, "the division error and the pg_sleep sample", func() bool {
			_, _ = session.Exec(t.Context(), "SELECT 1/0")
			mustExec(t, session, "SELECT pg_sleep(0.01)")

			return backend.hasLog(func(e *querysheriffv1.LogEvent) bool {
				return e.GetApplicationName() == "logs_probe" &&
					e.GetLogLevel() == querysheriffv1.LogEvent_LOG_LEVEL_ERROR &&
					e.GetMessage() == "division by zero" && e.GetStateCode() == "22012" &&
					e.GetStatement() == "SELECT 1/0"
			}) && backend.hasLog(func(e *querysheriffv1.LogEvent) bool {
				return e.GetApplicationName() == "logs_probe" &&
					strings.Contains(e.GetStatementSample().GetQuery(), "pg_sleep") &&
					e.GetStatementSample().GetDurationMs() >= 10
			})
		})
	})
}

func TestCollectorSurvivesPostgresOutage(t *testing.T) {
	t.Parallel()

	forEachPostgres(t, func(t *testing.T, image string) {
		server := testdb.NewPostgres(t, image)
		c := runCollector(t, server, "")

		waitFor(t, "activity reports before the outage", func() bool { return c.activityReportCount() > 0 })

		if err := server.Container.Stop(t.Context(), nil); err != nil {
			t.Fatalf("stop postgres: %v", err)
		}
		time.Sleep(inFlightTick)
		reportsAtOutage := c.activityReportCount()

		select {
		case <-c.exited:
			t.Fatalf("collector exited while postgres was down: %v", c.err)
		case <-time.After(outage):
		}

		if reports := c.activityReportCount(); reports != reportsAtOutage {
			t.Fatalf("got %d activity reports while postgres was down, want 0", reports-reportsAtOutage)
		}

		if err := server.Container.Start(t.Context()); err != nil {
			t.Fatalf("start postgres: %v", err)
		}

		waitFor(t, "activity reports after postgres is back", func() bool {
			return c.activityReportCount() > reportsAtOutage
		})
	})
}
