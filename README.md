# OCPP Server

A scalable and robust OCPP (Open Charge Point Protocol) server implementation in Go, designed to manage and communicate with electric vehicle charging stations.

## Features

- **OCPP 1.6 Support**: Full implementation of the OCPP 1.6 protocol for communication with charging stations
- **WebSocket Interface**: Real-time bidirectional communication with charge points
- **RESTful API**: HTTP API for integration with other systems and administrative control
- **Database Integration**: Persistent storage with both SQLite and PostgreSQL support
- **Background Services**: Automated meter value collection, offline transaction monitoring, and data backup
- **Flexible Configuration**: Environment variable-based configuration for easy deployment
- **TLS Support**: Optional secure communication with charge points and API clients
- **Docker Support**: Container-ready deployment for cloud environments

## Getting Started

### Prerequisites

- Go 1.21 or higher
- SQLite or PostgreSQL (optional for production deployments)

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

# Run the container with SQLite
docker run -p 9000:9000 -p 9001:9001 -e DB_TYPE=sqlite -v ocpp-data:/app/data ocpp-server

# Run with PostgreSQL
docker run -p 9000:9000 -p 9001:9001 \
  -e DB_TYPE=postgres \
  -e DB_HOST=your-postgres-host \
  -e DB_PORT=5432 \
  -e DB_USER=postgres \
  -e DB_PASSWORD=yourpassword \
  -e DB_NAME=ocpp_server \
  ocpp-server
```

### Running locally

```bash
# Run with SQLite (default)
./ocpp-server

# Run with custom configuration
DB_TYPE=postgres DB_HOST=localhost DB_PORT=5432 DB_USER=postgres DB_PASSWORD=postgres ./ocpp-server
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
| `DB_TYPE` | Database type (`sqlite` or `postgres`) | `sqlite` |
| `DB_SQLITE_PATH` | Path to SQLite database file | `ocpp_server.db` |
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
  - SQLite and PostgreSQL implementations

## No-Code OCPP Server Deployment Guide

This guide explains how to deploy a ready-made OCPP server without writing any code. We'll use Docker Hub and Google Cloud Run for a completely code-free deployment.

### What You'll Need

- Google Cloud account (with billing enabled)
- Web browser (for Google Cloud Console)
- No coding experience required!

### Step 1: Set Up Google Cloud Project

1. Visit [Google Cloud Console](https://console.cloud.google.com/)
2. Create a new project by clicking on the project dropdown at the top and selecting "New Project"
3. Name your project (e.g., "ocpp-server") and click "Create"
4. Select your new project from the project dropdown

### Step 2: Enable Required APIs

1. In the Cloud Console search bar, type "API Library" and select it
2. Search for and enable these APIs:
   - Cloud Run API
   - Cloud SQL Admin API
   - Cloud Build API
   - Secret Manager API

### Step 3: Create a Database

1. In the search bar, type "SQL" and select "SQL"
2. Click "Create Instance" 
3. Select "PostgreSQL"
4. Provide an instance ID (e.g., "ocpp-db")
5. Set a password (write it down, you'll need it)
6. Under "Customize your instance", choose "Enterprise" and the smallest configuration
7. Click "Create"
8. Once the database is ready, click on it, then go to "Databases" and click "Create Database"
9. Name the database "ocpp_server" and click "Create"
10. Click on "Users" and note the default "postgres" username (or create a new user)

### Step 4: Deploy the API Service in Cloud Run

1. In the search bar, type "Cloud Run" and select it
2. Click "Create Service"
3. In the "Container image URL" field, click "Browse" and then "Configure"
4. Select "Public registries" from the dropdown
5. Type "yourusername/ocpp-server:latest" (replace with the actual Docker Hub image name)
6. Click "Select"
7. Under "Service name" enter "ocpp-api"
8. Select "Allow unauthenticated invocations"
9. Under "Container port", enter "9001"
10. Click "Container, Variables & Secrets, Connections, Security"
11. Go to the "Variables" tab and add these environment variables:
    - OCPP_HOST: 0.0.0.0
    - OCPP_API_PORT: 9001
    - OCPP_WEBSOCKET_PORT: 9000
    - DB_TYPE: postgres
    - DB_HOST: 127.0.0.1
    - DB_PORT: 5432
    - DB_USER: postgres (or your custom user)
    - DB_PASSWORD: [your database password]
    - DB_NAME: ocpp_server
12. Go to the "Connections" tab, check "Cloud SQL connections" and select your database instance
13. Click "Create"

### Step 5: Deploy the WebSocket Service in Cloud Run

1. In Cloud Run, click "Create Service" again
2. Use the same container image as in Step 4
3. Under "Service name" enter "ocpp-websocket"
4. Select "Allow unauthenticated invocations"
5. Under "Container port", enter "9000"
6. Click "Container, Variables & Secrets, Connections, Security"
7. Add the same environment variables as in Step 4
8. Go to the "Connections" tab, check "Cloud SQL connections" and select your database instance
9. Under "Advanced Settings", find "Session affinity" and set it to "Enabled"
10. Click "Create"

### Step 6: Configure TLS (https/wss) with a Domain Name (Optional)

If you want to use your own domain name:

1. In the search bar, type "Load Balancing" and select it
2. Follow the wizard to create an external HTTPS load balancer
3. Point it to your Cloud Run services
4. Configure your domain's DNS to point to the load balancer's IP address

### Step 7: Start Using Your OCPP Server

1. In Cloud Run, click on the "ocpp-api" service
2. Note the URL (looks like "https://ocpp-api-xyz123.run.app")
3. Your API endpoints are available at:
   - `https://ocpp-api-xyz123.run.app/api/status`
   - `https://ocpp-api-xyz123.run.app/api/charge-points`
   - etc.

4. In Cloud Run, click on the "ocpp-websocket" service
5. Note the URL (looks like "https://ocpp-websocket-abc456.run.app")
6. Your WebSocket endpoint is:
   - `wss://ocpp-websocket-abc456.run.app/CP001` (replace CP001 with your charge point ID)

### Troubleshooting

- **Database connection issues:** Make sure your Cloud SQL instance is in the same region as your Cloud Run services
- **Service not working:** Check the logs in Cloud Run by clicking on your service, then "Logs"
- **WebSocket connection failures:** Ensure you're using "wss://" (not "ws://") and that session affinity is enabled

### Costs

- Cloud Run: Pay only for what you use (starts with a free tier)
- Cloud SQL: Smallest instance costs ~$10/month
- Data transfer: Usually minimal for OCPP traffic

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This project is licensed under the MIT License - see the LICENSE file for details.