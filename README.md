# Collector

A lightweight Go agent for PostgreSQL performance monitoring. It runs on the PostgreSQL host, samples the statistics views, tails the server jsonlog, and ships everything to the querysheriff backend over Connect RPC.

Supported: Debian 11–13, amd64, PostgreSQL 15–18. Also ships as a container image, for running as a sidecar next to a containerized PostgreSQL.

> Production install and operation live in the [`querysheriff/docs`](https://github.com/querysheriff/docs) repo (the "Collector" section). This README is for working on the collector itself.

## Local development

```sh
make dev-postgres-up   # containerized Postgres seeded from dev/init.sql
make dev               # run the collector against dev/querysheriff-collector.yml
make dev-postgres-down # tear it down
```

## Check

```sh
make check # fmt + lint + test
```

## Build & release

```sh
make deb                      # build the amd64 .deb into dist/ (via nfpm)
make docker                   # build the container image (for the Kubernetes sidecar deployment)
make release VERSION=0.1.0    # tag v0.1.0 + push; CI publishes the release and the image
```
