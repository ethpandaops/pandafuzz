# PandaFuzz Corpus Migration Tool

A production-ready tool for migrating corpus files between different storage backends (filesystem, S3, MinIO).

## Features

- **Multiple Storage Backends**: Supports filesystem, S3, and MinIO storage
- **Parallel Processing**: Configurable number of parallel workers for efficient migration
- **Progress Tracking**: Real-time progress with ETA calculation
- **Error Recovery**: Automatic retry with exponential backoff for transient failures
- **Data Integrity**: Optional checksum and size verification after migration
- **Resume Support**: Can resume interrupted migrations from a specific key
- **Batch Processing**: Memory-efficient processing of large file lists
- **Signal Handling**: Graceful shutdown on interrupt signals (Ctrl+C)
- **Dry Run Mode**: Preview migration without making changes
- **Confirmation Prompts**: Safety prompts for destructive operations

## Installation

```bash
go build -o migrate-corpus ./cmd/tools/migrate-corpus
```

## Usage

```bash
./migrate-corpus [options]
```

### Options

- `-source`: Source storage config file (YAML) - **Required**
- `-dest`: Destination storage config file (YAML) - **Required**
- `-prefix`: Prefix to migrate (empty for all files)
- `-dry-run`: Perform dry run without migrating
- `-parallel`: Number of parallel workers (default: 8)
- `-delete-source`: Delete files from source after successful migration
- `-verify`: Verify checksums after migration (default: true)
- `-verify-size`: Verify file sizes after migration (default: true)
- `-retries`: Maximum retry attempts for failed transfers (default: 3)
- `-batch-size`: Number of files to process in each batch (default: 100)
- `-resume`: Resume migration from specific key
- `-yes`: Skip confirmation prompts
- `-verbose`: Enable verbose logging

### Examples

#### Dry run migration from filesystem to S3
```bash
./migrate-corpus -source fs.yaml -dest s3.yaml -dry-run
```

#### Migrate all corpus files from filesystem to MinIO
```bash
./migrate-corpus -source fs.yaml -dest minio.yaml
```

#### Migrate specific campaign with cleanup
```bash
./migrate-corpus -source fs.yaml -dest s3.yaml -prefix corpus/campaign123 -delete-source
```

#### Resume interrupted migration
```bash
./migrate-corpus -source fs.yaml -dest s3.yaml -resume corpus/file1234.bin
```

#### High-performance migration with verification disabled
```bash
./migrate-corpus -source fs.yaml -dest s3.yaml -parallel 16 -verify=false -verify-size=false
```

## Configuration Files

### Filesystem Configuration (fs.yaml)
```yaml
type: filesystem
filesystem:
  base_path: /var/pandafuzz/corpus
```

### MinIO Configuration (minio.yaml)
```yaml
type: minio
minio:
  endpoint: localhost:9000
  access_key_id: pandafuzz
  secret_access_key: pandafuzz123
  corpus_bucket: corpus
  use_ssl: false
  use_path_style: true
```

### S3 Configuration (s3.yaml)
```yaml
type: s3
s3:
  region: us-east-1
  corpus_bucket: pandafuzz-corpus-prod
  use_ssl: true
  # AWS credentials can be provided via:
  # 1. Config file (not recommended)
  # 2. Environment variables (AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY)
  # 3. IAM role (recommended for EC2)
```

## Progress Display

The tool provides real-time progress updates including:
- Current file being processed
- Progress percentage
- Estimated time to completion (ETA)
- Throughput in MB/s

Example output:
```
Progress: 1234/5000 (24.7%) - ETA: 5m32s - corpus/campaign1/file123.bin
```

## Error Handling

- **Automatic Retry**: Failed transfers are automatically retried with exponential backoff
- **Error Reporting**: Failed files are listed at the end of migration
- **Partial Success**: Migration continues even if some files fail
- **Exit Codes**: Returns non-zero exit code if any files failed

## Performance Considerations

- **Parallel Workers**: Increase `-parallel` for faster migration (be mindful of API rate limits)
- **Batch Size**: Larger `-batch-size` reduces memory overhead for very large migrations
- **Verification**: Disabling verification (`-verify=false`) improves speed but reduces safety
- **Network**: Ensure adequate bandwidth between source and destination

## Safety Features

- **Confirmation Prompts**: Destructive operations require confirmation (bypass with `-yes`)
- **Signal Handling**: Graceful shutdown allows in-progress transfers to complete
- **Idempotent**: Re-running migration skips already migrated files
- **Dry Run**: Always test with `-dry-run` first

## Troubleshooting

### Migration is slow
- Increase parallel workers: `-parallel 16`
- Disable verification: `-verify=false -verify-size=false`
- Check network bandwidth between source and destination

### Out of memory errors
- Reduce batch size: `-batch-size 50`
- Reduce parallel workers: `-parallel 4`

### Permission errors
- Ensure credentials have read access to source
- Ensure credentials have write access to destination
- For S3/MinIO, check bucket policies

### Resume not working
- Ensure resume key exactly matches a file key in source
- Resume only processes files from the resume point forward