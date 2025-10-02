package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/feimaomiao/esportscalendar/components"
	"github.com/feimaomiao/esportscalendar/dbtypes"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Middleware struct {
	DB      *pgxpool.Pool
	DBConn  *dbtypes.Queries
	Context context.Context
}

func InitMiddleHandler() Middleware {
	log.Println("[ENTRY] InitMiddleHandler() - Initializing middleware with database connection")
	connStr := fmt.Sprintf("host=localhost port=5432 user=%s password=%s dbname=esports sslmode=disable",
		os.Getenv("postgres_user"),
		os.Getenv("postgres_password"))
	conn, err := pgxpool.New(context.Background(), connStr)
	if err != nil {
		panic(err)
	}

	dbConn := dbtypes.New(conn)

	return Middleware{
		DB:      conn,
		DBConn:  dbConn,
		Context: context.Background(),
	}
}
func (m *Middleware) IndexHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("[ENTRY] IndexHandler() - Processing request from %s %s", r.Method, r.URL.Path)
	// Fetch options from database or define statically

	games, err := m.DBConn.GetAllGames(m.Context)
	if err != nil {
		http.Error(w, "Failed to fetch games", http.StatusInternalServerError)
		return
	}

	var options []components.Option
	for _, game := range games {
		logo := components.DefaultLogo()
		ignored := map[int]bool{20: true, 25: true, 27: true, 29: true, 30: true}

		if ignored[int(game.ID)] {
			continue
		}
		if game.Slug.Valid {
			logo = components.LogoPath(game.Slug.String) + ".png"
		}
		options = append(options, components.Option{
			ID:      fmt.Sprintf("%d", game.ID),
			Label:   game.Name,
			Logo:    logo,
			Checked: false,
		})
	}

	component := components.Index(options)
	component.Render(m.Context, w)
}

func (m *Middleware) SecondPageHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("[ENTRY] SecondPageHandler() - Processing request from %s %s", r.Method, r.URL.Path)
	// Parse selected options from query params or form data
	r.ParseForm()
	selectedOptions := r.Form["options"]

	// For HTMX partial updates
	if r.Header.Get("HX-Request") == "true" {
		component := components.SecondPageContent(selectedOptions)
		component.Render(m.Context, w)
		return
	}

	// Full page load
	component := components.SecondPageContent(selectedOptions)
	component.Render(m.Context, w)
}
func (m *Middleware) LeagueOptionsHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("[ENTRY] LeagueOptionsHandler() - Processing API request from %s %s", r.Method, r.URL.Path)
	// Extract game ID from URL path
	path := strings.TrimPrefix(r.URL.Path, "/api/league-options/")
	gameID, err := strconv.ParseInt(path, 10, 32)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		response := map[string]interface{}{
			"error":   true,
			"message": "Invalid game ID",
			"leagues": []interface{}{},
		}
		json.NewEncoder(w).Encode(response)
		return
	}

	// Fetch leagues from database
	leagues, err := m.DBConn.GetLeaguesByGameID(m.Context, int32(gameID))
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		var message string
		// Check if it's a connection error or other database issue
		if strings.Contains(err.Error(), "connection") || strings.Contains(err.Error(), "connect") {
			message = "Database connection error. Please try again later."
		} else {
			message = "Unable to load leagues. Please refresh the page: " + err.Error()
		}
		response := map[string]interface{}{
			"error":   true,
			"message": message,
			"leagues": []interface{}{},
		}
		json.NewEncoder(w).Encode(response)
		return
	}

	// Convert to response format
	type LeagueResponse struct {
		ID    int32  `json:"id"`
		Name  string `json:"name"`
		Image string `json:"image"`
	}

	var leagueList []LeagueResponse
	for _, league := range leagues {
		image := "/static/images/default-logo.png"
		if league.ImageLink.Valid && league.ImageLink.String != "" {
			image = league.ImageLink.String
		}
		leagueList = append(leagueList, LeagueResponse{
			ID:    league.ID,
			Name:  league.Name,
			Image: image,
		})
	}

	// Return JSON response
	w.Header().Set("Content-Type", "application/json")
	response := map[string]interface{}{
		"error":   false,
		"message": "",
		"leagues": leagueList,
	}
	json.NewEncoder(w).Encode(response)
}
