package middleware

import (
	"context"
	"fmt"
	"os"

	"github.com/feimaomiao/esportscalendar/dbtypes"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

type Middleware struct {
	DB      *pgxpool.Pool
	DBConn  *dbtypes.Queries
	Context context.Context
	Cache   *lru.Cache[string, any]
	Logger  *zap.Logger
}

func InitMiddleHandler(logger *zap.Logger) Middleware {
	logger.Info("Initializing middleware with database connection")
	connStr := fmt.Sprintf("host=localhost port=5432 user=%s password=%s dbname=esports sslmode=disable",
		os.Getenv("postgres_user"),
		os.Getenv("postgres_password"))
	conn, err := pgxpool.New(context.Background(), connStr)
	if err != nil {
		panic(err)
	}

	dbConn := dbtypes.New(conn)

	// Initialize LRU cache with 32 entries
	// technically we should be able to cache the entire games, leagues and teams list?
	lruSize := 32
	cache, err := lru.New[string, any](lruSize)
	if err != nil {
		panic(err)
	}

	return Middleware{
		DB:      conn,
		DBConn:  dbConn,
		Context: context.Background(),
		Cache:   cache,
		Logger:  logger,
	}
}
