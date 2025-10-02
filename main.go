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

	// Serve static files for CSS and JS
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))

	mw := middleware.InitMiddleHandler()

	// Routes
	mux.HandleFunc("/", mw.IndexHandler)
	mux.HandleFunc("/second", mw.SecondPageHandler)
	mux.HandleFunc("/api/league-options/", mw.LeagueOptionsHandler)

	log.Println("Server starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
