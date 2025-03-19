FROM golang:1.21-alpine AS builder

# Set working directory
WORKDIR /build

# Install necessary dependencies for CGO and PostgreSQL
RUN apk add --no-cache gcc musl-dev postgresql-dev

# Copy go.mod and go.sum files to download dependencies
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy the rest of the application code
COPY . .

# Build the application with PostgreSQL support
RUN CGO_ENABLED=1 GOOS=linux go build -a -o ocpp-server .

# Create a minimal production image
FROM alpine:latest

# Install necessary runtime dependencies including PostgreSQL client libs
RUN apk add --no-cache ca-certificates tzdata sqlite-libs postgresql-client libpq

# Set working directory
WORKDIR /app

# Create data directories
RUN mkdir -p /app/data

# Copy the binary from the builder stage
COPY --from=builder /build/ocpp-server /app/

# Copy any required configuration files
COPY --from=builder /build/go.mod /app/
COPY --from=builder /build/go.sum /app/

# Create a non-root user to run the application
RUN addgroup -S ocpp && adduser -S -G ocpp ocpp
RUN chown -R ocpp:ocpp /app

# Create postgres-init directory for initialization scripts
RUN mkdir -p /app/postgres-init
COPY postgres-init/* /app/postgres-init/ 2>/dev/null || :
RUN chown -R ocpp:ocpp /app/postgres-init

# Use the non-root user
USER ocpp

# Set environment variables with sensible defaults for Docker
ENV OCPP_HOST=0.0.0.0
ENV OCPP_WEBSOCKET_PORT=9000
ENV OCPP_API_PORT=9001
ENV OCPP_SYSTEM_NAME=ocpp-central
# Default to PostgreSQL
ENV DB_TYPE=postgres
ENV DB_HOST=postgres
ENV DB_PORT=5432
ENV DB_USER=postgres
ENV DB_PASSWORD=postgres
ENV DB_NAME=ocpp_server
ENV DB_SSL_MODE=disable
# SQLite configuration still available
ENV DB_SQLITE_PATH=/app/data/ocpp_server.db

# Expose ports
EXPOSE 9000 9001

# Set the volume mount point for persistent data
VOLUME /app/data

# Wait for PostgreSQL script
COPY --chown=ocpp:ocpp wait-for-postgres.sh /app/
RUN chmod +x /app/wait-for-postgres.sh

# Set entrypoint with database wait capability
ENTRYPOINT ["/app/ocpp-server"]

# Add healthcheck to verify server is running properly
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD wget -q -O- http://localhost:$OCPP_API_PORT/api/status || exit 1