package components

type Option struct {
	ID      string
	Label   string
	Logo    string
	Slug    string // games.slug — drives the per-game accent CSS hook.
	Checked bool
}

type SeriesOption struct {
	ID   int32  `json:"id"`
	Name string `json:"name"`
}

// LogoPath returns a formatted path for local logo files.
func LogoPath(filename string) string {
	return "/static/images/" + filename
}

// DefaultLogo returns the path to a default logo.
func DefaultLogo() string {
	return "/static/images/default-logo.png"
}
