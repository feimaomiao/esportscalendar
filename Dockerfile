# Node stage for building CSS
FROM node:20-alpine AS css-builder

WORKDIR /build

# Copy package files
COPY package.json package-lock.json ./

# Install npm dependencies
RUN npm ci

# Copy input CSS and config
COPY input.css tailwind.config.js ./

# Create output directory
RUN mkdir -p ./public/static/css

# Build Tailwind CSS
RUN npx @tailwindcss/cli -i input.css -o ./public/static/css/tw.css --minify

# Go build stage
FROM golang:1.23.3-alpine AS builder

# Install build dependencies including templ and sqlc
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /build

# Install templ CLI
RUN go install github.com/a-h/templ/cmd/templ@latest

# Install sqlc
RUN go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest

# Copy go mod files
COPY go.mod go.sum ./

# Download Go dependencies
RUN go mod download

# Copy source code
COPY . .

# Generate SQLC code
RUN sqlc generate

# Generate templ code
RUN templ generate

# Copy built CSS from css-builder stage
COPY --from=css-builder /build/public/static/css/tw.css ./static/css/tw.css

# Build the application
# CGO_ENABLED=0 for static binary
# -ldflags="-w -s" to strip debug info and reduce size
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o esportscalendar .

# Runtime stage
FROM alpine:latest

# Install ca-certificates for HTTPS and tzdata for timezone support
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Create non-root user
RUN addgroup -g 1000 appuser && \
    adduser -D -u 1000 -G appuser appuser

# Copy binary from builder
COPY --from=builder /build/esportscalendar /app/esportscalendar

# Change ownership
RUN chown -R appuser:appuser /app

# Switch to non-root user
USER appuser

# Expose port
EXPOSE 8080

# Run the application
CMD ["/app/esportscalendar"]
