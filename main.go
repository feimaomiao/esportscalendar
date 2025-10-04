package main

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/feimaomiao/esportscalendar/middleware"
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
	mux := http.NewServeMux()

	// Serve embedded static files (CSS, JS, images, icons)
	staticSubFS, err := fs.Sub(staticFS, "static")
	if err != nil {
		logger.Fatal("Failed to create sub filesystem", zap.Error(err))
	}
	fileServer := http.FileServer(http.FS(staticSubFS))
	mux.Handle("/static/", http.StripPrefix("/static/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set proper MIME types for CSS and JS files
		if strings.HasSuffix(r.URL.Path, ".css") {
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
		} else if strings.HasSuffix(r.URL.Path, ".js") {
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		}
		fileServer.ServeHTTP(w, r)
	})))

	mw := middleware.InitMiddleHandler(logger)

	// Routes
	mux.HandleFunc("/lts", mw.SecondPageHandler)
	mux.HandleFunc("/preview", mw.PreviewHandler)
	mux.HandleFunc("/export", mw.ExportHandler)
	mux.HandleFunc("/how-to-use", mw.HowToUseHandler)
	mux.HandleFunc("/about", mw.AboutHandler)
	mux.HandleFunc("/api/league-options/", mw.LeagueOptionsHandler)
	mux.HandleFunc("/api/team-options/", mw.TeamOptionsHandler)

	// Calendar handler and index - handles both /:hash.ics and /
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".ics") {
			mw.CalendarHandler(w, r)
		} else {
			mw.IndexHandler(w, r)
		}
	})

	logger.Info("Server starting", zap.String("port", "8080"))
	secsInMin := 60
	minute := time.Duration(secsInMin) * time.Second
	// Create server with timeouts for security
	server := &http.Server{
		Addr:              ":8080",
		Handler:           mux,
		ReadTimeout:       minute,
		ReadHeaderTimeout: minute,
		WriteTimeout:      minute,
		IdleTimeout:       minute,
	}

	if serverErr := server.ListenAndServe(); serverErr != nil {
		logger.Fatal("Server failed to start", zap.Error(serverErr))
	}
}
