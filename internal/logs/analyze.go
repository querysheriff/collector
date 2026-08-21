package logs

import (
	"regexp"
	"strings"

	querysheriffv1 "buf.build/gen/go/querysheriff/backend/protocolbuffers/go/querysheriff/v1"
)

type classification = querysheriffv1.LogEvent_LogClassification

const classUnspecified = querysheriffv1.LogEvent_LOG_CLASSIFICATION_UNSPECIFIED

// e.g. "2026-06-14 12:00:00.123 UTC".
const postgresLogTimestamp = `(\d+-\d+-\d+ \d+:\d+:\d+(?:\.\d+)?(?:[\d:+-]+| \w+))`

// A cheap prefix check plus an optional capture regexp; a nil regexp classifies on the prefix alone.
type match struct {
	prefixes []string
	re       *regexp.Regexp
}

func (m match) matchesPrefix(content string) bool {
	for _, p := range m.prefixes {
		if strings.HasPrefix(content, p) {
			return true
		}
	}

	return false
}

type rule struct {
	class classification
	match match
	apply func(parts []string, e *AnalyzedLogEvent)
}

type Analyzer struct {
	rules []rule
}

func NewAnalyzer() *Analyzer {
	return &Analyzer{rules: allRules(newStatementSampleExtractor())}
}

func (a *Analyzer) Analyze(parsed ParsedLogEvent) AnalyzedLogEvent {
	event := AnalyzedLogEvent{ParsedLogEvent: parsed}
	a.classify(&event)

	return event
}

func (a *Analyzer) classify(event *AnalyzedLogEvent) {
	for i := range a.rules {
		r := &a.rules[i]
		if !r.match.matchesPrefix(event.Message) {
			continue
		}

		var parts []string
		if r.match.re != nil {
			if parts = r.match.re.FindStringSubmatch(event.Message); parts == nil {
				continue
			}
		}

		if r.class != classUnspecified {
			event.Classification = r.class
		}
		if r.apply != nil {
			r.apply(parts, event)
		}

		return
	}
}

func prefixRule(class classification, prefixes ...string) rule {
	return rule{class: class, match: match{prefixes: prefixes}}
}

func reRule(class classification, pattern string, prefixes ...string) rule {
	return rule{class: class, match: match{prefixes: prefixes, re: regexp.MustCompile(pattern)}}
}

func reRuleApply(
	class classification,
	pattern string,
	apply func(parts []string, e *AnalyzedLogEvent),
	prefixes ...string,
) rule {
	r := reRule(class, pattern, prefixes...)
	r.apply = apply

	return r
}

func allRules(samples *statementSampleExtractor) []rule {
	var rules []rule
	rules = append(rules, connectionRules()...)
	rules = append(rules, walRules()...)
	rules = append(rules, lockRules()...)
	rules = append(rules, checkpointRules()...)
	rules = append(rules, vacuumRules()...)
	rules = append(rules, serverFailureRules()...)
	rules = append(rules, serverLifecycleRules()...)
	rules = append(rules, serverMiscRules()...)
	rules = append(rules, standbyRules()...)
	rules = append(rules, constraintRules()...)
	rules = append(rules, schemaErrorRules()...)
	rules = append(rules, valueErrorRules()...)
	rules = append(rules, transactionErrorRules()...)
	rules = append(rules, statementRules(samples)...)

	return rules
}

func connectionRules() []rule {
	return []rule{
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_CONNECTION_REJECTED,
			`^(?:(?:pg_hba.conf rejects connection|no pg_hba.conf entry) for host "[^"]+", user "[^"]+", `+
				`database "[^"]+"(, SSL on|, SSL off)?|password authentication failed for user "[^"]+")`,
			"pg_hba.conf rejects connection ", "no pg_hba.conf entry for"),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_CONNECTION_REJECTED,
			`^(?:(?:Ident|password) authentication failed for user "([^"]+)"|`+
				`could not connect to Ident server at address "([^"]+)", port \d+: ([\w ]+))`,
			"password authentication failed for user", "Ident authentication failed for user",
			"could not connect to Ident server"),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_CONNECTION_REJECTED,
			`^database "([^"]+)" is not currently accepting connections`, "database"),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_CONNECTION_REJECTED,
			`^role "([^"]+)" is not permitted to log in`, "role"),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_CONNECTION_AUTHENTICATED,
			`^connection authenticated: (?:user|identity)="\w+" method=\w+ \(\w+:\d+\)`,
			"connection authenticated: "),
		prefixRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_CONNECTION_CLIENT_FAILED_TO_CONNECT,
			"incomplete startup packet", "invalid length of startup packet",
			"no PostgreSQL user name specified in startup packet",
			"invalid startup packet layout: expected terminator as last byte"),
		prefixRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_CONNECTION_LOST_OPEN_TX,
			"unexpected EOF on client connection with an open transaction"),
		prefixRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_CONNECTION_LOST,
			"unexpected EOF on client connection", "connection to client lost",
			"terminating connection because protocol synchronization was lost"),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_CONNECTION_LOST,
			`^could not (?:receive data from|send data to) client: [\w ]+`,
			"could not receive data from client", "could not send data to client"),
		prefixRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_CONNECTION_TERMINATED,
			"terminating connection due to administrator command"),
		prefixRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_OUT_OF_CONNECTIONS,
			"remaining connection slots are reserved for non-replication superuser connections",
			"sorry, too many clients already"),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_TOO_MANY_CONNECTIONS_ROLE,
			`^too many connections for role "([^"]+)"`, "too many connections for role"),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_TOO_MANY_CONNECTIONS_DATABASE,
			`^too many connections for database "([^"]+)"`, "too many connections for database"),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_COULD_NOT_ACCEPT_SSL_CONNECTION,
			`^could not accept SSL connection: [\w ]+`, "could not accept SSL connection: "),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_PROTOCOL_ERROR_UNSUPPORTED_VERSION,
			`^unsupported frontend protocol \d+\.\d+: server supports \d+\.\d+ to \d+\.\d+`,
			"unsupported frontend protocol"),
		prefixRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_PROTOCOL_ERROR_INCOMPLETE_MESSAGE,
			"incomplete message from client"),
		prefixRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_CONNECTION_RECEIVED, "connection received: "),
		prefixRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_CONNECTION_AUTHORIZED, "connection authorized: "),
		prefixRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_CONNECTION_DISCONNECTED, "disconnection: "),
	}
}

func walRules() []rule {
	return []rule{
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_RESTARTPOINT_AT,
			`^recovery restart point at (\w+)/(\w+)`, "recovery restart point at"),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_WAL_INVALID_RECORD_LENGTH,
			`^invalid record length at (\w+)/(\w+)(?:: wanted \d+, got \d+)?`, "invalid record length at "),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_WAL_REDO,
			`^redo (?:(?:starts|done) at (\w+)/(\w+)|is not required)`,
			"redo starts at", "redo done at", "redo is not required"),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_WAL_REDO,
			`^last completed transaction was at log time (\d+-\d+-\d+ \d+:\d+:\d+\.\d+[\d:+-]+)`,
			"last completed transaction was at log time "),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_WAL_ARCHIVE_COMMAND_FAILED,
			`^archiver process \(PID \d+\) exited with exit code \d+`, "archiver process"),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_WAL_ARCHIVE_COMMAND_FAILED,
			`^archive command (?:failed with exit code (\d+)|was terminated by signal (\d+)(: [\w ]+)?)`,
			"archive command"),
		prefixRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_WAL_BASE_BACKUP_COMPLETE,
			"pg_stop_backup complete, all required WAL segments have been archived"),
	}
}

func lockRules() []rule {
	return []rule{
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_LOCK_ACQUIRED,
			`^process \d+ acquired (\w+Lock) on (\w+)(?: [\(\)\d,]+)?( of \w+ \d+)* after ([\d\.]+) ms`,
			"process"),
		reRuleApply(classUnspecified,
			`^process \d+ (still waiting|avoided deadlock|detected deadlock while waiting) `+
				`for (\w+) on (\w+) (?:.+?) after ([\d\.]+) ms`,
			applyLockWait, "process"),
		prefixRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_LOCK_DEADLOCK_DETECTED, "deadlock detected"),
		prefixRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_LOCK_TIMEOUT,
			"canceling statement due to lock timeout"),
	}
}

func applyLockWait(parts []string, e *AnalyzedLogEvent) {
	switch parts[1] {
	case "still waiting":
		e.Classification = querysheriffv1.LogEvent_LOG_CLASSIFICATION_LOCK_WAITING
	case "avoided deadlock":
		e.Classification = querysheriffv1.LogEvent_LOG_CLASSIFICATION_LOCK_DEADLOCK_AVOIDED
	case "detected deadlock while waiting":
		e.Classification = querysheriffv1.LogEvent_LOG_CLASSIFICATION_LOCK_DEADLOCK_DETECTED
	}
}

func checkpointRules() []rule {
	return []rule{
		reRuleApply(classUnspecified, `^(checkpoint|restartpoint) starting: ([a-z- ]+)`,
			applyCheckpointStarting, "checkpoint", "restartpoint"),
		reRuleApply(classUnspecified,
			`^(checkpoint|restartpoint) complete: wrote (\d+) buffers \(([\d\.]+)%\)`+
				`(?:, wrote (\d+) SLRU buffers)?; `+
				`(\d+) WAL file\(s\) added, (\d+) removed, (\d+) recycled; `+
				`write=([\d\.]+) s, sync=([\d\.]+) s, total=([\d\.]+) s; `+
				`sync files=(\d+), longest=([\d\.]+) s, average=([\d\.]+) s`+
				`; distance=(\d+) kB, estimate=(\d+) kB`+
				`(?:; lsn=([A-F0-9]+/[A-F0-9]+), redo lsn=([A-F0-9]+/[A-F0-9]+))?`,
			applyCheckpointComplete, "checkpoint", "restartpoint"),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_CHECKPOINT_TOO_FREQUENT,
			`^checkpoints are occurring too frequently \((\d+) seconds? apart\)`, "checkpoints"),
	}
}

func applyCheckpointStarting(parts []string, e *AnalyzedLogEvent) {
	if parts[1] == "restartpoint" {
		e.Classification = querysheriffv1.LogEvent_LOG_CLASSIFICATION_RESTARTPOINT_STARTING
	} else {
		e.Classification = querysheriffv1.LogEvent_LOG_CLASSIFICATION_CHECKPOINT_STARTING
	}
}

func applyCheckpointComplete(parts []string, e *AnalyzedLogEvent) {
	if parts[1] == "restartpoint" {
		e.Classification = querysheriffv1.LogEvent_LOG_CLASSIFICATION_RESTARTPOINT_COMPLETE
	} else {
		e.Classification = querysheriffv1.LogEvent_LOG_CLASSIFICATION_CHECKPOINT_COMPLETE
	}
}

func vacuumRules() []rule {
	return []rule{
		prefixRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_AUTOVACUUM_CANCEL, "canceling autovacuum task"),
		prefixRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_SKIPPING_VACUUM_LOCK_NOT_AVAILABLE, "skipping vacuum of"),
		prefixRule(
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_SKIPPING_ANALYZE_LOCK_NOT_AVAILABLE,
			"skipping analyze of",
		),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_TXID_WRAPAROUND_WARNING,
			`^database (with OID (\d+)|"(.+?)") must be vacuumed within (\d+) transactions`, "database"),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_TXID_WRAPAROUND_ERROR,
			`^database is not accepting commands to avoid wraparound data loss in database (with OID (\d+)|"(.+?)")`,
			"database is not accepting commands to avoid wraparound data loss in database"),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_AUTOVACUUM_COMPLETED, autoVacuumPattern,
			"automatic vacuum of table", "automatic aggressive vacuum of table",
			"automatic aggressive vacuum to prevent wraparound of table"),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_AUTOANALYZE_COMPLETED, autoAnalyzePattern,
			"automatic analyze of table"),
		prefixRule(
			querysheriffv1.LogEvent_LOG_CLASSIFICATION_AUTOVACUUM_LAUNCHER_STARTED,
			"autovacuum launcher started",
		),
		prefixRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_AUTOVACUUM_LAUNCHER_SHUTTING_DOWN,
			"autovacuum launcher shutting down", "terminating autovacuum process due to administrator command"),
	}
}

func serverFailureRules() []rule {
	return []rule{
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_SERVER_CRASHED,
			`^server process \(PID (\d+)\) was terminated by signal (6|11)(: [\w ]+)?`, "server process"),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_SERVER_OUT_OF_MEMORY,
			`^server process \(PID (\d+)\) was terminated by signal (9)(: [\w ]+)?`,
			"out of memory", "server process"),
		prefixRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_SERVER_OUT_OF_MEMORY, "out of memory"),
		prefixRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_SERVER_CRASHED,
			"terminating any other active server processes",
			"terminating connection because of crash of another server process",
			"all server processes terminated; reinitializing"),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_SERVER_INVALID_CHECKSUM,
			`^page verification failed, calculated checksum (\d+) but expected (\d+)`, "page verification failed"),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_SERVER_INVALID_CHECKSUM,
			`^invalid page in block (\d+) of relation (\w+/\d+/\d+)`, "invalid page in block"),
		prefixRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_SERVER_STATS_COLLECTOR_TIMEOUT,
			"using stale statistics instead of current ones because stats collector is not responding",
			"pgstat wait timeout"),
	}
}

func serverLifecycleRules() []rule {
	return []rule{
		prefixRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_SERVER_RELOAD,
			"received SIGHUP, reloading configuration files"),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_SERVER_RELOAD,
			`^parameter "([^"]+)" (changed to "([^"]+)"|cannot be changed without restarting the server)`,
			"parameter"),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_SERVER_RELOAD,
			`^configuration file "([^"]+)" contains errors; unaffected changes were applied`,
			"configuration file"),
		prefixRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_SERVER_SHUTDOWN,
			"received fast shutdown request", "received smart shutdown request",
			"aborting any active transactions", "shutting down",
			"the database system is shutting down", "database system is shut down"),
		prefixRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_SERVER_START,
			"database system is ready to accept connections",
			"database system is ready to accept read only connections",
			"MultiXact member wraparound protections are now enabled", "entering standby mode",
			"redirecting log output to logging collector process", "ending log output to stderr"),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_SERVER_START,
			`^database system was shut down(?: in recovery)? at `+postgresLogTimestamp,
			"database system was shut down at ", "database system was shut down in recovery at "),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_SERVER_START_RECOVERING,
			`^(?:database system was not properly shut down; automatic recovery in progress|`+
				`(?:database system was interrupted; last known up at|`+
				`database system shutdown was interrupted; last known up at|`+
				`database system was interrupted while in recovery at(?: log time)?) `+postgresLogTimestamp+`)`,
			"database system was interrupted; last known up at ",
			"database system shutdown was interrupted; last known up at ",
			"database system was interrupted while in recovery at ",
			"database system was not properly shut down; automatic recovery in progress"),
	}
}

func serverMiscRules() []rule {
	return []rule{
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_SERVER_MISC,
			`^could not open usermap file "(.+)": (.+)`, "could not open usermap file"),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_SERVER_MISC,
			`^could not link file "(.+)" to "(.+)": (.+)`, "could not link file"),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_SERVER_MISC,
			`^unexpected pageaddr \w+/\w+ in log segment \w+, offset \d+`, "unexpected pageaddr"),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_SERVER_TEMP_FILE_CREATED,
			`^temporary file: path "(.+?)", size (\d+)`, "temporary file: path "),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_SERVER_PROCESS_EXITED,
			`^worker process: (.+?) \(PID (\d+)\) (?:exited with exit code (\d+)|was terminated by signal (\d+))`,
			"worker process: "),
	}
}

func standbyRules() []rule {
	return []rule{
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_STANDBY_RESTORED_WAL_FROM_ARCHIVE,
			`^restored log file "([^"]+)" from archive`, "restored log file"),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_STANDBY_STARTED_STREAMING,
			`^(?:started streaming WAL from primary|restarted WAL streaming) at (\w+)/(\w+) on timeline (\d+)`,
			"started streaming WAL", "restarted WAL streaming"),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_STANDBY_STREAMING_INTERRUPTED,
			`^could not receive data from WAL stream: ([\w: ]+)`, "could not receive data from WAL stream"),
		prefixRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_STANDBY_STOPPED_STREAMING,
			"terminating walreceiver process due to administrator command"),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_STANDBY_CONSISTENT_RECOVERY_STATE,
			`^consistent recovery state reached at (\w+)/(\w+)`, "consistent recovery state reached at"),
		prefixRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_STANDBY_STATEMENT_CANCELED,
			"canceling statement due to conflict with recovery"),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_STANDBY_INVALID_TIMELINE,
			`^according to history file, WAL location .+? belongs to timeline \d+, `+
				`but previous recovered WAL file came from timeline \d+`,
			"according to history file, WAL location"),
	}
}

func constraintRules() []rule {
	return []rule{
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_UNIQUE_CONSTRAINT_VIOLATION,
			`^duplicate key value violates unique constraint "(.+)"`,
			"duplicate key value violates unique constraint"),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_FOREIGN_KEY_CONSTRAINT_VIOLATION,
			`^insert or update on table "(.+?)" violates foreign key constraint "(.+?)"`,
			"insert or update on table"),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_FOREIGN_KEY_CONSTRAINT_VIOLATION,
			`^update or delete on table "(.+?)" violates foreign key constraint "(.+?)" on table "(.+?)"`,
			"update or delete on table"),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_NOT_NULL_CONSTRAINT_VIOLATION,
			`^null value in column "(.+?)" violates not-null constraint`, "null value in column"),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_CHECK_CONSTRAINT_VIOLATION,
			`^new row for relation "(.+?)" violates check constraint "(.+?)"`, "new row for relation"),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_CHECK_CONSTRAINT_VIOLATION,
			`^check constraint "(.+?)" is violated by some row`, "check constraint"),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_CHECK_CONSTRAINT_VIOLATION,
			`^column "(.+?)" of table "(.+?)" contains values that violate the new constraint`, "column"),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_CHECK_CONSTRAINT_VIOLATION,
			`^value for domain (.+?) violates check constraint "(.+?)"`, "value for domain"),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_EXCLUSION_CONSTRAINT_VIOLATION,
			`^conflicting key value violates exclusion constraint "(.+?)"`,
			"conflicting key value violates exclusion constraint"),
	}
}

func schemaErrorRules() []rule {
	return []rule{
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_SYNTAX_ERROR,
			`^syntax error at (?:end of input|or near "(.+)")(?: at character \d+)?`, "syntax error at"),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_COLUMN_MISSING_FROM_GROUP_BY,
			`^column "([^"]+)" must appear in the GROUP BY clause or be used in an aggregate function`+
				`(?: at character \d+)?`,
			"column"),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_COLUMN_DOES_NOT_EXIST,
			`^column "([^"]+)" of relation "([^"]+)" does not exist(?: at character \d+)?`, "column"),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_COLUMN_DOES_NOT_EXIST,
			`^column (?:"[^"]+"|[\w.]+) does not exist(?: at character \d+)?`, "column"),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_COLUMN_REFERENCE_AMBIGUOUS,
			`^column reference "([^"]+)" is ambiguous(?: at character \d+)?`, "column"),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_RELATION_DOES_NOT_EXIST,
			`^relation "([^"]+)" does not exist(?: at character \d+)?`, "relation"),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_FUNCTION_DOES_NOT_EXIST,
			`^function ([^"]+) does not exist(?: at character \d+)?`, "function"),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_OPERATOR_DOES_NOT_EXIST,
			`^operator does not exist: (\w+) ([`+regexp.QuoteMeta("+*/<>=~!@#%^&|`?-")+`]+) (\w+)`+
				`(?: at character \d+)?`,
			"operator does not exist: "),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_PERMISSION_DENIED,
			`^permission denied for (?:column|relation|table|sequence|database|function|operator|type|language|`+
				`large object|schema|operator class|operator family|collation|conversion|tablespace|`+
				`text search dictionary|text search configuration|foreign-data wrapper|foreign server|`+
				`event trigger|extension) ([\w_-]+)(?: at character \d+)?`,
			"permission denied"),
	}
}

func valueErrorRules() []rule {
	return []rule{
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_INVALID_INPUT_SYNTAX,
			`^invalid input syntax for [\w ]+(?:: "([^"]+)")?(?: at character \d+)?`, "invalid input syntax for"),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_VALUE_TOO_LONG_FOR_TYPE,
			`^value too long for type ([\w ()]+)`, "value too long for type"),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_INVALID_VALUE,
			`^invalid value "([^"]+)" for "([^"]+)"`, "invalid value"),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_MALFORMED_ARRAY_LITERAL,
			`^malformed array literal: "(.+)"(?: at character \d+)?`, "malformed array literal"),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_SUBQUERY_MISSING_ALIAS,
			`^subquery in FROM must have an alias(?: at character \d+)?`, "subquery in FROM must have an alias"),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_INSERT_TARGET_COLUMN_MISMATCH,
			`^INSERT has more expressions than target columns(?: at character \d+)?`,
			"INSERT has more expressions than target columns"),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_ANY_ALL_REQUIRES_ARRAY,
			`^op ANY/ALL \(array\) requires array on right side(?: at character \d+)?`,
			"op ANY/ALL (array) requires array on right side"),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_COLUMN_CANNOT_BE_CAST,
			`^column "([^"]+)" cannot be cast to type "([^"]+)"`, "column"),
		prefixRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_DIVISION_BY_ZERO, "division by zero"),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_CANNOT_DROP,
			`^cannot drop ([^"]+) because other objects depend on it`, "cannot drop"),
		prefixRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_INTEGER_OUT_OF_RANGE, "integer out of range"),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_PARAM_MISSING,
			`^there is no parameter \$\d+(?: at character \d+)?`, "there is no parameter $"),
		prefixRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_NO_SUCH_SAVEPOINT, "no such savepoint"),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_UNTERMINATED_QUOTED_STRING,
			`^unterminated quoted string(?: at or near "(.+?)")?(?: at character \d+)?`,
			"unterminated quoted string"),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_UNTERMINATED_QUOTED_IDENTIFIER,
			`^unterminated quoted identifier(?: at or near "(.+?)")?(?: at character \d+)?`,
			"unterminated quoted identifier"),
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_INVALID_BYTE_SEQUENCE,
			`^invalid byte sequence for encoding "([^"]+)": (.*)`, "invalid byte sequence for encoding"),
		prefixRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_INVALID_REGEXP, "invalid regular expression: "),
	}
}

func transactionErrorRules() []rule {
	return []rule{
		prefixRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_TRANSACTION_IS_ABORTED,
			"current transaction is aborted, commands ignored until end of transaction block"),
		prefixRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_ON_CONFLICT_NO_CONSTRAINT_MATCH,
			"there is no unique or exclusion constraint matching the ON CONFLICT specification"),
		prefixRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_ON_CONFLICT_ROW_AFFECTED_TWICE,
			"ON CONFLICT DO UPDATE command cannot affect row a second time"),
		prefixRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_COULD_NOT_SERIALIZE_REPEATABLE_READ,
			"could not serialize access due to concurrent update"),
		prefixRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_COULD_NOT_SERIALIZE_SERIALIZABLE,
			"could not serialize access due to read/write dependencies among transactions"),
		prefixRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_INCONSISTENT_RANGE_BOUNDS,
			"range lower bound must be less than or equal to range upper bound"),
	}
}

func statementRules(samples *statementSampleExtractor) []rule {
	durationRe := regexp.MustCompile(
		`^duration: ([\d\.]+) ms(?:  (?:statement|(parse|bind|execute|execute fetch from) ([^:]+)(?:/([^:]+))?):\s+)?`,
	)

	return []rule{
		reRuleApply(querysheriffv1.LogEvent_LOG_CLASSIFICATION_STATEMENT_AUTO_EXPLAIN,
			`^duration: ([\d\.]+) ms\s+ plan:\s+`,
			func(parts []string, e *AnalyzedLogEvent) {
				explainText := strings.TrimSpace(e.Message[len(parts[0]):])
				if sample, err := samples.fromAutoExplain(explainText, parts[1]); err == nil {
					e.StatementSample = &sample
				}
			},
			"duration: "),
		{
			class: querysheriffv1.LogEvent_LOG_CLASSIFICATION_STATEMENT_DURATION,
			match: match{prefixes: []string{"duration: "}},
			apply: func(_ []string, e *AnalyzedLogEvent) {
				parts := durationRe.FindStringSubmatch(e.Message)
				if parts == nil {
					return
				}
				query := e.Message[len(parts[0]):]
				if sample, ok := samples.fromLogMinDuration(query, parts[1], parts[2], e.Detail); ok {
					e.StatementSample = &sample
				}
			},
		},
		reRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_STATEMENT_LOG,
			`^(?:statement|(?:execute|execute fetch from) (?:[^:]+)(?:/(?:[^:]+))?): (.*)`,
			"statement: ", "execute "),
		prefixRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_STATEMENT_CANCELED_USER,
			"canceling statement due to user request"),
		prefixRule(querysheriffv1.LogEvent_LOG_CLASSIFICATION_STATEMENT_CANCELED_TIMEOUT,
			"canceling statement due to statement timeout"),
	}
}

const autoVacuumPattern = `^automatic (?P<aggressive>aggressive )?vacuum (?P<wraparound>to prevent wraparound )?` +
	`of table "(?P<relname>.+?)": index scans: (?P<idx_scans>\d+),?\s*` +
	`pages: (?P<pages_removed>\d+) removed, (?P<pages_remain>\d+) remain,\s*` +
	`(?:(?P<pages_scanned>\d+) scanned \((?P<pages_scanned_pct>[\d.]+)% of total\)` +
	`(?:, (?P<pages_eagerly_scanned>\d+) eagerly scanned)?)?\s*,?\s*` +
	`(?:(?P<pages_skipped_pins>\d+) skipped due to pins)?(?:, (?P<pages_skipped_frozen>\d+) skipped frozen)?\s*` +
	`tuples: (?P<tuples_removed>\d+) removed, (?P<tuples_remain>\d+) remain, ` +
	`(?P<tuples_new_dead>\d+) are dead but not yet removable(?:, oldest xmin: (?P<oldest_xmin>\d+))?,?\s*` +
	`(?:tuples missed: (?P<missed_dead_tuples>\d+) dead from (?P<missed_dead_pages>\d+) pages ` +
	`not removed due to cleanup lock contention)?,?\s*` +
	`(?:removable cutoff: (?P<cutoff>\d+), which was (?P<cutoff_age>\d+) XIDs old when operation ended)?,?\s*` +
	`(?:new relfrozenxid: (?P<new_frozenxid>\d+), which is (?P<new_frozenxid_diff>\d+) ` +
	`XIDs ahead of previous value)?,?\s*` +
	`(?:new relminmxid: (?P<new_minmxid>\d+), which is (?P<new_minmxid_diff>\d+) ` +
	`MXIDs ahead of previous value)?,?\s*` +
	`(?:frozen: (?P<frozen_pages>\d+) pages from table \((?P<frozen_pages_pct>[\d.]+)% of total\) ` +
	`had (?P<frozen_tuples>\d+) tuples frozen)?,?\s*` +
	`(?:visibility map: (?P<vm_all_visible>\d+) pages set all-visible, (?P<vm_all_frozen>\d+) ` +
	`pages set all-frozen \((?P<vm_all_visible_prev>\d+) were all-visible\))?\s*` +
	`(?:index scan (?P<idxscan_status>not needed|needed|bypassed|bypassed by failsafe): ` +
	`(?P<idxscan_pages>\d+) pages from table \((?P<idxscan_pages_pct>[\d.]+)% of total\) ` +
	`(?:have|had) (?P<idxscan_dead>\d+) dead item identifiers(?: removed)?)?,?\s*` +
	`(?P<idx_details>(?:index ".+?": pages: \d+ in total, \d+ newly deleted, ` +
	`\d+ currently deleted, \d+ reusable,?\s*)*)?` +
	`(?:I/O timings: read: (?P<io_read_ms>[\d.]+) ms, write: (?P<io_write_ms>[\d.]+) ms)?,?\s*` +
	`(?:avg read rate: (?P<io_read_rate>[\d.]+) MB/s, avg write rate: (?P<io_write_rate>[\d.]+) MB/s)?,?\s*` +
	`buffer usage: (?P<buffer_hits>\d+) hits, (?P<buffer_misses>\d+) (?:misses|reads), ` +
	`(?P<buffers_dirtied>\d+) dirtied,?\s*` +
	`(?:WAL usage: (?P<wal_records>\d+) records, (?P<wal_fpis>\d+) full page images, ` +
	`(?P<wal_bytes>\d+) bytes)?,?\s*` +
	`(?:(?P<wal_buffers_full>\d+) buffers full)?\s*` +
	`system usage: CPU(?:(?: (?P<cpu_s>[\d.]+)s/(?P<cpu_u>[\d.]+)u sec elapsed (?P<cpu_tot>[\d.]+) sec)|` +
	`(?:: user: (?P<cpu_user>[\d.]+) s, system: (?P<cpu_system>[\d.]+) s, elapsed: (?P<cpu_elapsed>[\d.]+) s))`

const autoAnalyzePattern = `^automatic analyze of table "(.+?)"\s*` +
	`(?:I/O timings: read: ([\d.]+) ms, write: ([\d.]+) ms)?\s*` +
	`(?:avg read rate: ([\d.]+) MB/s, avg write rate: ([\d.]+) MB/s)?\s*` +
	`(?:buffer usage: (\d+) hits, (\d+) (?:misses|reads), (\d+) dirtied)?\s*` +
	`system usage: CPU(?:(?: ([\d.]+)s/([\d.]+)u sec elapsed ([\d.]+) sec)|` +
	`(?:: user: ([\d.]+) s, system: ([\d.]+) s, elapsed: ([\d.]+) s))`
