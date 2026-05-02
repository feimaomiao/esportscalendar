package middleware

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/feimaomiao/esportscalendar/dbtypes"
)

type Middleware struct {
	DB         *pgxpool.Pool
	DBConn     dbtypes.Querier
	Context    context.Context
	RedisCache Cache
	Logger     *zap.Logger
	BaseURL    string
}

func InitMiddleHandler(logger *zap.Logger) Middleware {
	logger.Info("Initializing middleware with database connection")
	ctx := context.Background()

	pgHost := os.Getenv("POSTGRES_HOST")
	if pgHost == "" {
		pgHost = "postgres"
	}
	pgPort := os.Getenv("POSTGRES_PORT")
	if pgPort == "" {
		pgPort = "5432"
	}
	// Build the pool config from a sanitized base string (no password) and
	// set the password on the struct directly so it never appears in any
	// format-string buffer that could end up in a log.
	baseConnStr := fmt.Sprintf("host=%s port=%s user=%s dbname=esports sslmode=disable",
		pgHost, pgPort,
		os.Getenv("postgres_user"))
	pgxCfg, err := pgxpool.ParseConfig(baseConnStr)
	if err != nil {
		panic(err)
	}
	pgxCfg.ConnConfig.Password = os.Getenv("postgres_password")
	conn, err := pgxpool.NewWithConfig(ctx, pgxCfg)
	if err != nil {
		panic(err)
	}

	dbConn := dbtypes.New(conn)

	// Clean up old unfinished matches from before today
	if markErr := dbConn.MarkPastUnfinishedMatchesAsFinished(ctx); markErr != nil {
		logger.Warn("Failed to mark past unfinished matches as finished", zap.Error(markErr))
	}

	// Declare cache as the interface type up front so that a Redis init
	// failure leaves a true-nil interface (not a typed-nil *RedisCache boxed
	// inside an interface, which would silently break the `m.RedisCache != nil`
	// guards across handlers).
	var cache Cache
	redisCache, err := NewRedisCache(ctx, logger)
	if err != nil {
		logger.Error("Failed to initialize Redis cache, falling back to no cache", zap.Error(err))
	} else {
		cache = redisCache
	}

	// Get base URL from environment variable with default fallback
	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = "https://esportscalendar.app"
		logger.Info("BASE_URL not set, using default", zap.String("base_url", baseURL))
	} else {
		logger.Info("Using BASE_URL from environment", zap.String("base_url", baseURL))
	}

	return Middleware{
		DB:         conn,
		DBConn:     dbConn,
		Context:    ctx,
		RedisCache: cache,
		Logger:     logger,
		BaseURL:    baseURL,
	}
}

// Cleanup performs cleanup operations on shutdown.
func (m *Middleware) Cleanup() {
	m.Logger.Info("Starting cleanup")

	// Close database connection
	if m.DB != nil {
		m.DB.Close()
		m.Logger.Info("Database connection closed")
	}

	// Close Redis connection
	if m.RedisCache != nil {
		if err := m.RedisCache.Close(); err != nil {
			m.Logger.Error("Failed to close Redis cache", zap.Error(err))
		}
	}

	m.Logger.Info("Cleanup complete")
}
