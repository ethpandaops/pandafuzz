# Storage Backend

This package provides a unified interface for storing corpus files in different storage systems:
- Local filesystem
- MinIO (S3-compatible)
- AWS S3
- Any S3-compatible storage (GCS, DigitalOcean Spaces, etc.)

## Features

- **Unified Interface**: Single `StorageBackend` interface for all storage types
- **Presigned URLs**: Direct upload/download from S3 without proxying through master
- **Multipart Uploads**: Efficient handling of large files
- **Metadata Support**: Store and retrieve custom metadata with objects
- **Health Checks**: Verify storage connectivity
- **Content-Addressed Storage**: Files stored by hash for deduplication
- **Multiple Buckets**: Separate buckets for corpus, quarantine, and backup

## Configuration

### Filesystem Backend

```yaml
storage:
  type: filesystem
  filesystem:
    base_path: ./storage/corpus
  max_file_size: 104857600  # 100MB
  enable_dedup: true
```

### MinIO Backend

```yaml
storage:
  type: minio
  minio:
    endpoint: localhost:9000
    access_key_id: pandafuzz
    secret_access_key: pandafuzz123
    corpus_bucket: corpus
    quarantine_bucket: quarantine
    backup_bucket: backup
    use_ssl: false
    use_path_style: true
    part_size: 67108864  # 64MB
    concurrency: 4
```

### AWS S3 Backend

```yaml
storage:
  type: s3
  s3:
    region: us-east-1
    corpus_bucket: pandafuzz-corpus-prod
    quarantine_bucket: pandafuzz-quarantine-prod
    backup_bucket: pandafuzz-backup-prod
    use_ssl: true
    # Credentials from IAM role or environment variables
```

## Usage

```go
import (
    "github.com/ethpandaops/pandafuzz/pkg/config"
    "github.com/ethpandaops/pandafuzz/pkg/storage/backend"
)

// Create backend from configuration
storage, err := backend.NewStorageBackend(cfg, logger)
if err != nil {
    log.Fatal(err)
}

// Store a file
err = storage.Store(ctx, "corpus/campaign1/abc123", reader, size)

// Retrieve a file
reader, err := storage.Retrieve(ctx, "corpus/campaign1/abc123")
defer reader.Close()

// Generate presigned URL for direct upload
uploadURL, err := storage.PutPresignedURL(ctx, key, 1*time.Hour)

// Generate presigned URL for direct download
downloadURL, err := storage.GetPresignedURL(ctx, key, 1*time.Hour)
```

## Storage Layout

Files are stored using content-addressed storage:
```
corpus/
├── {campaign_id}/
│   └── {hash[0:2]}/
│       └── {hash}
quarantine/
└── ...
backup/
└── ...
```

This structure:
- Provides natural sharding by hash prefix
- Enables efficient deduplication
- Scales to millions of files
- Works identically on filesystem and S3