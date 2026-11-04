package testdb

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib" // database/sql driver for the readiness check
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/testcontainers/testcontainers-go"
	pgmodule "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	superuser       = "postgres"
	collectorRole   = "querysheriff_collector"
	database        = "postgres"
	containerPort   = "5432/tcp"
	containerLogDir = "/logs"
	logDirMode      = 0o777
)

// Mirrors dev/init.sql.
const initSQL = `
CREATE EXTENSION pg_stat_statements;
CREATE USER querysheriff_collector WITH PASSWORD 'querysheriff_collector' CONNECTION LIMIT 5;
GRANT pg_monitor TO querysheriff_collector;
`

type Server struct {
	Container    *pgmodule.PostgresContainer
	AdminDSN     string
	CollectorDSN string
	// LogDir is the host directory the server writes its jsonlogs to.
	LogDir string
}

//nolint:gochecknoglobals // One container per image, shared by the package's tests.
var shared sync.Map

// Postgres returns the server shared by the package's tests, so each test uses its own table and database names.
func Postgres(t *testing.T, image string) *Server {
	t.Helper()

	once, _ := shared.LoadOrStore(image, sync.OnceValues(func() (*Server, error) { return start(image) }))
	startOnce, _ := once.(func() (*Server, error))

	server, err := startOnce()
	if err != nil {
		t.Fatalf("start %s: %v", image, err)
	}

	return server
}

// NewPostgres starts a server owned by t, for tests that stop or restart it.
func NewPostgres(t *testing.T, image string) *Server {
	t.Helper()

	server, err := start(image)
	if server != nil {
		t.Cleanup(func() { _ = os.RemoveAll(server.LogDir) })
		testcontainers.CleanupContainer(t, server.Container)
	}
	if err != nil {
		t.Fatalf("start %s: %v", image, err)
	}

	return server
}

func start(image string) (*Server, error) {
	ctx := context.Background()

	logDir, err := os.MkdirTemp("", "querysheriff-pglogs-")
	if err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}
	// Postgres runs as another uid inside the container.
	if err = os.Chmod(logDir, logDirMode); err != nil {
		return nil, fmt.Errorf("chmod log dir: %w", err)
	}

	// A fixed host port survives a container restart, so a running collector can reconnect.
	hostPort, err := freePort(ctx)
	if err != nil {
		return nil, err
	}

	ctr, err := pgmodule.Run(ctx, image,
		pgmodule.WithUsername(superuser), pgmodule.WithPassword(superuser), pgmodule.WithDatabase(database),
		testcontainers.WithCmd("postgres",
			"-c", "fsync=off",
			"-c", "shared_preload_libraries=pg_stat_statements",
			"-c", "logging_collector=on",
			"-c", "log_destination=jsonlog",
			"-c", "log_directory="+containerLogDir,
			"-c", "log_file_mode=0644",
		),
		testcontainers.WithHostConfigModifier(func(hc *container.HostConfig) {
			hc.Binds = []string{logDir + ":" + containerLogDir}
			hc.PortBindings = network.PortMap{
				network.MustParsePort(containerPort): {{HostPort: strconv.Itoa(hostPort)}},
			}
		}),
		// logging_collector moves the "ready to accept connections" lines the module waits for into the log files.
		testcontainers.WithWaitStrategy(wait.ForSQL(containerPort, "pgx", func(host string, port network.Port) string {
			return dsn(superuser, net.JoinHostPort(host, port.Port()))
		})),
	)
	server := &Server{Container: ctr, LogDir: logDir}
	if err != nil {
		return server, fmt.Errorf("run container: %w", err)
	}

	endpoint, err := ctr.PortEndpoint(ctx, containerPort, "")
	if err != nil {
		return server, fmt.Errorf("port endpoint: %w", err)
	}

	server.AdminDSN = dsn(superuser, endpoint)
	server.CollectorDSN = dsn(collectorRole, endpoint)

	return server, initialize(ctx, server.AdminDSN)
}

func initialize(ctx context.Context, adminDSN string) error {
	conn, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	if _, err = conn.Exec(ctx, initSQL); err != nil {
		return fmt.Errorf("init: %w", err)
	}

	return nil
}

func dsn(user, endpoint string) string {
	return fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", user, user, endpoint, database)
}

func freePort(ctx context.Context) (int, error) {
	var listenConfig net.ListenConfig

	listener, err := listenConfig.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("find free port: %w", err)
	}
	defer func() { _ = listener.Close() }()

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, errors.New("find free port: not a TCP address")
	}

	return addr.Port, nil
}
