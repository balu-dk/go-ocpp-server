FROM golang:1.21-alpine AS builder

# Set working directory
WORKDIR /build

# Install necessary dependencies for CGO and database drivers
RUN apk add --no-cache gcc musl-dev postgresql-dev

# Copy go.mod and go.sum files to download dependencies
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy the rest of the application code
COPY . .

# Build the application with database support
RUN CGO_ENABLED=1 GOOS=linux go build -a -o ocpp-server .

# Create a minimal production image
FROM alpine:latest

# Install necessary runtime dependencies
RUN apk add --no-cache ca-certificates tzdata sqlite-libs 
# Uncomment to add PostgreSQL client support
# RUN apk add --no-cache postgresql-client libpq

# Set working directory
WORKDIR /app

# Create data directories
RUN mkdir -p /app/data

# Copy the binary from the builder stage
COPY --from=builder /build/ocpp-server /app/

# Create a non-root user to run the application
RUN addgroup -S ocpp && adduser -S -G ocpp ocpp
RUN chown -R ocpp:ocpp /app

# Use the non-root user
USER ocpp

# Set environment variables with sensible defaults for Docker
ENV OCPP_HOST=0.0.0.0
ENV OCPP_WEBSOCKET_PORT=9000
ENV OCPP_API_PORT=9001
ENV OCPP_SYSTEM_NAME=ocpp-central
# Default to SQLite
ENV DB_TYPE=sqlite
ENV DB_SQLITE_PATH=/app/data/ocpp_server.db

# PostgreSQL configuration (commented out)
# ENV DB_TYPE=postgres
# ENV DB_HOST=postgres
# ENV DB_PORT=5432
# ENV DB_USER=postgres
# ENV DB_PASSWORD=postgres
# ENV DB_NAME=ocpp_server
# ENV DB_SSL_MODE=disable

# Expose ports
EXPOSE 9000 9001

# Set the volume mount point for persistent data
VOLUME /app/data

# Set entrypoint
ENTRYPOINT ["/app/ocpp-server"]

# Add healthcheck to verify server is running properly
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD wget -q -O- http://localhost:$OCPP_API_PORT/api/status || exit 1