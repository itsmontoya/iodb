package iodb

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

var exampleBucket *Bucket

func TestNewBucket(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T) (dir string, key string)
		wantErr   error
		expectErr bool
	}{
		{
			name: "returns error when getBucketsAndFiles fails for missing directory",
			setup: func(t *testing.T) (dir string, key string) {
				t.Helper()
				return filepath.Join(t.TempDir(), "missing-parent"), "missing-bucket"
			},
			wantErr: os.ErrNotExist,
		},
		{
			name: "returns error when child directory cannot be opened during populate iteration",
			setup: func(t *testing.T) (dir string, key string) {
				var (
					parentDir = t.TempDir()
					rootDir   = filepath.Join(parentDir, "root")
					childDir  = filepath.Join(rootDir, "child")
					err       error
				)

				t.Helper()
				if runtime.GOOS == "windows" {
					t.Skip("directory permission semantics differ on windows")
				}

				if err = os.Mkdir(rootDir, 0o755); err != nil {
					t.Fatalf("create root dir %q: %v", rootDir, err)
				}
				if err = os.Mkdir(childDir, 0o755); err != nil {
					t.Fatalf("create child dir %q: %v", childDir, err)
				}
				if err = os.Chmod(childDir, 0o000); err != nil {
					t.Fatalf("chmod child dir %q: %v", childDir, err)
				}

				t.Cleanup(func() {
					_ = os.Chmod(childDir, 0o755)
				})

				return parentDir, "root"
			},
			expectErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var (
				dir string
				key string
				out *Bucket
				err error
			)

			dir, key = test.setup(t)
			out, err = newBucket(dir, key)

			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("newBucket() error = %v, want %v", err, test.wantErr)
				}
				if out != nil {
					t.Fatalf("newBucket() bucket = %#v, want nil", out)
				}
				return
			}

			if test.expectErr {
				if err == nil {
					t.Fatal("newBucket() error = nil, want non-nil error")
				}
				return
			}

			if err != nil {
				t.Fatalf("newBucket() error = %v", err)
			}
		})
	}
}

func TestNewBucketRemovesTempFilesDuringPopulate(t *testing.T) {
	var (
		parentDir = t.TempDir()
		rootDir   = filepath.Join(parentDir, "root")
		tempName  = ".tmp_transient"
		tempPath  = filepath.Join(rootDir, tempName)
		out       *Bucket
		ok        bool
		err       error
	)

	if err = os.Mkdir(rootDir, 0o755); err != nil {
		t.Fatalf("create root dir %q: %v", rootDir, err)
	}

	if err = os.WriteFile(tempPath, []byte("temp"), 0o644); err != nil {
		t.Fatalf("create temp file %q: %v", tempPath, err)
	}

	if out, err = newBucket(parentDir, "root"); err != nil {
		t.Fatalf("newBucket(%q, %q) error = %v", parentDir, "root", err)
	}

	if _, err = os.Stat(tempPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temp file still exists or unexpected stat error, got: %v", err)
	}

	if _, ok = out.Get(tempName); ok {
		t.Fatalf("Get(%q) returned ok=true, want false", tempName)
	}
}

func TestBucketGetBucket(t *testing.T) {
	tests := []struct {
		name       string
		initBucket func(t *testing.T) *Bucket
		key        string
		wantKey    string
		wantOK     bool
	}{
		{
			name: "returns existing child bucket",
			initBucket: func(t *testing.T) *Bucket {
				return newTestBucket(t, "root", withChildBuckets("child"))
			},
			key:     "child",
			wantKey: "child",
			wantOK:  true,
		},
		{
			name: "returns not found for missing child bucket",
			initBucket: func(t *testing.T) *Bucket {
				return newTestBucket(t, "root")
			},
			key:    "missing",
			wantOK: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			b := test.initBucket(t)
			got, ok := b.GetBucket(test.key)
			if ok != test.wantOK {
				t.Fatalf("GetBucket() ok = %v, want %v", ok, test.wantOK)
			}

			if !test.wantOK {
				if got != nil {
					t.Fatalf("GetBucket() bucket = %#v, want nil", got)
				}
				return
			}

			if got == nil {
				t.Fatal("GetBucket() bucket = nil, want non-nil")
			}
			if got.Key() != test.wantKey {
				t.Fatalf("GetBucket() key = %q, want %q", got.Key(), test.wantKey)
			}
		})
	}
}

func TestBucketCreateBucket(t *testing.T) {
	tests := []struct {
		name       string
		initBucket func(t *testing.T) *Bucket
		key        string
		wantErr    error
		expectErr  bool
		assert     func(t *testing.T, b *Bucket, out *Bucket)
	}{
		{
			name: "creates child bucket and indexes it",
			initBucket: func(t *testing.T) *Bucket {
				return newTestBucket(t, "root")
			},
			key: "child",
			assert: func(t *testing.T, b *Bucket, out *Bucket) {
				t.Helper()
				if out == nil {
					t.Fatal("CreateBucket() returned nil bucket")
				}
				if out.Key() != "child" {
					t.Fatalf("CreateBucket() key = %q, want %q", out.Key(), "child")
				}

				fullpath := filepath.Join(b.dir, "child")
				info, err := os.Stat(fullpath)
				if err != nil {
					t.Fatalf("stat created bucket path %q: %v", fullpath, err)
				}
				if !info.IsDir() {
					t.Fatalf("created path %q is not a directory", fullpath)
				}

				got, ok := b.GetBucket("child")
				if !ok {
					t.Fatal("GetBucket() did not return created bucket")
				}
				if got != out {
					t.Fatal("GetBucket() did not return same bucket pointer")
				}
			},
		},
		{
			name: "returns existing bucket when key already exists",
			initBucket: func(t *testing.T) *Bucket {
				return newTestBucket(t, "root", withChildBuckets("child"))
			},
			key: "child",
			assert: func(t *testing.T, b *Bucket, out *Bucket) {
				t.Helper()
				got, ok := b.GetBucket("child")
				if !ok {
					t.Fatal("expected existing child bucket")
				}
				if out != got {
					t.Fatal("CreateBucket() returned different pointer for existing key")
				}
			},
		},
		{
			name: "returns error for empty key",
			initBucket: func(t *testing.T) *Bucket {
				return newTestBucket(t, "root")
			},
			key:     "",
			wantErr: ErrEmptyKey,
		},
		{
			name: "returns error for invalid key format",
			initBucket: func(t *testing.T) *Bucket {
				return newTestBucket(t, "root")
			},
			key:     "bad/key",
			wantErr: ErrInvalidKeyFormat,
		},
		{
			name: "returns error when target path already exists as file",
			initBucket: func(t *testing.T) *Bucket {
				var (
					parentDir = t.TempDir()
					bucketDir = filepath.Join(parentDir, "root")
					err       error
					out       *Bucket
				)

				if err = os.Mkdir(bucketDir, 0o755); err != nil {
					t.Fatalf("create bucket dir %q: %v", bucketDir, err)
				}

				if err = os.WriteFile(filepath.Join(parentDir, "taken"), []byte("x"), 0o644); err != nil {
					t.Fatalf("create conflicting file: %v", err)
				}

				if out, err = newBucket(parentDir, "root"); err != nil {
					t.Fatalf("newBucket(%q, %q) error = %v", parentDir, "root", err)
				}

				return out
			},
			key:       "taken",
			expectErr: true,
			assert: func(t *testing.T, b *Bucket, out *Bucket) {
				t.Helper()
				if out != nil {
					t.Fatal("CreateBucket() bucket = non-nil, want nil on mkdir error")
				}
				if _, ok := b.GetBucket("taken"); ok {
					t.Fatal("bucket tree unexpectedly contains failed create key")
				}
			},
		},
		{
			name: "returns error when child directory exists but cannot be read for newBucket",
			initBucket: func(t *testing.T) *Bucket {
				var (
					b        *Bucket
					err      error
					fullpath string
				)

				if runtime.GOOS == "windows" {
					t.Skip("directory permission semantics differ on windows")
				}

				b = newTestBucket(t, "root")
				fullpath = filepath.Join(b.dir, "child")
				if err = os.Mkdir(fullpath, 0o755); err != nil {
					t.Fatalf("create child bucket path %q: %v", fullpath, err)
				}

				if err = os.Chmod(fullpath, 0o000); err != nil {
					t.Fatalf("chmod child bucket path %q: %v", fullpath, err)
				}

				t.Cleanup(func() {
					_ = os.Chmod(fullpath, 0o755)
				})

				return b
			},
			key:       "child",
			expectErr: true,
			assert: func(t *testing.T, b *Bucket, out *Bucket) {
				t.Helper()
				if out != nil {
					t.Fatal("CreateBucket() bucket = non-nil, want nil when newBucket fails")
				}
				if _, ok := b.GetBucket("child"); ok {
					t.Fatal("bucket tree unexpectedly contains key from failed newBucket call")
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var (
				b   = test.initBucket(t)
				out *Bucket
				err error
			)

			out, err = b.CreateBucket(test.key)

			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("CreateBucket() error = %v, want %v", err, test.wantErr)
				}
				return
			}

			if test.expectErr {
				if err == nil {
					t.Fatal("CreateBucket() error = nil, want non-nil error")
				}
			} else if err != nil {
				t.Fatalf("CreateBucket() error = %v", err)
			}

			if test.assert != nil {
				test.assert(t, b, out)
			}
		})
	}
}

func TestBucketGetOrCreateBucket(t *testing.T) {
	tests := []struct {
		name       string
		initBucket func(t *testing.T) *Bucket
		key        string
		wantErr    error
		assert     func(t *testing.T, b *Bucket, out *Bucket)
	}{
		{
			name: "returns existing child bucket",
			initBucket: func(t *testing.T) *Bucket {
				return newTestBucket(t, "root", withChildBuckets("child"))
			},
			key: "child",
			assert: func(t *testing.T, b *Bucket, out *Bucket) {
				t.Helper()
				got, ok := b.GetBucket("child")
				if !ok || got != out {
					t.Fatal("GetOrCreateBucket() did not return existing bucket")
				}
			},
		},
		{
			name: "creates child bucket when missing",
			initBucket: func(t *testing.T) *Bucket {
				return newTestBucket(t, "root")
			},
			key: "new-child",
			assert: func(t *testing.T, b *Bucket, out *Bucket) {
				t.Helper()
				if out == nil {
					t.Fatal("GetOrCreateBucket() returned nil bucket")
				}
				if out.Key() != "new-child" {
					t.Fatalf("GetOrCreateBucket() key = %q, want %q", out.Key(), "new-child")
				}
			},
		},
		{
			name: "returns validation error",
			initBucket: func(t *testing.T) *Bucket {
				return newTestBucket(t, "root")
			},
			key:     "bad/key",
			wantErr: ErrInvalidKeyFormat,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			b := test.initBucket(t)
			out, err := b.GetOrCreateBucket(test.key)

			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("GetOrCreateBucket() error = %v, want %v", err, test.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("GetOrCreateBucket() error = %v", err)
			}

			if test.assert != nil {
				test.assert(t, b, out)
			}
		})
	}
}

func TestBucketGet(t *testing.T) {
	tests := []struct {
		name       string
		initBucket func(t *testing.T) *Bucket
		key        string
		wantKey    string
		wantOK     bool
	}{
		{
			name: "returns existing file",
			initBucket: func(t *testing.T) *Bucket {
				return newTestBucket(t, "root", withFiles(map[string]string{"one.txt": "one"}))
			},
			key:     "one.txt",
			wantKey: "one.txt",
			wantOK:  true,
		},
		{
			name: "returns not found for missing file",
			initBucket: func(t *testing.T) *Bucket {
				return newTestBucket(t, "root")
			},
			key:    "missing.txt",
			wantOK: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			b := test.initBucket(t)
			got, ok := b.Get(test.key)
			if ok != test.wantOK {
				t.Fatalf("Get() ok = %v, want %v", ok, test.wantOK)
			}

			if !test.wantOK {
				if got != nil {
					t.Fatalf("Get() file = %#v, want nil", got)
				}
				return
			}

			if got == nil {
				t.Fatal("Get() file = nil, want non-nil")
			}
			if got.Key() != test.wantKey {
				t.Fatalf("Get() key = %q, want %q", got.Key(), test.wantKey)
			}
		})
	}
}

func TestBucketCreate(t *testing.T) {
	tests := []struct {
		name       string
		initBucket func(t *testing.T) *Bucket
		key        string
		wantErr    error
		expectErr  bool
		assert     func(t *testing.T, b *Bucket, out *File)
	}{
		{
			name: "creates file and indexes it",
			initBucket: func(t *testing.T) *Bucket {
				return newTestBucket(t, "root")
			},
			key: "created.txt",
			assert: func(t *testing.T, b *Bucket, out *File) {
				t.Helper()
				if out == nil {
					t.Fatal("Create() returned nil file")
				}
				if out.Key() != "created.txt" {
					t.Fatalf("Create() key = %q, want %q", out.Key(), "created.txt")
				}

				fullpath := filepath.Join(b.dir, "created.txt")
				if _, err := os.Stat(fullpath); err != nil {
					t.Fatalf("stat created file %q: %v", fullpath, err)
				}

				got, ok := b.Get("created.txt")
				if !ok {
					t.Fatal("Get() did not return created file")
				}
				if got != out {
					t.Fatal("Get() did not return same file pointer")
				}
			},
		},
		{
			name: "returns existing file when key already exists",
			initBucket: func(t *testing.T) *Bucket {
				return newTestBucket(t, "root", withFiles(map[string]string{"existing.txt": "x"}))
			},
			key: "existing.txt",
			assert: func(t *testing.T, b *Bucket, out *File) {
				t.Helper()
				got, ok := b.Get("existing.txt")
				if !ok {
					t.Fatal("expected existing file")
				}
				if out != got {
					t.Fatal("Create() returned different pointer for existing key")
				}
			},
		},
		{
			name: "returns error for empty key",
			initBucket: func(t *testing.T) *Bucket {
				return newTestBucket(t, "root")
			},
			key:     "",
			wantErr: ErrEmptyKey,
		},
		{
			name: "returns error for invalid key format",
			initBucket: func(t *testing.T) *Bucket {
				return newTestBucket(t, "root")
			},
			key:     "bad/key",
			wantErr: ErrInvalidKeyFormat,
		},
		{
			name: "returns error when touchFile fails because target path is a directory",
			initBucket: func(t *testing.T) *Bucket {
				var (
					parentDir = t.TempDir()
					bucketDir = filepath.Join(parentDir, "root")
					err       error
					out       *Bucket
				)

				if err = os.Mkdir(bucketDir, 0o755); err != nil {
					t.Fatalf("create bucket dir %q: %v", bucketDir, err)
				}

				if err = os.Mkdir(filepath.Join(parentDir, "taken.txt"), 0o755); err != nil {
					t.Fatalf("create conflicting directory for touchFile: %v", err)
				}

				if out, err = newBucket(parentDir, "root"); err != nil {
					t.Fatalf("newBucket(%q, %q) error = %v", parentDir, "root", err)
				}

				return out
			},
			key:       "taken.txt",
			expectErr: true,
			assert: func(t *testing.T, b *Bucket, out *File) {
				t.Helper()
				if out != nil {
					t.Fatal("Create() file = non-nil, want nil when touchFile fails")
				}
				if _, ok := b.Get("taken.txt"); ok {
					t.Fatal("file index unexpectedly contains key from failed create")
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var (
				b   = test.initBucket(t)
				out *File
				err error
			)

			out, err = b.Create(test.key)

			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("Create() error = %v, want %v", err, test.wantErr)
				}
				return
			}

			if test.expectErr {
				if err == nil {
					t.Fatal("Create() error = nil, want non-nil error")
				}
			} else if err != nil {
				t.Fatalf("Create() error = %v", err)
			}

			if test.assert != nil {
				test.assert(t, b, out)
			}
		})
	}
}

func TestBucketGetOrCreate(t *testing.T) {
	tests := []struct {
		name       string
		initBucket func(t *testing.T) *Bucket
		key        string
		wantErr    error
		assert     func(t *testing.T, b *Bucket, out *File)
	}{
		{
			name: "returns existing file",
			initBucket: func(t *testing.T) *Bucket {
				return newTestBucket(t, "root", withFiles(map[string]string{"existing.txt": "x"}))
			},
			key: "existing.txt",
			assert: func(t *testing.T, b *Bucket, out *File) {
				t.Helper()
				got, ok := b.Get("existing.txt")
				if !ok || got != out {
					t.Fatal("GetOrCreate() did not return existing file")
				}
			},
		},
		{
			name: "creates file when missing",
			initBucket: func(t *testing.T) *Bucket {
				return newTestBucket(t, "root")
			},
			key: "new.txt",
			assert: func(t *testing.T, b *Bucket, out *File) {
				t.Helper()
				if out == nil {
					t.Fatal("GetOrCreate() returned nil file")
				}
				if out.Key() != "new.txt" {
					t.Fatalf("GetOrCreate() key = %q, want %q", out.Key(), "new.txt")
				}
			},
		},
		{
			name: "returns validation error",
			initBucket: func(t *testing.T) *Bucket {
				return newTestBucket(t, "root")
			},
			key:     "bad/key",
			wantErr: ErrInvalidKeyFormat,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			b := test.initBucket(t)
			out, err := b.GetOrCreate(test.key)

			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("GetOrCreate() error = %v, want %v", err, test.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("GetOrCreate() error = %v", err)
			}

			if test.assert != nil {
				test.assert(t, b, out)
			}
		})
	}
}

func newTestBucket(t *testing.T, name string, opts ...func(t *testing.T, bucketDir string)) (out *Bucket) {
	var (
		parentDir = t.TempDir()
		bucketDir = filepath.Join(parentDir, name)
		err       error
	)

	t.Helper()

	if err = os.Mkdir(bucketDir, 0o755); err != nil {
		t.Fatalf("create bucket dir %q: %v", bucketDir, err)
	}

	for _, opt := range opts {
		opt(t, bucketDir)
	}

	if out, err = newBucket(parentDir, name); err != nil {
		t.Fatalf("newBucket(%q, %q) error = %v", parentDir, name, err)
	}

	return out
}

func withChildBuckets(keys ...string) func(t *testing.T, bucketDir string) {
	return func(t *testing.T, bucketDir string) {
		var err error
		t.Helper()
		for _, key := range keys {
			if err = os.Mkdir(filepath.Join(bucketDir, key), 0o755); err != nil {
				t.Fatalf("create child bucket %q: %v", key, err)
			}
		}
	}
}

func withFiles(entries map[string]string) func(t *testing.T, bucketDir string) {
	return func(t *testing.T, bucketDir string) {
		var err error
		t.Helper()
		for key, contents := range entries {
			if err = os.WriteFile(filepath.Join(bucketDir, key), []byte(contents), 0o644); err != nil {
				t.Fatalf("create bucket file %q: %v", key, err)
			}
		}
	}
}

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
