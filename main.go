package main

import (
	"net/http"
	"strings"

	"github.com/feimaomiao/esportscalendar/middleware"
	"go.uber.org/zap"

	// loads .env file automatically.
	_ "github.com/joho/godotenv/autoload"
)

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	logger.Info("Starting application")
	mux := http.NewServeMux()

	// Serve static files for CSS and JS with proper MIME types
	fileServer := http.FileServer(http.Dir("./static"))
	mux.Handle("/static/", http.StripPrefix("/static/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set proper MIME types for CSS and JS files
		if len(r.URL.Path) > 4 && r.URL.Path[len(r.URL.Path)-4:] == ".css" {
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
		} else if len(r.URL.Path) > 3 && r.URL.Path[len(r.URL.Path)-3:] == ".js" {
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		}
		fileServer.ServeHTTP(w, r)
	})))

	mw := middleware.InitMiddleHandler(logger)

	// Routes
	mux.HandleFunc("/lts", mw.SecondPageHandler)
	mux.HandleFunc("/preview", mw.PreviewHandler)
	mux.HandleFunc("/export", mw.ExportHandler)
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
	if err := http.ListenAndServe(":8080", mux); err != nil {
		logger.Fatal("Server failed to start", zap.Error(err))
	}
}
