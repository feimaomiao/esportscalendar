package main

import (
	"log"
	"net/http"

	"github.com/feimaomiao/esportscalendar/middleware"

	// loads .env file automatically.
	_ "github.com/joho/godotenv/autoload"
)

func main() {
	log.Println("[ENTRY] main() - Starting application")
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

	mw := middleware.InitMiddleHandler()

	// Routes
	mux.HandleFunc("/", mw.IndexHandler)
	mux.HandleFunc("/lts", mw.SecondPageHandler)
	mux.HandleFunc("/preview", mw.PreviewHandler)
	mux.HandleFunc("/api/league-options/", mw.LeagueOptionsHandler)
	mux.HandleFunc("/api/team-options/", mw.TeamOptionsHandler)

	log.Println("Server starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
