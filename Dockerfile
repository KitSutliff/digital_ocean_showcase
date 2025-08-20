# Multi-stage build for efficiency
FROM golang:1.22.8 AS builder

WORKDIR /app
COPY go.mod ./
# Remove go.sum copy since it doesn't exist and isn't needed (no external deps)

# Copy source code
COPY . .

# Build binary
RUN go build -o package-indexer ./app/cmd/server

# Production image - using Ubuntu as required by challenge
FROM ubuntu:22.04

# Install netcat for healthcheck and set up non-root user
RUN apt-get update && apt-get install -y netcat-openbsd && \
    apt-get clean && rm -rf /var/lib/apt/lists/* && \
    groupadd -g 1001 appgroup && \
    useradd -u 1001 -g appgroup -s /bin/bash appuser

WORKDIR /app
COPY --from=builder /app/package-indexer .

# Change ownership and switch user
RUN chown appuser:appgroup package-indexer
USER appuser

EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD nc -z localhost 8080 || exit 1

CMD ["./package-indexer", "-quiet"]
