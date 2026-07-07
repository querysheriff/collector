module github.com/pgdozor/collector

go 1.26.3

require (
	buf.build/gen/go/pgdozor/backend/connectrpc/go v1.20.0-20260704215323-69b97c87060d.1
	buf.build/gen/go/pgdozor/backend/protocolbuffers/go v1.36.11-20260704215323-69b97c87060d.1
	connectrpc.com/connect v1.20.0
	github.com/fsnotify/fsnotify v1.10.1
	github.com/goccy/go-yaml v1.19.2
	github.com/jackc/pgx/v5 v5.10.0
	github.com/papertrail/go-tail v0.0.0-20221103124010-5087eb6a0a07
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	golang.org/x/sync v0.17.0 // indirect
	golang.org/x/sys v0.13.0 // indirect
	golang.org/x/text v0.29.0 // indirect
)
