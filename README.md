# PandaFuzz

A minimalist, self-hosted fuzzing orchestration tool written in Go. PandaFuzz strips down complex fuzzing infrastructure to its bare essentials, providing simple bot coordination and file-based result storage without any cloud dependencies.

## Features

### Core Capabilities (Reliable & Fault-Tolerant)
- ✅ **Persistent State Management**: SQLite/BadgerDB for complete recovery
- ✅ **Atomic Job Assignment**: Race-condition-free job distribution
- ✅ **Master-Centric Storage**: Only master writes to filesystem
- ✅ **Timeout Management**: All operations have configurable timeouts
- ✅ **Bot Failure Recovery**: Automatic detection and job reassignment
- ✅ **Crash Deduplication**: SHA256-based duplicate detection
- ✅ **Job Isolation**: Separate working directories per job
- ✅ **AFL++ & LibFuzzer**: Core fuzzing engine support
- ✅ **VPN-Only Operation**: No external authentication needed
- ✅ **Docker Deployment**: Single container with all dependencies

### Reliability Features
- ✅ **Complete State Recovery**: Resume after any component failure
- ✅ **Orphaned Job Cleanup**: Automatic reassignment of failed jobs
- ✅ **Corpus Metadata Persistence**: Never lose fuzzing progress
- ✅ **Resource Limits**: Prevent resource exhaustion
- ✅ **Input Validation**: Secure against malformed bot inputs
- ✅ **Process Isolation**: Isolated fuzzer execution

### Explicitly Excluded (By Design)
- ❌ Multi-master support (single master only)
- ❌ Cloud integrations (fully self-hosted)
- ❌ Complex authentication (VPN provides security)
- ❌ Advanced analytics or ML features
- ❌ Real-time collaboration
- ❌ External integrations (issue trackers, notifications)

## Quick Start

1. Clone the repository:
```bash
git clone https://github.com/ethpandaops/pandafuzz.git
cd pandafuzz
```

2. Start with Docker Compose:
```bash
docker-compose up -d
```

3. Access the web dashboard:
```
http://localhost:8080
```

## Documentation

- [Architecture Overview](docs/architecture.md) - System design and component details
- [Development Guide](docs/development.md) - Development status and technical roadmap
- [API Documentation](docs/api.md) - RESTful API reference
- [Campaign Features](docs/CAMPAIGN_FEATURES.md) - Campaign-based fuzzing details
- [Storage Architecture](STORAGE_ARCHITECTURE.md) - Storage layer design

## Architecture

PandaFuzz uses a reliable master-centric architecture:

1. **Master**: Single coordinator with persistent state and exclusive filesystem access
2. **Bot**: Stateless agents that communicate results via API only

### Key Design Decisions:
- **Single Master**: Eliminates coordination complexity
- **Master-Only Writes**: Prevents filesystem conflicts
- **Persistent State**: Complete recovery from any failure
- **Atomic Operations**: Race-condition-free job assignment
- **Timeout Everything**: Assume all components can fail

## Configuration

### Master Configuration (configs/master.yaml)
```yaml
server:
  port: 8080
  timeout: 30s

database:
  type: sqlite  # or badger
  path: /storage/data/pandafuzz.db

storage:
  path: /storage

timeouts:
  bot_heartbeat: 60s
  job_execution: 3600s
  master_recovery: 300s

limits:
  max_concurrent_jobs: 10
  max_corpus_size: 1GB
  max_crash_size: 10MB
```

### Bot Configuration (configs/bot.yaml)
```yaml
bot:
  id: ${BOT_ID}
  master_url: ${MASTER_URL}
  heartbeat_interval: 30s
  timeout: 30s

fuzzing:
  work_dir: /tmp/fuzzing
  capabilities: ["afl++", "libfuzzer"]
  
timeouts:
  job_execution: 3600s
  master_communication: 30s
```

## API Endpoints

### Core Operations
- `POST /api/bots/register` - Register a new bot with capabilities
- `GET /api/jobs` - List all jobs with filtering
- `POST /api/jobs` - Create a new fuzzing job with strategy
- `GET /api/crashes/{job_id}` - Get crashes for a job with deduplication

### Advanced Features
- `POST /api/mutators` - Upload custom mutator
- `POST /api/grammars` - Upload grammar definition
- `GET /api/corpus/seeds` - Get prioritized corpus seeds
- `POST /api/leaks` - Report memory leaks
- `GET /api/strategies` - List available fuzzing strategies

## Development

### Prerequisites
- Go 1.21+
- Docker and Docker Compose
- AFL++ (for local development)

### Building
```bash
go mod download
go build -o pandafuzz-master ./cmd/master
go build -o pandafuzz-bot ./cmd/bot
```

### Running Tests
```bash
go test ./...
```

## Deployment Options

### Docker (Recommended)
Use the provided Dockerfile and docker-compose.yml for easy deployment.

### Kubernetes
```bash
kubectl apply -f k8s/
```

### Systemd
See `scripts/systemd/` for service files.

## Contributing

PandaFuzz aims to stay minimal. Please consider whether new features align with the project's philosophy of simplicity before submitting PRs.

## License

MIT License - see LICENSE file for details.