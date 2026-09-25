# Collector

A lightweight Go agent for PostgreSQL performance monitoring. It runs on the PostgreSQL host, samples the statistics views, tails the server jsonlog, and ships everything to the backend over Connect RPC.

Supported: Debian 11–13, amd64, PostgreSQL 15–18. Also ships as a container image, for running as a sidecar next to a containerized PostgreSQL.
