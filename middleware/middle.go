package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/feimaomiao/esportscalendar/components"
	"github.com/feimaomiao/esportscalendar/dbtypes"
	"github.com/jackc/pgx/v5"
)

type Middleware struct {
	DB      *pgx.Conn
	DBConn  *dbtypes.Queries
	Context context.Context
}

func InitMiddleHandler() Middleware {
	connStr := fmt.Sprintf("host=localhost port=5432 user=%s password=%s dbname=esports sslmode=disable",
		os.Getenv("postgres_user"),
		os.Getenv("postgres_password"))
	conn, err := pgx.Connect(context.Background(), connStr)
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

func (m *Middleware) SeriesListHandler(w http.ResponseWriter, r *http.Request) {
	// Extract game ID from URL path
	path := strings.TrimPrefix(r.URL.Path, "/api/serieslist/")
	gameID, err := strconv.ParseInt(path, 10, 32)
	if err != nil {
		http.Error(w, "Invalid game ID", http.StatusBadRequest)
		return
	}

	// Fetch series from database
	series, err := m.DBConn.GetSeriesByGameID(m.Context, int32(gameID))
	if err != nil {
		http.Error(w, "Failed to fetch series", http.StatusInternalServerError)
		return
	}

	// Convert to response format
	type SeriesResponse struct {
		ID   int32  `json:"id"`
		Name string `json:"name"`
	}

	var response []SeriesResponse
	for _, serie := range series {
		response = append(response, SeriesResponse{
			ID:   serie.ID,
			Name: serie.Name,
		})
	}

	// Return JSON response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (m *Middleware) LeagueOptionsHandler(w http.ResponseWriter, r *http.Request) {
	// Extract game ID from URL path
	path := strings.TrimPrefix(r.URL.Path, "/api/league-options/")
	gameID, err := strconv.ParseInt(path, 10, 32)
	if err != nil {
		http.Error(w, "Invalid game ID", http.StatusBadRequest)
		return
	}

	// Fetch leagues from database
	leagues, err := m.DBConn.GetLeaguesByGameID(m.Context, int32(gameID))
	if err != nil {
		http.Error(w, "Failed to fetch leagues", http.StatusInternalServerError)
		return
	}

	// Generate HTML options
	w.Header().Set("Content-Type", "text/html")
	if len(leagues) == 0 {
		w.Write([]byte(`<option disabled>No leagues available for this game</option>`))
		return
	}

	for _, league := range leagues {
		w.Write([]byte(fmt.Sprintf(`<option value="%d">%s</option>`, league.ID, league.Name)))
	}
}
