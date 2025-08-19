# Multi-stage build for LogSieve
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN make build

# Final stage
FROM alpine:3.18

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN adduser -D -s /bin/sh logsieve

# Create directories
RUN mkdir -p /etc/logsieve/profiles \
    /var/cache/logsieve/profiles \
    /var/log/logsieve && \
    chown -R logsieve:logsieve /etc/logsieve /var/cache/logsieve /var/log/logsieve

# Copy binaries from builder
COPY --from=builder /app/dist/server /usr/local/bin/logsieve-server
COPY --from=builder /app/dist/logsieve /usr/local/bin/logsieve

# Copy example config
COPY --from=builder /app/profiles/ /etc/logsieve/profiles/

# Switch to non-root user
USER logsieve

# Expose ports
EXPOSE 8080 9090

# Set default environment variables
ENV LOGSIEVE_SERVER_PORT=8080 \
    LOGSIEVE_METRICS_PORT=9090 \
    LOGSIEVE_LOGGING_LEVEL=info

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Default command
CMD ["logsieve-server"]