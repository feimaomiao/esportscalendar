package middleware

import (
	"context"
	"fmt"
	"os"

	"github.com/feimaomiao/esportscalendar/dbtypes"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

type Middleware struct {
	DB         *pgxpool.Pool
	DBConn     *dbtypes.Queries
	Context    context.Context
	RedisCache *RedisCache
	Logger     *zap.Logger
}

func InitMiddleHandler(logger *zap.Logger) Middleware {
	logger.Info("Initializing middleware with database connection")
	ctx := context.Background()

	connStr := fmt.Sprintf("host=postgres port=5432 user=%s password=%s dbname=esports sslmode=disable",
		os.Getenv("postgres_user"),
		os.Getenv("postgres_password"))
	conn, err := pgxpool.New(ctx, connStr)
	if err != nil {
		panic(err)
	}

	dbConn := dbtypes.New(conn)

	// Initialize Redis cache
	redisCache, err := NewRedisCache(ctx, logger)
	if err != nil {
		logger.Error("Failed to initialize Redis cache, falling back to no cache", zap.Error(err))
		// Continue without cache - app will still work but slower
		redisCache = nil
	}

	return Middleware{
		DB:         conn,
		DBConn:     dbConn,
		Context:    ctx,
		RedisCache: redisCache,
		Logger:     logger,
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
