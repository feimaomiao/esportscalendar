// Package testdb spins a real Postgres in a Docker container for
// integration tests. Gate consumers with a build tag (e.g. //go:build
// dbtest) so default `go test ./...` stays fast.
//
// Typical usage:
//
//	//go:build dbtest
//
//	package mypkg
//
//	var pool *pgxpool.Pool
//
//	func TestMain(m *testing.M) { testdb.RunMain(m, &pool) }
package testdb

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	// pgReadyOccurrences is 2 because the official postgres image emits
	// "ready to accept connections" once during init (when running setup
	// scripts on a private socket) and again when serving on TCP. Waiting
	// for the second occurrence avoids a race where init scripts finish
	// after we connect.
	pgReadyOccurrences = 2

	// pgStartupTimeout is generous because cold-pulling the image on
	// first run can take longer than warm-cache startup.
	pgStartupTimeout = 60 * time.Second
)

// schemaFiles, in order, populate a fresh test DB. Paths are relative
// to the repo root (located via go.mod walk-up).
func schemaFiles() []string {
	return []string{
		"sqlc/schema.sql", // public schema (shared, owned by stalka)
	}
}

// RunMain is the canonical TestMain entry. It spins one Postgres
// container per package run, applies the schema files, points
// *poolPtr at a live pool, and tears everything down on exit.
//
// On any setup failure it writes to stderr and exits non-zero — the
// container would otherwise leak.
func RunMain(m *testing.M, poolPtr **pgxpool.Pool) {
	ctx := context.Background()

	ctr, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("ecal_test"),
		postgres.WithUsername("ecal"),
		postgres.WithPassword("ecal"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(pgReadyOccurrences).
				WithStartupTimeout(pgStartupTimeout),
		),
	)
	if err != nil {
		failf(nil, "start container: %v", err)
	}

	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		failf(ctr, "get DSN: %v", err)
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		failf(ctr, "create pool: %v", err)
	}

	if applyErr := applySchemas(ctx, pool); applyErr != nil {
		pool.Close()
		failf(ctr, "apply schemas: %v", applyErr)
	}

	*poolPtr = pool
	code := m.Run()

	pool.Close()
	if termErr := ctr.Terminate(ctx); termErr != nil {
		fmt.Fprintf(os.Stderr, "testdb: terminate container: %v\n", termErr)
	}
	os.Exit(code)
}

func failf(ctr *postgres.PostgresContainer, format string, args ...any) {
	fmt.Fprintf(os.Stderr, "testdb: "+format+"\n", args...)
	if ctr != nil {
		_ = ctr.Terminate(context.Background())
	}
	os.Exit(1)
}

func applySchemas(ctx context.Context, pool *pgxpool.Pool) error {
	root, err := projectRoot()
	if err != nil {
		return err
	}
	for _, name := range schemaFiles() {
		path := filepath.Join(root, name)
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("read %s: %w", path, readErr)
		}
		if _, execErr := pool.Exec(ctx, string(body)); execErr != nil {
			return fmt.Errorf("exec %s: %w", path, execErr)
		}
	}
	return nil
}

// projectRoot finds the repo root by walking up from cwd until it finds
// a go.mod file. Tests run with cwd = the package dir, so this works
// regardless of which package invokes the helper.
func projectRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for d := cwd; ; d = filepath.Dir(d) {
		if _, statErr := os.Stat(filepath.Join(d, "go.mod")); statErr == nil {
			return d, nil
		}
		parent := filepath.Dir(d)
		if parent == d {
			return "", fmt.Errorf("go.mod not found from %s", cwd)
		}
	}
}
