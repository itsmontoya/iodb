# iodb

A lightweight, file-system-backed key/value store for Go.

`iodb` maps:

- Buckets to directories
- Files to values

It is useful when you want simple persistent storage without running a separate database process.

Under the hood, `iodb` uses [`streambuf`](https://github.com/itsmontoya/streambuf) for file read/write behavior.

## Install

```bash
go get github.com/itsmontoya/iodb
```

## Requirements

- Go `1.25.3+` (per `go.mod`)

## Quick Start

```go
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/itsmontoya/iodb"
)

func main() {
	var (
		db  *iodb.DB
		err error
	)

	db, err = iodb.New("./data")
	if err != nil {
		log.Fatal(err)
	}

	var users *iodb.Bucket
	users, err = db.GetOrCreateBucket("users")
	if err != nil {
		log.Fatal(err)
	}

	var profile *iodb.File
	profile, err = users.GetOrCreate("alice.json")
	if err != nil {
		log.Fatal(err)
	}

	err = profile.Update(func(w io.Writer) (err error) {
		_, err = w.Write([]byte(`{"name":"alice","active":true}`))
		return err
	})
	if err != nil {
		log.Fatal(err)
	}

	err = profile.Read(func(r io.Reader) (err error) {
		var b []byte
		if b, err = io.ReadAll(r); err != nil {
			return err
		}

		fmt.Println(string(b))
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	go func() {
		_ = profile.Append(func(w io.Writer) (err error) {
			_, err = w.Write([]byte("\nupdated-at=now"))
			return err
		})
	}()

	_ = profile.StreamingRead(ctx, func(r io.Reader) (err error) {
		var b = make([]byte, 256)
		_, _ = r.Read(b)
		return nil
	})
}
```

## Data Model

- `DB` is the root bucket.
- `Bucket` represents a directory.
- `File` represents one persisted value inside a bucket.

## Why streambuf Matters

`iodb` uses `streambuf` to provide file semantics that are stronger than plain `os.File` read/write coordination.

- Append-only write behavior for active buffers
- Multiple independent readers, each with its own cursor
- File-backed reads share one underlying file descriptor, so many readers do not require one FD per reader
- Tail-style readers that can wait for future bytes (`StreamingRead`)
- Reader lifecycle and close signaling semantics (`streambuf.ErrIsClosed`)
- Predictable behavior when rotating from old data to new data during `Update`

In practice, this means you can safely mix:

- Snapshot reads (`Read`)
- Follow/tail reads (`StreamingRead`)
- Concurrent appends (`Append`)
- Atomic content replacement (`Update`)

without manually building your own multi-reader stream coordination.

## API Overview

### Database

- `New(path string) (*DB, error)`
  - Creates the root directory if missing.
  - Loads existing files and child buckets from disk.

### Bucket

- `GetBucket(key string) (*Bucket, bool)`
- `CreateBucket(key string) (*Bucket, error)`
- `GetOrCreateBucket(key string) (*Bucket, error)`
- `Get(key string) (*File, bool)`
- `Create(key string) (*File, error)`
- `GetOrCreate(key string) (*File, error)`

### File

- `Read(func(io.Reader) error) error`
  - Non-streaming read of the current buffer state.
- `StreamingRead(ctx context.Context, func(io.Reader) error) error`
  - Tail-style streaming read.
- `Update(func(io.Writer) error) error`
  - Atomic replace using temp file + rename + directory sync.
- `Append(func(io.Writer) error) error`
  - Appends to the current stream buffer.

## Key Rules

Keys are validated for file and bucket creation:

- Must not be empty
- Must match `^[A-Za-z0-9](?:[A-Za-z0-9._-]{0,254})$`
- Path separators are not allowed
- Maximum length is 255 characters

Exported validation errors:

- `ErrEmptyKey`
- `ErrInvalidKeyFormat`

## Concurrency and Update Semantics

- Bucket lookups/creates are guarded by `sync.RWMutex`.
- File updates are synchronized with a transaction mutex.
- `Update` rotates the backing stream buffer after replacing the on-disk file.
- Readers started before rotation may continue on the previous buffer.
- Appends racing with `Update` can return `streambuf.ErrIsClosed`.
- `Read` is snapshot-style and reaches EOF at current end.
- `StreamingRead` is tail-style and waits for new bytes until closed/canceled.

## Testing

```bash
go test ./...
```

## License

[MIT](./LICENSE)
