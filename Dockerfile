# Build stage
FROM golang:1.23.3-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code and static files
COPY . .

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
