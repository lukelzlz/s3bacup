# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

S3Backup is a Go-based streaming backup tool that archives and uploads to S3-compatible storage in a single pipeline. Key features: streaming tar.gz with optional AES-256-CTR encryption, multipart upload with configurable concurrency, multi-cloud support (AWS S3, Qiniu Kodo, Aliyun OSS), and storage class configuration.

## Development Commands

```bash
# Build
go build -o s3backup cmd/s3backup/main.go

# Run tests
go test ./...

# Run specific package tests
go test ./pkg/storage/...

# Cross-platform build (for releases)
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o s3backup-linux-amd64 cmd/s3backup/main.go
GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o s3backup-darwin-arm64 cmd/s3backup/main.go

# Run the binary
./s3backup backup --help

# Run with progress disabled (for scripting)
./s3backup backup --no-progress /path/to/backup
```

## Architecture

### Streaming Pipeline

The core architecture uses `io.Pipe` to connect archiving with uploading, eliminating temporary files:

```
FileSystem → tar.Writer → gzip.Writer → EncryptWriter → io.PipeWriter
                                                                    ↓
                                                            io.PipeReader
                                                                    ↓
                                                            chunk buffer → concurrent upload → S3
```

Memory usage is approximately `concurrency × chunk_size` (default: 4 × 5MB = 20MB).

### Module Structure

- `internal/cli/`: Cobra CLI commands (root.go, backup.go)
- `pkg/config/`: Configuration loading with priority: CLI args > env vars > .env file > YAML file > defaults
- `pkg/storage/`: StorageAdapter interface with implementations for AWS, Qiniu, Aliyun
- `pkg/crypto/`: AES-256-CTR + HMAC-SHA512 streaming encryption
- `pkg/archive/`: tar.gz archiver with glob-based exclusions
- `pkg/uploader/`: Multipart upload manager with worker pool pattern
- `pkg/progress/`: Progress reporting interface with Bar and Silent implementations

### Storage Adapter Interface

All storage providers implement `pkg/storage/adapter.go`:

```go
type StorageAdapter interface {
    InitMultipartUpload(ctx, key, opts) (uploadID, error)
    UploadPart(ctx, key, uploadID, partNum, data, size) (etag, error)
    CompleteMultipartUpload(ctx, key, uploadID, parts) error
    AbortMultipartUpload(ctx, key, uploadID) error
    SupportedStorageClasses() []StorageClass
    SetStorageClass(ctx, key, class) error
}
```

### Storage Class Mapping

| Generic         | AWS S3        | Qiniu Kodo | Aliyun OSS    |
|-----------------|---------------|------------|---------------|
| standard        | STANDARD      | 0          | Standard      |
| ia              | STANDARD_IA   | 1          | IA            |
| archive         | GLACIER       | 2          | Archive       |
| deep_archive    | DEEP_ARCHIVE  | 3          | ColdArchive   |

### Encryption Format

Encrypted files: `[4 bytes "S3BE"][16 bytes IV][encrypted data][8 bytes length][64 bytes HMAC]`

Key derivation uses Argon2id from password or direct read from key file.

## Configuration Priority

1. Command-line flags (`--access-key`, `--secret-key`, etc.)
2. Environment variables (`S3BACKUP_*`)
3. `.s3backup.env` file (current dir or `~/.s3backup.env`)
4. `~/.s3backup.yaml` or `--config` specified file
5. Code defaults

## Important Implementation Details

- **Endpoint normalization**: The `normalizeEndpoint()` function in `pkg/storage/adapter.go` auto-adds `https://` prefix if missing for backward compatibility.
- **Chunk size minimum**: S3 Multipart Upload requires minimum 5MB chunks (enforced in config validation).
- **Glob pattern matching**: Uses `github.com/gobwas/glob` for exclude patterns (e.g., `".git/**"`, `"*.log"`).
- **Buffer pool**: `pkg/uploader/uploader.go` uses `sync.Pool` for 5MB buffers to reduce GC pressure.
- **Context cancellation**: All operations respect context cancellation for graceful shutdown (24-hour timeout default).
- **Concurrent upload race condition**: The uploader uses `readDone` channel and separate result/error channels to coordinate between reader goroutine and upload workers, preventing race conditions when collecting results.

## Progress Display

The uploader now supports progress reporting via `pkg/progress/Reporter` interface:
- `progress.NewBar()`: Terminal progress bar with upload speed (writes to stderr)
- `progress.NewSilent()`: No output (for scripts/logs)
- `--no-progress` flag disables the progress bar

Progress updates come from concurrent upload workers using `atomic.Int64` for thread-safety.

## Known Limitations

- Backup-only (restore command planned but not implemented)
- No incremental backup (always full backup)
- No resume for interrupted uploads
- Progress bar shows "unknown total" mode because tar.gz stream size is not known upfront

## Testing

Current test coverage:
- `pkg/storage/adapter_test.go`: `normalizeEndpoint()` variations
- `pkg/progress/bar_test.go`: Progress bar functionality

When adding new features:
- Test `normalizeEndpoint()` variations for endpoint handling
- Verify storage class mapping for each provider
- Test chunk size validation (must be >= 5MB)
