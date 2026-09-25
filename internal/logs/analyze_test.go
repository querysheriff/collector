package logs_test

import (
	"testing"

	querysheriffv1 "github.com/querysheriff/collector/gen/querysheriff/v1"

	"github.com/querysheriff/collector/internal/logs"
)

func TestAnalyzeClassification(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		msg  string
		want querysheriffv1.LogEvent_LogClassification
	}{
		// Connections / authentication.
		{
			"hba_reject",
			`pg_hba.conf rejects connection for host "10.0.0.1", user "bob", database "app", SSL off`,
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_CONNECTION_REJECTED,
		},
		{
			"no_hba_entry",
			`no pg_hba.conf entry for host "10.0.0.1", user "bob", database "app"`,
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_CONNECTION_REJECTED,
		},
		{
			"password_failed",
			`password authentication failed for user "bob"`,
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_CONNECTION_REJECTED,
		},
		{
			"role_not_permitted",
			`role "bob" is not permitted to log in`,
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_CONNECTION_REJECTED,
		},
		{
			"db_not_accepting",
			`database "app" is not currently accepting connections`,
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_CONNECTION_REJECTED,
		},
		{
			"authenticated",
			`connection authenticated: identity="bob" method=md5 (pg_ident:5)`,
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_CONNECTION_AUTHENTICATED,
		},
		{
			"client_failed",
			"incomplete startup packet",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_CONNECTION_CLIENT_FAILED_TO_CONNECT,
		},
		{
			"lost_open_tx",
			"unexpected EOF on client connection with an open transaction",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_CONNECTION_LOST_OPEN_TX,
		},
		{
			"lost",
			"unexpected EOF on client connection",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_CONNECTION_LOST,
		},
		{
			"lost_socket",
			"could not receive data from client: Connection reset by peer",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_CONNECTION_LOST,
		},
		{
			"terminated",
			"terminating connection due to administrator command",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_CONNECTION_TERMINATED,
		},
		{
			"out_of_connections",
			"sorry, too many clients already",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_OUT_OF_CONNECTIONS,
		},
		{
			"too_many_role",
			`too many connections for role "bob"`,
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_TOO_MANY_CONNECTIONS_ROLE,
		},
		{
			"too_many_db",
			`too many connections for database "app"`,
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_TOO_MANY_CONNECTIONS_DATABASE,
		},
		{
			"ssl_accept",
			"could not accept SSL connection: EOF detected",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_COULD_NOT_ACCEPT_SSL_CONNECTION,
		},
		{
			"proto_version",
			"unsupported frontend protocol 1234.5679: server supports 3.0 to 3.0",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_PROTOCOL_ERROR_UNSUPPORTED_VERSION,
		},
		{
			"proto_incomplete",
			"incomplete message from client",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_PROTOCOL_ERROR_INCOMPLETE_MESSAGE,
		},
		{
			"received",
			"connection received: host=10.0.0.1 port=5432",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_CONNECTION_RECEIVED,
		},
		{
			"authorized",
			"connection authorized: user=bob database=app",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_CONNECTION_AUTHORIZED,
		},
		{
			"disconnection",
			"disconnection: session time: 0:00:05.123 user=bob database=app host=10.0.0.1 port=5432",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_CONNECTION_DISCONNECTED,
		},

		// WAL / archiving / replication.
		{
			"wal_invalid_len",
			"invalid record length at 0/16D62F8: wanted 24, got 0",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_WAL_INVALID_RECORD_LENGTH,
		},
		{
			"redo_starts",
			"redo starts at 0/16D62F8",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_WAL_REDO,
		},
		{
			"archiver_exited",
			"archiver process (PID 1234) exited with exit code 1",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_WAL_ARCHIVE_COMMAND_FAILED,
		},
		{
			"archive_cmd_failed",
			"archive command failed with exit code 1",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_WAL_ARCHIVE_COMMAND_FAILED,
		},
		{
			"base_backup",
			"pg_stop_backup complete, all required WAL segments have been archived",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_WAL_BASE_BACKUP_COMPLETE,
		},
		{
			"restartpoint_at",
			"recovery restart point at 0/16D62F8",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_RESTARTPOINT_AT,
		},
		{
			"standby_restored",
			`restored log file "000000010000000000000001" from archive`,
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_STANDBY_RESTORED_WAL_FROM_ARCHIVE,
		},
		{
			"standby_streaming",
			"started streaming WAL from primary at 0/3000000 on timeline 1",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_STANDBY_STARTED_STREAMING,
		},
		{
			"standby_interrupted",
			"could not receive data from WAL stream: connection closed",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_STANDBY_STREAMING_INTERRUPTED,
		},
		{
			"standby_stopped",
			"terminating walreceiver process due to administrator command",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_STANDBY_STOPPED_STREAMING,
		},
		{
			"standby_consistent",
			"consistent recovery state reached at 0/3000000",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_STANDBY_CONSISTENT_RECOVERY_STATE,
		},
		{
			"standby_canceled",
			"canceling statement due to conflict with recovery",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_STANDBY_STATEMENT_CANCELED,
		},

		// Locks.
		{
			"lock_acquired",
			"process 123 acquired ShareLock on transaction 456 after 1500.250 ms",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_LOCK_ACQUIRED,
		},
		{
			"lock_waiting",
			"process 123 still waiting for ShareLock on transaction 456 after 1000.5 ms",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_LOCK_WAITING,
		},
		{
			"deadlock",
			"deadlock detected",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_LOCK_DEADLOCK_DETECTED,
		},
		{
			"lock_timeout",
			"canceling statement due to lock timeout",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_LOCK_TIMEOUT,
		},

		// Checkpoints.
		{
			"checkpoint_starting",
			"checkpoint starting: shutdown immediate",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_CHECKPOINT_STARTING,
		},
		{
			"restartpoint_starting",
			"restartpoint starting: shutdown",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_RESTARTPOINT_STARTING,
		},
		{
			"checkpoint_complete",
			"checkpoint complete: wrote 100 buffers (0.6%); 0 WAL file(s) added, 0 removed, 1 recycled; " +
				"write=2.500 s, sync=0.100 s, total=2.700 s; sync files=10, longest=0.050 s, average=0.010 s; " +
				"distance=1024 kB, estimate=2048 kB",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_CHECKPOINT_COMPLETE,
		},
		{
			"checkpoints_frequent",
			"checkpoints are occurring too frequently (5 seconds apart)",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_CHECKPOINT_TOO_FREQUENT,
		},

		// Vacuum / autovacuum.
		{
			"autovacuum_cancel",
			"canceling autovacuum task",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_AUTOVACUUM_CANCEL,
		},
		{
			"skipping_vacuum",
			`skipping vacuum of "orders" --- lock not available`,
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_SKIPPING_VACUUM_LOCK_NOT_AVAILABLE,
		},
		{
			"skipping_analyze",
			`skipping analyze of "orders" --- lock not available`,
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_SKIPPING_ANALYZE_LOCK_NOT_AVAILABLE,
		},
		{
			"wraparound_warning",
			`database "app" must be vacuumed within 1000000 transactions`,
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_TXID_WRAPAROUND_WARNING,
		},
		{
			"wraparound_error",
			`database is not accepting commands to avoid wraparound data loss in database "app"`,
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_TXID_WRAPAROUND_ERROR,
		},
		{
			"av_launcher_started",
			"autovacuum launcher started",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_AUTOVACUUM_LAUNCHER_STARTED,
		},
		{
			"av_launcher_shutdown",
			"autovacuum launcher shutting down",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_AUTOVACUUM_LAUNCHER_SHUTTING_DOWN,
		},

		// Server lifecycle / misc.
		{
			"server_crashed",
			"server process (PID 1234) was terminated by signal 11: Segmentation fault",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_SERVER_CRASHED,
		},
		{
			"oom_crash",
			"server process (PID 1234) was terminated by signal 9: Killed",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_SERVER_OUT_OF_MEMORY,
		},
		{
			"oom",
			"out of memory",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_SERVER_OUT_OF_MEMORY,
		},
		{
			"crashed_others",
			"terminating any other active server processes",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_SERVER_CRASHED,
		},
		{
			"reload",
			"received SIGHUP, reloading configuration files",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_SERVER_RELOAD,
		},
		{
			"shutdown",
			"received fast shutdown request",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_SERVER_SHUTDOWN,
		},
		{
			"server_start",
			"database system is ready to accept connections",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_SERVER_START,
		},
		{
			"shut_down_at",
			"database system was shut down at 2026-06-14 12:00:00 UTC",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_SERVER_START,
		},
		{
			"recovering",
			"database system was not properly shut down; automatic recovery in progress",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_SERVER_START_RECOVERING,
		},
		{
			"page_verification",
			"page verification failed, calculated checksum 1 but expected 2",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_SERVER_INVALID_CHECKSUM,
		},
		{
			"invalid_page",
			"invalid page in block 5 of relation base/16384/1259",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_SERVER_INVALID_CHECKSUM,
		},
		{
			"temp_file",
			`temporary file: path "base/pgsql_tmp/foo", size 1024`,
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_SERVER_TEMP_FILE_CREATED,
		},
		{
			"usermap",
			`could not open usermap file "/etc/x": No such file`,
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_SERVER_MISC,
		},
		{
			"param_changed",
			`parameter "work_mem" changed to "4MB"`,
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_SERVER_RELOAD,
		},
		{
			"param_restart",
			`parameter "shared_buffers" cannot be changed without restarting the server`,
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_SERVER_RELOAD,
		},
		{
			"config_errors",
			`configuration file "/etc/x.conf" contains errors; unaffected changes were applied`,
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_SERVER_RELOAD,
		},
		{
			"worker_exited",
			"worker process: logical replication launcher (PID 123) exited with exit code 1",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_SERVER_PROCESS_EXITED,
		},
		{
			"stats_timeout",
			"pgstat wait timeout",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_SERVER_STATS_COLLECTOR_TIMEOUT,
		},

		// Constraint violations.
		{
			"unique",
			`duplicate key value violates unique constraint "users_pkey"`,
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_UNIQUE_CONSTRAINT_VIOLATION,
		},
		{
			"fk_insert",
			`insert or update on table "orders" violates foreign key constraint "fk_user"`,
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_FOREIGN_KEY_CONSTRAINT_VIOLATION,
		},
		{
			"fk_delete",
			`update or delete on table "users" violates foreign key constraint "fk_user" on table "orders"`,
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_FOREIGN_KEY_CONSTRAINT_VIOLATION,
		},
		{
			"not_null",
			`null value in column "name" violates not-null constraint`,
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_NOT_NULL_CONSTRAINT_VIOLATION,
		},
		{
			"check_new_row",
			`new row for relation "users" violates check constraint "age_check"`,
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_CHECK_CONSTRAINT_VIOLATION,
		},
		{
			"check_some_row",
			`check constraint "age_check" is violated by some row`,
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_CHECK_CONSTRAINT_VIOLATION,
		},
		{
			"exclusion",
			`conflicting key value violates exclusion constraint "no_overlap"`,
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_EXCLUSION_CONSTRAINT_VIOLATION,
		},

		// Query errors.
		{
			"syntax",
			`syntax error at or near "SELCT"`,
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_SYNTAX_ERROR,
		},
		{
			"group_by",
			`column "foo" must appear in the GROUP BY clause or be used in an aggregate function`,
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_COLUMN_MISSING_FROM_GROUP_BY,
		},
		{
			"column_missing",
			`column "foo" does not exist`,
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_COLUMN_DOES_NOT_EXIST,
		},
		{
			"column_missing_on_table",
			`column "foo" of relation "bar" does not exist`,
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_COLUMN_DOES_NOT_EXIST,
		},
		{
			"column_ambiguous",
			`column reference "id" is ambiguous`,
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_COLUMN_REFERENCE_AMBIGUOUS,
		},
		{
			"relation_missing",
			`relation "foo" does not exist`,
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_RELATION_DOES_NOT_EXIST,
		},
		{
			"function_missing",
			"function foo(integer) does not exist",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_FUNCTION_DOES_NOT_EXIST,
		},
		{
			"input_syntax",
			`invalid input syntax for type integer: "abc"`,
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_INVALID_INPUT_SYNTAX,
		},
		{
			"value_too_long",
			"value too long for type character varying(10)",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_VALUE_TOO_LONG_FOR_TYPE,
		},
		{
			"invalid_value",
			`invalid value "x" for "YYYY"`,
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_INVALID_VALUE,
		},
		{
			"array_literal",
			`malformed array literal: "{1,2"`,
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_MALFORMED_ARRAY_LITERAL,
		},
		{
			"subquery_alias",
			"subquery in FROM must have an alias",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_SUBQUERY_MISSING_ALIAS,
		},
		{
			"insert_mismatch",
			"INSERT has more expressions than target columns",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_INSERT_TARGET_COLUMN_MISMATCH,
		},
		{
			"any_all",
			"op ANY/ALL (array) requires array on right side",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_ANY_ALL_REQUIRES_ARRAY,
		},
		{
			"operator_missing",
			"operator does not exist: integer = text",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_OPERATOR_DOES_NOT_EXIST,
		},
		{
			"permission_denied",
			"permission denied for table users",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_PERMISSION_DENIED,
		},
		{
			"tx_aborted",
			"current transaction is aborted, commands ignored until end of transaction block",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_TRANSACTION_IS_ABORTED,
		},
		{
			"on_conflict_no_match",
			"there is no unique or exclusion constraint matching the ON CONFLICT specification",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_ON_CONFLICT_NO_CONSTRAINT_MATCH,
		},
		{
			"on_conflict_twice",
			"ON CONFLICT DO UPDATE command cannot affect row a second time",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_ON_CONFLICT_ROW_AFFECTED_TWICE,
		},
		{
			"cannot_cast",
			`column "id" cannot be cast to type "integer"`,
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_COLUMN_CANNOT_BE_CAST,
		},
		{
			"division_by_zero",
			"division by zero",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_DIVISION_BY_ZERO,
		},
		{
			"cannot_drop",
			"cannot drop table foo because other objects depend on it",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_CANNOT_DROP,
		},
		{
			"integer_range",
			"integer out of range",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_INTEGER_OUT_OF_RANGE,
		},
		{
			"invalid_regexp",
			"invalid regular expression: brackets [] not balanced",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_INVALID_REGEXP,
		},
		{
			"param_missing",
			"there is no parameter $2",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_PARAM_MISSING,
		},
		{
			"no_savepoint",
			"no such savepoint",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_NO_SUCH_SAVEPOINT,
		},
		{
			"unterminated_string",
			`unterminated quoted string at or near "'abc"`,
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_UNTERMINATED_QUOTED_STRING,
		},
		{
			"unterminated_ident",
			`unterminated quoted identifier at or near """abc"`,
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_UNTERMINATED_QUOTED_IDENTIFIER,
		},
		{
			"byte_sequence",
			`invalid byte sequence for encoding "UTF8": 0x00`,
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_INVALID_BYTE_SEQUENCE,
		},
		{
			"serialize_repeatable",
			"could not serialize access due to concurrent update",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_COULD_NOT_SERIALIZE_REPEATABLE_READ,
		},
		{
			"serialize_serializable",
			"could not serialize access due to read/write dependencies among transactions",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_COULD_NOT_SERIALIZE_SERIALIZABLE,
		},
		{
			"range_bounds",
			"range lower bound must be less than or equal to range upper bound",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_INCONSISTENT_RANGE_BOUNDS,
		},

		// Statements.
		{
			"statement_log",
			"statement: SELECT 1",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_STATEMENT_LOG,
		},

		// Unmatched.
		{
			"unmatched",
			"this message matches no classifier",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_UNSPECIFIED,
		},
		{
			// Shares the "process" prefix with the lock rules but matches neither regexp.
			"prefix_hit_regexp_miss",
			"process 42 is doing something unrecognized",
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_UNSPECIFIED,
		},
	}

	a := logs.NewAnalyzer()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			if got := a.Analyze(logs.ParsedLogEvent{Message: c.msg}).Classification; got != c.want {
				t.Errorf("classification of %q = %v, want %v", c.msg, got, c.want)
			}
		})
	}
}
