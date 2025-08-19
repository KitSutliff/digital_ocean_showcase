# Multi-stage build for optimal image size and security
FROM golang:1.21-alpine AS builder

# Install git and ca-certificates (needed for go modules and HTTPS)
RUN apk add --no-cache git ca-certificates

# Set working directory
WORKDIR /app

# Copy go mod files first (for better layer caching)
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary with optimizations
RUN CGO_ENABLED=0 GOOS=linux go build \
    -a -installsuffix cgo \
    -ldflags '-extldflags "-static"' \
    -o package-indexer ./cmd/server

# Production stage - use Ubuntu as specified in requirements
FROM ubuntu:22.04

# Install ca-certificates and netcat for health checks
RUN apt-get update && \
    apt-get install -y ca-certificates netcat && \
    rm -rf /var/lib/apt/lists/*

# Create non-root user for security
RUN useradd -r -s /bin/false indexer

# Set working directory
WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /app/package-indexer .

# Change ownership to non-root user
RUN chown indexer:indexer /app/package-indexer

# Switch to non-root user
USER indexer

# Expose port
EXPOSE 8080

# Add health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD nc -z localhost 8080 || exit 1

# Run the binary
CMD ["./package-indexer"]
