# QuerySheriff collector

The collector is a Go agent that runs next to a PostgreSQL server. It samples `pg_stat_activity`, `pg_locks` and `pg_stat_statements`, tails the server's jsonlog, and sends the results to the [QuerySheriff backend](https://github.com/querysheriff/backend) over Connect RPC.

It supports self-managed PostgreSQL 15 to 18 and ships two ways: as a Debian package (Debian 11 to 13, amd64) that runs under systemd, and as a container image for running as a sidecar next to a containerized PostgreSQL. Installation is covered in the [deployment guide](https://github.com/querysheriff/docs#6-install-the-collector); this README is about working on the code.

## Requirements

- Go 1.26.3
- Docker with Compose, for the dev database and the integration tests

golangci-lint, buf and nfpm are pinned in the `Makefile` and run through `go run`. Nothing else needs installing.

## Local development

```sh
make dev-up     # PostgreSQL 17 on :5432 and Adminer on :8080
make dev        # run the collector with dev/querysheriff-collector.yml
make dev-down   # stop the containers and delete the data volume
```

The dev PostgreSQL is set up like a monitored server: `pg_stat_statements` and `auto_explain` are preloaded and jsonlog is written to `dev/logs/`, which the dev config tails. `dev/init.sql` creates the `querysheriff_collector` role and an empty `demo` database to run queries in. Activity is only reported for transactions open longer than a second, so `SELECT pg_sleep(2)` is a quick way to see one.

The dev config sends to a backend at `http://localhost:3000/api` with the placeholder token `qsc_replace-me`. Collection works without a backend; every rejected report is logged. To send real data, put a token created under Admin > Collectors in the dashboard into `dev/querysheriff-collector.yml`.

The backend's dev stack publishes the same host ports (5432 and 8080), so only one of the two stacks can run at a time.

## Configuration

The collector reads one YAML file, `/etc/querysheriff-collector.yml` unless `-config` says otherwise. Unknown keys are an error.

| Key | Required | Description |
| --- | --- | --- |
| `backend.url` | yes | Backend API URL including the `/api` prefix. `http` or `https`. |
| `backend.token` | yes | Collector token (`qsc_...`) created in the dashboard. The backend also uses it to tell which server the data comes from. |
| `postgres` | yes | Connection string for the monitored server, as a URL or a key/value DSN. |
| `logs.path` | no | Directory that holds the jsonlog files. Empty turns log collection off. |
| `logs.delete_rotated` | no | Delete rotated log files that match the server's `log_filename`. Default `false`. |
| `logs.delete_rotated_json_only` | no | With `delete_rotated`, delete only the `.json` files and leave the `.log` files to another tool. Default `false`. |

[`packaging/querysheriff-collector.yml.example`](packaging/querysheriff-collector.yml.example) is the commented config the Debian package installs.

| Flag | Description |
| --- | --- |
| `-config <path>` | Config file. Default `/etc/querysheriff-collector.yml`. |
| `-version` | Print the version and exit. |

Collection intervals are fixed in `internal/app/app.go`: activity every second, logs every 10 seconds, statements and the health heartbeat every minute.

## Project structure

```
cmd/collector/      entry point: flags, config, signal handling
internal/           application code, see the table below
gen/                Go code generated from the backend's proto schema
dev/                Compose file, init SQL and config for local runs
packaging/          nfpm config, systemd unit, maintainer scripts, example config
scripts/install.sh  installs the latest release .deb on a Debian host
Dockerfile          distroless image used for the sidecar
```

| Package | Responsibility |
| --- | --- |
| `internal/app` | Connects to PostgreSQL and rejects unsupported versions. Runs activity, statement and health collection on separate goroutines so a slow one never delays the others, and restarts the log pipeline when it fails. |
| `internal/postgres` | Queries against the stats views. Activity joins `pg_stat_activity` with ungranted locks and `pg_blocking_pids()`, and keeps client backends whose transaction has been open for over a second or that wait on a lock. Statements keeps the previous `pg_stat_statements` snapshot in memory and returns deltas; the first collection after a start or a stats reset only sets the baseline. |
| `internal/logs` | Follows the newest `.json` file in `logs.path`, parses jsonlog records, classifies messages, and extracts statement samples from `log_min_duration_statement` and `auto_explain` output. Files are read from the end, so a restart never resends old lines and lines written while the collector was down are skipped. QuerySheriff's own log records are collected too, but suspended for a while if they burst. The classification rules and sample parsing are adapted from pganalyze-collector. |
| `internal/backend` | Connect client for `ActivityService`, `StatementService`, `LogService` and `HealthService`, with the mapping from internal structs to protobuf. |
| `internal/schedule` | Ticks aligned to the wall clock. A task never overlaps itself, and ticks missed during a long run merge into the next one. |
| `internal/config` | YAML loading and validation. |
| `internal/testdb` | PostgreSQL 15 and 17 test containers with the jsonlog directory mounted on the host. |

## Commands

| Command | Description |
| --- | --- |
| `make dev-up` / `make dev-down` | Start or remove the dev PostgreSQL and Adminer. |
| `make dev` | Run against `dev/querysheriff-collector.yml`. |
| `go run ./cmd/collector -config <path>` | Run with another config. |
| `go build ./cmd/collector` | Build for the current platform. |
| `make build-linux` | Build a static linux/amd64 binary into `dist/`. |
| `make test-unit` | Unit tests. |
| `make test` | Unit and integration tests. The integration tests run the whole collector against PostgreSQL 15 and 17 in Docker and a fake backend. |
| `make fmt` | Format the code with golangci-lint. |
| `make lint` | golangci-lint, with and without the `integration` build tag. |
| `make tidy` | `go mod tidy`. |
| `make check` | `fmt`, `tidy`, `lint` and `test`. Run it before you push. |
| `make proto-update` | Regenerate `gen/` from a backend running on `localhost:3000`. |

To run a single test:

```sh
go test ./internal/logs/ -run TestTailDirectorySwitchesToWrittenFile -v -count=1
```

## Wire schema

`gen/` is generated from the proto files that the backend serves at `/schema.zip`; never edit it by hand. A schema change starts in [backend](https://github.com/querysheriff/backend#changing-the-wire-schema). Run that backend locally, then `make proto-update` here and commit `gen/`.

## Further reading

- [Deployment guide](https://github.com/querysheriff/docs): PostgreSQL settings, installation, sidecar setup, troubleshooting
