# iodb

A lightweight, file-system-backed key/value store for Go.

`iodb` maps:

- Buckets to directories
- Files to values

It is useful when you want simple persistent storage without running a separate database process.

Under the hood, `iodb` uses [`streambuf`](https://github.com/itsmontoya/streambuf) for file read/write behavior.

- Independent readers with their own cursors
- Tail-style streaming reads (`StreamingRead`)
- Safe concurrent append and read coordination

## Examples
### New
```go
func ExampleNew() {
	var err error
	if exampleDB, err = New("path/to/dir"); err != nil {
		log.Fatal(err)
	}
}
```

### Bucket
#### GetBucket
```go
func ExampleBucket_GetBucket() {
	var (
		b  *Bucket
		ok bool
	)

	if b, ok = exampleBucket.GetBucket("my_bucket"); ok {
		log.Fatalf("my_bucket not found")
	}

	fmt.Println("Bucket", b)
}
```

#### CreateBucket
```go
func ExampleBucket_CreateBucket() {
	var (
		b   *Bucket
		err error
	)

	if b, err = exampleBucket.CreateBucket("my_bucket"); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Bucket", b)
}
```

#### GetOrCreateBucket
```go
func ExampleBucket_GetOrCreateBucket() {
	var (
		b   *Bucket
		err error
	)

	if b, err = exampleBucket.GetOrCreateBucket("my_bucket"); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Bucket", b)
}
```

#### Get
```go
func ExampleBucket_Get() {
	var (
		f  *File
		ok bool
	)

	if f, ok = exampleBucket.Get("my_file"); ok {
		log.Fatalf("my_file not found")
	}

	fmt.Println("File", f)
}
```

#### Create
```go
func ExampleBucket_Create() {
	var (
		f   *File
		err error
	)

	if f, err = exampleBucket.Create("my_file"); err != nil {
		log.Fatal(err)
	}

	fmt.Println("File", f)
}
```

#### GetOrCreate
```go
func ExampleBucket_GetOrCreate() {
	var (
		f   *File
		err error
	)

	if f, err = exampleBucket.GetOrCreate("my_file"); err != nil {
		log.Fatal(err)
	}

	fmt.Println("File", f)
}
```

### File
#### Read
```go
func ExampleFile_Read() {
	var err error
	if err = exampleFile.Read(func(r io.Reader) (err error) {
		return nil
	}); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Read success")
}
```

#### StreamingRead
```go
func ExampleFile_StreamingRead() {
	var err error
	if err = exampleFile.StreamingRead(context.Background(), func(r io.Reader) (err error) {
		return nil
	}); err != nil {
		log.Fatal(err)
	}

	fmt.Println("StreamingRead success")
}
```

#### Update
```go
func ExampleFile_Update() {
	var err error
	if err = exampleFile.Update(func(w io.Writer) (err error) {
		return nil
	}); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Update success")
}
```

#### Append
```go
func ExampleFile_Append() {
	var err error
	if err = exampleFile.Append(func(w io.Writer) (err error) {
		return nil
	}); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Append success")
}
```

## Data Model

- `DB` is the root bucket.
- `Bucket` represents a directory.
- `File` represents one persisted value inside a bucket.

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
go test --race
```

## License

[MIT](./LICENSE)
