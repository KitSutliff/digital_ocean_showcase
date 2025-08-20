# Multi-stage build for efficiency
FROM golang:1.24-alpine AS builder

WORKDIR /app
COPY go.mod ./
# Remove go.sum copy since it doesn't exist and isn't needed (no external deps)

# Copy source code
COPY . .

# Build binary
RUN go build -o package-indexer ./cmd/server

# Production image
FROM alpine:latest

# Security: run as non-root
RUN addgroup -g 1001 appgroup && \
    adduser -u 1001 -G appgroup -s /bin/sh -D appuser

WORKDIR /app
COPY --from=builder /app/package-indexer .

# Change ownership and switch user
RUN chown appuser:appgroup package-indexer
USER appuser

EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD nc -z localhost 8080 || exit 1

CMD ["./package-indexer", "-quiet"]
