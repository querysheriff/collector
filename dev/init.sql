-- setup postgres
CREATE EXTENSION IF NOT EXISTS pg_stat_statements;

-- setup querysheriff_collector
CREATE USER querysheriff_collector WITH PASSWORD 'querysheriff_collector' CONNECTION LIMIT 5;
GRANT pg_monitor TO querysheriff_collector;

-- setup demo data
CREATE USER demo WITH PASSWORD 'demo';
CREATE DATABASE demo WITH OWNER demo;

-- setup querysheriff_backend
CREATE USER querysheriff_backend WITH PASSWORD 'querysheriff_backend';
CREATE DATABASE querysheriff WITH OWNER querysheriff_backend;
