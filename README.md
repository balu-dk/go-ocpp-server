# OCPP Server

A scalable and robust OCPP (Open Charge Point Protocol) server implementation in Go, designed to manage and communicate with electric vehicle charging stations.

## Features

- **OCPP 1.6 Support**: Full implementation of the OCPP 1.6 protocol for communication with charging stations
- **WebSocket Interface**: Real-time bidirectional communication with charge points
- **RESTful API**: HTTP API for integration with other systems and administrative control
- **PostgreSQL Database**: Persistent storage with PostgreSQL support
- **Background Services**: Automated meter value collection, offline transaction monitoring, and data backup
- **Flexible Configuration**: Environment variable-based configuration for easy deployment
- **TLS Support**: Optional secure communication with charge points and API clients
- **Docker Support**: Container-ready deployment for cloud environments

## Getting Started

### Prerequisites

- Go 1.21 or higher
- PostgreSQL database

### Installation

1. Clone the repository:
   ```
   git clone https://github.com/yourusername/ocpp-server.git
   cd ocpp-server
   ```

2. Build the server:
   ```
   go build -o ocpp-server
   ```

### Running with Docker

```bash
# Build the Docker image
docker build -t ocpp-server .

# Run with PostgreSQL
docker-compose up -d
```

### Running locally

```bash
# Make sure PostgreSQL is running and accessible
export DB_TYPE=postgres
export DB_HOST=localhost
export DB_PORT=5432
export DB_USER=postgres
export DB_PASSWORD=postgres
export DB_NAME=ocpp_server
./ocpp-server
```

## Configuration

The server can be configured using environment variables:

### OCPP Server Configuration

| Variable | Description | Default |
|----------|-------------|---------|
| `OCPP_HOST` | Hostname or IP address for binding the server | `localhost` |
| `OCPP_WEBSOCKET_PORT` | Port for WebSocket OCPP connections | `9000` |
| `OCPP_API_PORT` | Port for the HTTP API | `9001` |
| `OCPP_SYSTEM_NAME` | Name of the central system | `ocpp-central` |
| `OCPP_HEARTBEAT_INTERVAL` | Interval in seconds between charge point heartbeats | `60` |
| `OCPP_USE_TLS` | Enable TLS for secure connections | `false` |
| `OCPP_CERT_FILE` | Path to TLS certificate file | `cert.pem` |
| `OCPP_KEY_FILE` | Path to TLS key file | `key.pem` |

### Database Configuration

| Variable | Description | Default |
|----------|-------------|---------|
| `DB_TYPE` | Database type (always `postgres`) | `postgres` |
| `DB_HOST` | PostgreSQL host | `localhost` |
| `DB_PORT` | PostgreSQL port | `5432` |
| `DB_USER` | PostgreSQL username | `postgres` |
| `DB_PASSWORD` | PostgreSQL password | `postgres` |
| `DB_NAME` | PostgreSQL database name | `ocpp_server` |
| `DB_SSL_MODE` | PostgreSQL SSL mode | `disable` |

## API Reference

The server provides a comprehensive HTTP API for management and integration. Below is the list of available endpoints:

### Server Status

```
GET /api/status
```

Returns the current server status, number of connected charge points, and database information.

### Charge Points

```
GET /api/charge-points
```

Returns a list of all charge points.

```
GET /api/charge-points/:id
```

Returns details for a specific charge point and its connectors.

### Connectors

```
GET /api/connectors?chargePointId=:id
```

Returns all connectors for a charge point.

### Transactions

```
GET /api/transactions
```

Returns all transactions with optional filtering.

Query parameters:
- `chargePointId`: Filter by charge point ID
- `isComplete`: Filter by completion status (`true` or `false`)
- `transactionId`: Get a specific transaction

### Logs

```
GET /api/logs
```

Returns system logs with optional filtering.

Query parameters:
- `chargePointId`: Filter by charge point ID
- `level`: Filter by log level (INFO, WARNING, ERROR)
- `limit`: Limit number of results (default: 100)
- `offset`: Pagination offset

### Authorizations

```
GET /api/authorizations
GET /api/authorizations?idTag=:id
POST /api/authorizations
DELETE /api/authorizations?idTag=:id
```

Manage RFID cards/tokens for authorization.

### Commands

The following command endpoints allow control of charge points:

#### Remote Start Transaction

```
POST /api/commands/remote-start
```

Request body:
```json
{
  "chargePointId": "CP001",
  "idTag": "RFID123",
  "connectorId": 1
}
```

#### Remote Stop Transaction

```
POST /api/commands/remote-stop
```

Request body:
```json
{
  "chargePointId": "CP001",
  "transactionId": 12345
}
```

Alternatively, you can use connectorId instead of transactionId:
```json
{
  "chargePointId": "CP001",
  "connectorId": 1,
  "reason": "Completed"
}
```

#### Reset Charge Point

```
POST /api/commands/reset
```

Request body:
```json
{
  "chargePointId": "CP001",
  "type": "Soft"
}
```

Type can be either "Soft" or "Hard".

#### Unlock Connector

```
POST /api/commands/unlock-connector
```

Request body:
```json
{
  "chargePointId": "CP001",
  "connectorId": 1
}
```

#### Get Configuration

```
GET /api/commands/get-configuration?chargePointId=CP001&keys=key1,key2
```

Or using POST:
```
POST /api/commands/get-configuration
```

Request body:
```json
{
  "chargePointId": "CP001",
  "keys": ["key1", "key2"]
}
```

#### Change Configuration

```
POST /api/commands/change-configuration
```

Request body:
```json
{
  "chargePointId": "CP001",
  "key": "HeartbeatInterval",
  "value": "60"
}
```

#### Clear Cache

```
POST /api/commands/clear-cache
```

Request body:
```json
{
  "chargePointId": "CP001"
}
```

#### Trigger Message

```
POST /api/commands/trigger-message
```

Request body:
```json
{
  "chargePointId": "CP001",
  "requestedMessage": "StatusNotification",
  "connectorId": 1
}
```

Valid message types: BootNotification, DiagnosticsStatusNotification, FirmwareStatusNotification, Heartbeat, MeterValues, StatusNotification.

#### Generic Command

```
POST /api/commands/generic
```

Request body:
```json
{
  "chargePointId": "CP001",
  "action": "GetDiagnostics",
  "payload": {
    "location": "ftp://example.com/diagnostics",
    "retries": 3
  }
}
```

### Administrative Endpoints

#### Close Transaction

```
POST /api/admin/close-transaction
```

Request body:
```json
{
  "transactionId": 12345,
  "finalEnergy": 10.5,
  "reason": "Administrative"
}
```

#### Energy Report

```
GET /api/reports/energy?startDate=2023-01-01&endDate=2023-01-31
```

Generates an energy consumption report for the specified period.

## Architecture

The server is structured into several modules:

- **ocpp** - Core OCPP protocol implementation
  - WebSocket handling
  - Message processing
  - Command management
  - Central system handler
- **server** - Web services and API
  - API server implementation
  - Command endpoints
  - Admin endpoints
- **database** - Data persistence layer
  - Database models
  - Database service interface
  - PostgreSQL implementation

## Deployment

See the deployment guides for specific cloud providers:
- [DigitalOcean Deployment Guide](DIGITALOCEAN.md)

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This project is licensed under the MIT License - see the LICENSE file for details.