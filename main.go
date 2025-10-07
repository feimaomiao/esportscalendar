package main

import (
	"context"
	"embed"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/feimaomiao/esportscalendar/middleware"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	// loads .env file automatically.
	_ "github.com/joho/godotenv/autoload"
)

//go:embed static
var staticFS embed.FS

func main() {
	logger, err := zap.NewProduction()
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = logger.Sync() // Ignore error on shutdown
	}()

	logger.Info("Starting application")

	// Set Gin to release mode for production
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()

	// Add Gin's built-in recovery and logging middleware
	router.Use(gin.Recovery())

	// Serve embedded static files (CSS, JS, images, icons)
	staticSubFS, err := fs.Sub(staticFS, "static")
	if err != nil {
		logger.Fatal("Failed to create sub filesystem", zap.Error(err))
	}
	fileServer := http.FileServer(http.FS(staticSubFS))
	router.GET("/static/*filepath", func(c *gin.Context) {
		// Set proper MIME types for CSS and JS files
		if strings.HasSuffix(c.Request.URL.Path, ".css") {
			c.Header("Content-Type", "text/css; charset=utf-8")
		} else if strings.HasSuffix(c.Request.URL.Path, ".js") {
			c.Header("Content-Type", "application/javascript; charset=utf-8")
		}
		c.Request.URL.Path = strings.TrimPrefix(c.Request.URL.Path, "/static/")
		fileServer.ServeHTTP(c.Writer, c.Request)
	})

	mw := middleware.InitMiddleHandler(logger)

	// Serve robots.txt and sitemap.xml from embedded static files
	router.GET("/robots.txt", func(c *gin.Context) {
		c.Header("Content-Type", "text/plain; charset=utf-8")
		data, readErr := staticFS.ReadFile("static/robots.txt")
		if readErr != nil {
			c.Status(http.StatusNotFound)
			return
		}
		c.Data(http.StatusOK, "text/plain; charset=utf-8", data)
	})
	router.GET("/sitemap.xml", func(c *gin.Context) {
		c.Header("Content-Type", "application/xml; charset=utf-8")
		data, readErr := staticFS.ReadFile("static/sitemap.xml")
		if readErr != nil {
			c.Status(http.StatusNotFound)
			return
		}
		c.Data(http.StatusOK, "application/xml; charset=utf-8", data)
	})

	// Routes
	router.GET("/", mw.IndexHandler)
	router.Any("/lts", mw.SecondPageHandler)
	router.POST("/preview", mw.PreviewHandler)
	router.POST("/export", mw.ExportHandler)
	router.GET("/how-to-use", mw.HowToUseHandler)
	router.GET("/about", mw.AboutHandler)
	router.GET("/api/league-options/*param", mw.LeagueOptionsHandler)
	router.GET("/api/team-options/*param", mw.TeamOptionsHandler)

	// NoRoute handler for .ics files (calendar downloads)
	router.NoRoute(func(c *gin.Context) {
		if strings.HasSuffix(c.Request.URL.Path, ".ics") {
			mw.CalendarHandler(c)
		} else {
			c.Status(http.StatusNotFound)
		}
	})

	logger.Info("Server starting", zap.String("port", "8080"))
	secsInMin := 60
	minute := time.Duration(secsInMin) * time.Second
	// Create server with timeouts for security
	server := &http.Server{
		Addr:              ":8080",
		Handler:           router,
		ReadTimeout:       minute,
		ReadHeaderTimeout: minute,
		WriteTimeout:      minute,
		IdleTimeout:       minute,
	}

	// Channel to listen for interrupt signals
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Start server in a goroutine
	go func() {
		if serverErr := server.ListenAndServe(); serverErr != nil && serverErr != http.ErrServerClosed {
			logger.Fatal("Server failed to start", zap.Error(serverErr))
		}
	}()

	logger.Info("Server started successfully")

	// Wait for interrupt signal
	<-quit
	logger.Info("Shutdown signal received")

	// Create shutdown context with timeout
	const shutdownTimeout = 10 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	// Gracefully shutdown the server
	if shutdownErr := server.Shutdown(ctx); shutdownErr != nil {
		logger.Error("Server forced to shutdown", zap.Error(shutdownErr))
	} else {
		logger.Info("Server shutdown gracefully")
	}

	// Perform cleanup
	mw.Cleanup()
}
