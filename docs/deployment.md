# PandaFuzz Deployment Guide

## Deployment Options

PandaFuzz supports multiple deployment methods to fit your infrastructure needs.

## Docker (Recommended)

The easiest way to deploy PandaFuzz is using Docker Compose:

```bash
docker-compose up -d
```

This will start:
- Master server on port 8080
- PostgreSQL database
- One bot instance (can be scaled)

### Scaling Bots

To run multiple bot instances:

```bash
docker-compose up -d --scale bot=10
```

### Custom Configuration

1. Copy the example configuration:
   ```bash
   cp configs/master-docker.yaml configs/my-master.yaml
   ```

2. Mount your configuration in docker-compose.yml:
   ```yaml
   volumes:
     - ./configs/my-master.yaml:/app/configs/master.yaml
   ```

## Kubernetes

Deploy to Kubernetes using the provided manifests:

```bash
kubectl apply -f k8s/
```

This creates:
- Master deployment with service
- Bot deployment (scalable)
- ConfigMaps for configuration
- PersistentVolumeClaims for storage

### Scaling in Kubernetes

```bash
kubectl scale deployment pandafuzz-bot --replicas=20
```

### Helm Chart (Coming Soon)

A Helm chart is planned for easier Kubernetes deployments with customizable values.

## Systemd

For bare metal deployments, systemd service files are provided:

### Master Service

1. Copy the service file:
   ```bash
   sudo cp scripts/systemd/pandafuzz-master.service /etc/systemd/system/
   ```

2. Update paths in the service file

3. Enable and start:
   ```bash
   sudo systemctl enable pandafuzz-master
   sudo systemctl start pandafuzz-master
   ```

### Bot Service

1. Copy the service file:
   ```bash
   sudo cp scripts/systemd/pandafuzz-bot.service /etc/systemd/system/
   ```

2. Configure environment variables in `/etc/pandafuzz/bot.env`

3. Enable and start:
   ```bash
   sudo systemctl enable pandafuzz-bot
   sudo systemctl start pandafuzz-bot
   ```

## Production Considerations

### Storage

- Use persistent volumes for `/storage` directory
- Regular backups of SQLite database
- Consider using PostgreSQL for larger deployments

### Networking

- Deploy behind VPN or secure network
- Use reverse proxy for TLS termination
- Configure firewall rules appropriately

### Monitoring

- Enable Prometheus metrics endpoint
- Set up alerting for:
  - Master downtime
  - Bot failures
  - Storage usage
  - Job completion rates

### Resource Requirements

#### Master
- CPU: 2 cores
- Memory: 2GB
- Storage: 10GB + corpus/crash storage

#### Bot
- CPU: 1 core per bot
- Memory: 1GB per bot
- Storage: 5GB temporary space

### High Availability

PandaFuzz currently supports single-master architecture. For HA:
- Use external database (PostgreSQL)
- Regular backups
- Quick failover procedures
- Monitor master health

## Environment Variables

### Master
- `PANDAFUZZ_CONFIG`: Path to configuration file
- `PANDAFUZZ_PORT`: Override server port
- `PANDAFUZZ_STORAGE`: Override storage path

### Bot
- `BOT_ID`: Unique bot identifier
- `MASTER_URL`: Master API endpoint
- `BOT_CONFIG`: Path to bot configuration

## Security

1. **Network Security**
   - Deploy within VPN
   - Use firewalls to restrict access
   - Enable TLS for external access

2. **File Permissions**
   - Run as non-root user
   - Restrict storage directory permissions
   - Use read-only root filesystem in containers

3. **Updates**
   - Regular security updates
   - Monitor for vulnerabilities
   - Automated deployment pipeline