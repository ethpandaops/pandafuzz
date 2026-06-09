# Docker Setup Guide

## Quick Start

```bash
# Start the master and 2 bot instances
docker-compose up -d

# Scale to more bots
docker-compose up -d --scale bot=5

# View logs
docker-compose logs -f bot
docker-compose logs -f master

# Stop everything
docker-compose down
```

## Configuration Files

### For Docker Deployment
- `bot-docker.yaml` - Bot configuration for Docker containers (uses `http://master:8080`)
- `master-docker.yaml` - Master configuration for Docker
- `docker-compose.yml` - Docker Compose orchestration

### For Local Development
- `configs/master.yaml` - Master configuration example
- `configs/bot.example.yaml` - Bot configuration example

## Why Two Bot Config Files?

The bot configuration needs different master URLs depending on the environment:
- **Docker**: Bots connect to `http://master:8080` (using Docker service name)
- **Local**: Bots connect to `http://localhost:8080`

Since Go's `os.ExpandEnv()` doesn't support shell-style default values like `${VAR:-default}`, we use separate config files for clarity.

## Troubleshooting

### Bots Not Connecting

1. Check master is healthy:
   ```bash
   docker-compose ps
   # Should show master as "healthy"
   ```

2. Check master logs:
   ```bash
   docker-compose logs master | grep -E "(Started|Error|8080)"
   ```

3. Check bot logs:
   ```bash
   docker-compose logs bot | grep -E "(Registering|Error|master_url)"
   ```

4. Verify network connectivity:
   ```bash
   # Test from bot container
   docker-compose exec bot wget -O- http://master:8080/health
   ```

### Common Issues

- **"parse error: first path segment in URL cannot contain colon"**: This means the environment variable expansion failed. Make sure you're using `bot-docker.yaml` in Docker.
- **"connection refused"**: Master might not be ready yet. Bots will retry automatically.
- **"no such host"**: Docker networking issue. Restart with `docker-compose down && docker-compose up -d`.

## Profiles

```bash
# Development mode with extra tools
docker-compose --profile development up -d

# With monitoring (Prometheus + Grafana)
docker-compose --profile monitoring up -d

# With PostgreSQL instead of SQLite
docker-compose --profile postgres up -d
```

## Scaling

```bash
# Scale bots dynamically
docker-compose up -d --scale bot=10

# Check bot status
curl -H "X-API-Key: dev-api-key" http://localhost:8080/api/v1/bots
```
