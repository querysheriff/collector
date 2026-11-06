<p align="center">
  <img src="https://raw.githubusercontent.com/querysheriff/frontend/master/static/logo.svg" alt="QuerySheriff logo" width="96">
</p>

<h1 align="center">QuerySheriff collector</h1>

<p align="center">Go agent that runs on a PostgreSQL server and sends its stats and logs to the backend</p>

<p align="center">
  <a href="https://github.com/querysheriff/docs/blob/master/deployment.md#6-install-the-collector">Install</a> ·
  <a href="#configuration">Configuration</a> ·
  <a href="#development">Development</a> ·
  <a href="https://github.com/querysheriff/docs">Docs</a>
</p>

The collector is the part of QuerySheriff that touches your database. It reads PostgreSQL's statistics views and follows its jsonlog, then pushes everything to the [backend](https://github.com/querysheriff/backend) over Connect RPC. It needs only the `pg_monitor` role and read access to the log directory. In production it typically uses about 30 MiB of memory and 0.5% of one CPU core.

| Every | It sends |
| --- | --- |
| 1 second | Sessions from `pg_stat_activity` with a transaction open over a second or waiting on a lock, each with its most direct blocker |
| 10 seconds | New log lines, classified by message, with slow query samples and `auto_explain` plans pulled out of them |
| 1 minute | Per-query deltas from `pg_stat_statements` |
| 1 minute | A heartbeat with the server's database list |

It supports self-managed PostgreSQL 15 to 18. Nothing is kept on disk: after a restart the collector reads the log from its current end and starts a fresh statistics baseline.

## Install

On Debian 11 to 13 (amd64), this installs the latest release as a systemd service:

```sh
curl -fsSL https://raw.githubusercontent.com/querysheriff/collector/master/scripts/install.sh | sudo sh
```

For PostgreSQL in Kubernetes, run the `ghcr.io/querysheriff/collector` image as a sidecar instead. The [deployment guide](https://github.com/querysheriff/docs/blob/master/deployment.md#6-install-the-collector) covers both, plus the PostgreSQL settings the collector needs.

## Configuration

The collector reads one YAML file, `/etc/querysheriff-collector.yml` by default, and rejects unknown keys.

```yaml
backend:
  url: https://querysheriff.example.com/api
  token: qsc_CHANGE_ME   # created under Admin > Collectors in the dashboard

postgres: postgresql://querysheriff_collector:CHANGE_ME@localhost:5432/postgres

logs:
  path: /var/log/postgresql   # the server's log_directory; omit the block to skip logs
  delete_rotated: true        # delete rotated files that match log_filename
```

The token also tells the backend which server the data belongs to. `logs.delete_rotated_json_only: true` limits deletion to `.json` files. The commented reference is [`packaging/querysheriff-collector.yml.example`](packaging/querysheriff-collector.yml.example).

## Development

Requires Go 1.26.3 and Docker. Linters and generators are pinned in the `Makefile` and run through `go run`.

```sh
make dev-up   # PostgreSQL 17 with pg_stat_statements, auto_explain and jsonlog into dev/logs
make dev      # run the collector with dev/querysheriff-collector.yml
```

```console
time=… level=INFO msg="querysheriff collector started" postgres_version_num=170010
time=… level=INFO msg="log collection started" path=dev/logs
```

Reports fail and get logged until `dev/querysheriff-collector.yml` has a real token from a running backend. The backend's dev stack uses the same ports (5432 and 8080), so stop one before starting the other. `make dev-down` removes the dev database.

| Command | What it does |
| --- | --- |
| `make test-unit` | Unit tests |
| `make test` | Unit and integration tests, against PostgreSQL 15 and 17 in Docker |
| `make lint` / `make fmt` | golangci-lint check / format |
| `make check` | `fmt`, `tidy`, `lint` and `test` |
| `make proto-update` | Regenerate `gen/` from a backend running on `localhost:3000` |

`gen/` is generated from the backend's proto schema and never edited by hand. The log classification rules and sample parsing are adapted from [pganalyze-collector](https://github.com/pganalyze/collector).

## Learn more

The [deployment guide](https://github.com/querysheriff/docs/blob/master/deployment.md) covers PostgreSQL settings, installation, the sidecar setup and troubleshooting.
