# Deploying go-ocpp-server on DigitalOcean

This guide explains how to deploy the OCPP server on a DigitalOcean Droplet with a managed PostgreSQL database.

## Prerequisites

- DigitalOcean account
- Domain name (optional, but recommended for production)
- Basic knowledge of terminal commands

## Step 1: Create a Managed PostgreSQL Database

1. Log in to your DigitalOcean dashboard
2. Navigate to **Databases** → **Create Database Cluster**
3. Select **PostgreSQL**
4. Choose a plan (Starter at $15/mo is sufficient for testing)
5. Select a datacenter region close to your users
6. Name it "ocpp-db" and click **Create Database Cluster**

Once created, note these connection details from the dashboard:
- Host address
- Port (usually 25060)
- Username (default: `doadmin`)
- Password
- Database name (default: `defaultdb`)

## Step 2: Create a Docker Droplet

1. Navigate to **Droplets** → **Create Droplet**
2. Under Marketplace, select **Docker**
3. Choose a plan (Basic Shared CPU, 2GB RAM / 1 CPU is good to start)
4. Select the **same region** as your database
5. Add your SSH key or set a password
6. Create the Droplet

## Step 3: Deploy Your OCPP Server

SSH into your Droplet:
```bash
ssh root@your-droplet-ip
```

Install docker-compose (if not already installed):
```bash
# Install docker-compose using apt
apt update
apt install -y docker-compose

# Verify installation
docker-compose --version
```

Clone the repository:
```bash
git clone https://github.com/balu-dk/go-ocpp-server.git
cd go-ocpp-server
```

Create a docker-compose file for DigitalOcean:
```bash
nano docker-compose.yml
```

Add the following content (replace database connection details with yours):
```yaml
version: '3.8'

services:
  ocpp-server:
    build:
      context: .
      dockerfile: Dockerfile
    ports:
      - "9000:9000"  # OCPP WebSocket port
      - "9001:9001"  # API port
    environment:
      - OCPP_HOST=0.0.0.0
      - OCPP_WEBSOCKET_PORT=9000
      - OCPP_API_PORT=9001
      - OCPP_SYSTEM_NAME=ocpp-central
      - OCPP_HEARTBEAT_INTERVAL=60
      # DigitalOcean Managed PostgreSQL Configuration
      - DB_TYPE=postgres
      - DB_HOST=your-db-host.db.ondigitalocean.com
      - DB_PORT=25060
      - DB_USER=doadmin
      - DB_PASSWORD=your-db-password
      - DB_NAME=defaultdb
      - DB_SSL_MODE=require
    volumes:
      - ocpp-data:/app/data
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "wget", "-q", "-O-", "http://localhost:9001/api/status"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 5s

volumes:
  ocpp-data:
```

Build and run the container:
```bash
docker-compose up -d
```

## Step 4: Set Up NGINX as a Reverse Proxy

Install NGINX and Certbot:
```bash
apt update
apt install -y nginx certbot python3-certbot-nginx
```

Create an NGINX configuration:
```bash
nano /etc/nginx/sites-available/ocpp-server
```

Add this configuration (replace yourdomain.com with your domain):
```nginx
server {
    listen 80;
    server_name yourdomain.com;
    
    location / {
        return 301 https://$host$request_uri;
    }
}

server {
    listen 443 ssl;
    server_name yourdomain.com;
    
    # SSL certificate paths (will be added by certbot)
    
    # API endpoints
    location /api/ {
        proxy_pass http://localhost:9001;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
    
    # WebSocket endpoint
    location / {
        proxy_pass http://localhost:9000;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
    }
}
```

Enable the site and restart NGINX:
```bash
ln -s /etc/nginx/sites-available/ocpp-server /etc/nginx/sites-enabled/
nginx -t
systemctl restart nginx
```

## Step 5: Configure DNS and SSL

1. Create an A record for your domain pointing to your Droplet's IP address
2. Once DNS has propagated (may take a few minutes to hours), obtain an SSL certificate:
   ```bash
   certbot --nginx -d yourdomain.com
   ```
3. Follow the prompts to complete the SSL setup

## Step 6: Configure Firewall

Set up a basic firewall:
```bash
ufw allow 22/tcp    # SSH
ufw allow 80/tcp    # HTTP
ufw allow 443/tcp   # HTTPS
ufw enable
```

## Step 7: Verify Your Deployment

Check if your containers are running:
```bash
docker-compose ps
```

View logs:
```bash
docker-compose logs -f
```

Test your API endpoint:
```bash
curl https://yourdomain.com/api/status
```

## Using Your OCPP Server

### WebSocket Connection Format

For OCPP clients to connect to your server, they should use:

```
wss://yourdomain.com/{chargePointID}
```

Replace `{chargePointID}` with the actual charge point identifier.

### API Endpoints

Access the API at:
```
https://yourdomain.com/api/status
https://yourdomain.com/api/charge-points
https://yourdomain.com/api/transactions
```

## Maintenance

### Updating the Server

To update your OCPP server:

```bash
cd ~/go-ocpp-server
git pull
docker-compose down
docker-compose up -d --build
```

### Viewing Logs

```bash
# OCPP server logs
docker-compose logs -f

# NGINX logs
tail -f /var/log/nginx/access.log
tail -f /var/log/nginx/error.log
```

### Database Management

Access your database from the DigitalOcean dashboard or connect using:

```bash
apt install -y postgresql-client
psql "sslmode=require host=your-db-host port=25060 user=doadmin password=your-password dbname=defaultdb"
```

## Troubleshooting

### Connection Issues

If clients can't connect:
1. Ensure ports 9000 and 9001 are not blocked internally
2. Check NGINX configuration
3. Verify SSL certificates are valid
4. Ensure the database is accessible from the Droplet

### Database Connection

If the server can't connect to the database:
1. Verify database credentials
2. Check if the Droplet's IP is in the database's allowed hosts
3. Confirm SSL mode is set to "require"

### Container Issues

If the container won't start:
```bash
docker-compose logs ocpp-server
```

## Performance Optimization

For better performance:
1. Increase your Droplet's resources if needed
2. Configure NGINX caching for API responses
3. Enable connection pooling in the application
4. Use a CDN for static content if applicable