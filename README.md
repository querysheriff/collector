<p align="center">
  <img src="https://raw.githubusercontent.com/querysheriff/frontend/master/static/logo.svg" alt="QuerySheriff logo" width="96">
</p>

<h1 align="center">QuerySheriff collector</h1>

The collector is a Go daemon that runs on a PostgreSQL server and sends its statistics and logs to the QuerySheriff [backend](https://github.com/querysheriff/backend) over Connect RPC. It never writes to the database, keeps no state on disk and opens no ports.

It collects:

- Activity, every second
  - Sessions with a transaction open for over a second or waiting on a lock
  - The sessions blocking them
- Query statistics, every minute
  - Per-query deltas from `pg_stat_statements`
- Logs, every 10 seconds
  - New jsonlog lines, classified by message
  - Slow query samples and `auto_explain` plans
- Health, every minute
  - A heartbeat with the server's database list

## Installation

The collector supports self-managed PostgreSQL 15 to 18 and comes as:

- A Debian package for Debian 11 to 13 (amd64), installed by [`scripts/install.sh`](scripts/install.sh)
- A Docker image, `ghcr.io/querysheriff/collector`, to run as a sidecar next to PostgreSQL in Kubernetes

The [deployment guide](https://github.com/querysheriff/docs/blob/master/deployment.md#6-install-the-collector) covers both.

## Configuration

Configuration lives in `/etc/querysheriff-collector.yml`. Set the backend URL, a collector token (created under Admin > Collectors in the dashboard, one per server) and the PostgreSQL connection URL. To collect logs, also set `logs.path` to the server's `log_directory`.

[`packaging/querysheriff-collector.yml.example`](packaging/querysheriff-collector.yml.example) describes every key.

## Monitoring user

Give the collector a role with `pg_monitor`. The server also needs `pg_stat_statements`, jsonlog logging and, for execution plans, `auto_explain`. See [step 1 of the deployment guide](https://github.com/querysheriff/docs/blob/master/deployment.md#1-prepare-the-monitored-postgresql-servers).

## Development

Requires Go 1.26.3 and Docker.

```sh
make dev-up   # PostgreSQL 17 in Docker
make dev      # run the collector
```
