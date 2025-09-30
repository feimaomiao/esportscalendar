tw:
	@npx @tailwindcss/cli -i input.css -o ./public/static/css/tw.css --watch

dev:
	@templ generate -watch -proxyport=7332 -proxy="http://localhost:8080" -open-browser=false -cmd="go run main.go"
