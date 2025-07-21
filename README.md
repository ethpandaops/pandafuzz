# PandaFuzz

A minimalist, self-hosted fuzzing orchestration tool written in Go. PandaFuzz strips down complex fuzzing infrastructure to its bare essentials, providing simple bot coordination and file-based result storage without any cloud dependencies.

## Quick Start

```bash
# Clone the repository
git clone https://github.com/ethpandaops/pandafuzz.git
cd pandafuzz

# Start with Docker Compose
docker-compose up -d

# Access the web dashboard at http://localhost:8080

# Run the test script to verify AFL++ and LibFuzzer integration
./scripts/run-test-with-corpus.sh

# Or test individual fuzzers
./scripts/run-test-with-corpus.sh afl++      # Test only AFL++
./scripts/run-test-with-corpus.sh libfuzzer  # Test only LibFuzzer

```

### Docker Compose Example

```yaml
version: '3.8'

services:
  master:
    build:
      context: .
      dockerfile: Dockerfile
      target: master
    ports:
      - "8080:8080"
    volumes:
      - ./storage:/storage
      - ./configs/master-docker.yaml:/app/configs/master.yaml
    environment:
      - PANDAFUZZ_CONFIG=/app/configs/master.yaml
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost:8080/api/v1/status"]
      interval: 30s
      timeout: 10s
      retries: 3

  bot:
    build:
      context: .
      dockerfile: Dockerfile
      target: bot
    environment:
      - BOT_ID=bot-${HOSTNAME:-default}
      - MASTER_URL=http://master:8080
    depends_on:
      - master
    deploy:
      replicas: 1
```

## Examples

Check out the [examples/](examples/) directory for:
- `create-fuzzing-job.sh` - Complete example that compiles and fuzzes a vulnerable binary
- `web-ui-job-example.md` - Guide for using the web UI
- `FUZZER_CONFIGURATION.md` - Advanced fuzzer configuration

## Documentation

- [Architecture Overview](docs/architecture.md) - System design and component details
- [API Documentation](docs/api.md) - RESTful API reference
- [Configuration Guide](docs/configuration.md) - Configuration options and examples
- [Development Guide](docs/development.md) - Building and testing
- [Deployment Guide](docs/deployment.md) - Deployment options and production setup

## Contributing

PandaFuzz aims to stay minimal. Please consider whether new features align with the project's philosophy of simplicity before submitting PRs.

## License

GNU Affero General Public License v3.0 - see LICENSE.md file for details.
