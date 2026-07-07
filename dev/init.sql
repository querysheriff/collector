-- setup postgres
CREATE EXTENSION IF NOT EXISTS pg_stat_statements;

-- setup pgdozor_collector
CREATE USER pgdozor_collector WITH PASSWORD 'pgdozor_collector' CONNECTION LIMIT 5;
GRANT pg_monitor TO pgdozor_collector;

-- setup demo data
CREATE USER demo WITH PASSWORD 'demo';
CREATE DATABASE demo WITH OWNER demo;

-- setup pgdozor_backend
CREATE USER pgdozor_backend WITH PASSWORD 'pgdozor_backend';
CREATE DATABASE pgdozor WITH OWNER pgdozor_backend;
