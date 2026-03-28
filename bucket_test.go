package iodb

import (
	"errors"
	"fmt"
	"io"
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

				fullpath := filepath.Join(b.filepath(), "child")
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

				if err = os.WriteFile(filepath.Join(bucketDir, "taken"), []byte("x"), 0o644); err != nil {
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
				fullpath = filepath.Join(b.filepath(), "child")
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

func TestBucketCreateBucketNestedPath(t *testing.T) {
	var (
		root     = newTestBucket(t, "root", withChildBuckets("child"))
		child    *Bucket
		created  *Bucket
		ok       bool
		wantPath string
		wrong    string
		err      error
	)

	if child, ok = root.GetBucket("child"); !ok {
		t.Fatal("GetBucket(child) ok = false, want true")
	}

	if created, err = child.CreateBucket("grand"); err != nil {
		t.Fatalf("CreateBucket(grand) error = %v", err)
	}

	wantPath = filepath.Join(root.filepath(), "child", "grand")
	if created.filepath() != wantPath {
		t.Fatalf("created bucket filepath = %q, want %q", created.filepath(), wantPath)
	}

	if _, err = os.Stat(wantPath); err != nil {
		t.Fatalf("stat nested bucket %q: %v", wantPath, err)
	}

	wrong = filepath.Join(root.filepath(), "grand")
	if _, err = os.Stat(wrong); err == nil {
		t.Fatalf("unexpected root-level bucket path exists: %q", wrong)
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stat root-level bucket %q error = %v, want %v", wrong, err, os.ErrNotExist)
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

				fullpath := filepath.Join(b.filepath(), "created.txt")
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

				if err = os.Mkdir(filepath.Join(bucketDir, "taken.txt"), 0o755); err != nil {
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

func TestBucketCreateNestedFilePath(t *testing.T) {
	var (
		root     = newTestBucket(t, "root", withChildBuckets("child"))
		child    *Bucket
		created  *File
		ok       bool
		wantPath string
		wrong    string
		err      error
	)

	if child, ok = root.GetBucket("child"); !ok {
		t.Fatal("GetBucket(child) ok = false, want true")
	}

	if created, err = child.Create("grand.txt"); err != nil {
		t.Fatalf("Create(grand.txt) error = %v", err)
	}

	wantPath = filepath.Join(root.filepath(), "child", "grand.txt")
	if created.filepath() != wantPath {
		t.Fatalf("created file filepath = %q, want %q", created.filepath(), wantPath)
	}

	if _, err = os.Stat(wantPath); err != nil {
		t.Fatalf("stat nested file %q: %v", wantPath, err)
	}

	wrong = filepath.Join(root.filepath(), "grand.txt")
	if _, err = os.Stat(wrong); err == nil {
		t.Fatalf("unexpected root-level file path exists: %q", wrong)
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stat root-level file %q error = %v, want %v", wrong, err, os.ErrNotExist)
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

func TestBucketDelete(t *testing.T) {
	tests := []struct {
		name       string
		initBucket func(t *testing.T) *Bucket
		key        string
		wantErr    error
		expectErr  bool
		assert     func(t *testing.T, b *Bucket, err error)
	}{
		{
			name: "returns validation error for empty key",
			initBucket: func(t *testing.T) *Bucket {
				return newTestBucket(t, "root")
			},
			key:     "",
			wantErr: ErrEmptyKey,
		},
		{
			name: "returns validation error for invalid key format",
			initBucket: func(t *testing.T) *Bucket {
				return newTestBucket(t, "root")
			},
			key:     "bad/key",
			wantErr: ErrInvalidKeyFormat,
		},
		{
			name: "returns nil when key does not exist",
			initBucket: func(t *testing.T) *Bucket {
				return newTestBucket(t, "root", withFiles(map[string]string{
					"existing.txt": "contents",
				}))
			},
			key: "missing.txt",
			assert: func(t *testing.T, b *Bucket, err error) {
				var (
					out *File
					ok  bool
				)

				t.Helper()
				if err != nil {
					t.Fatalf("Delete() error = %v", err)
				}

				if out, ok = b.Get("existing.txt"); !ok || out == nil {
					t.Fatal("Delete() unexpectedly mutated bucket for missing key")
				}
			},
		},
		{
			name: "deletes existing file from disk and index",
			initBucket: func(t *testing.T) *Bucket {
				var (
					b   *Bucket
					f   *File
					ok  bool
					err error
				)

				b = newTestBucket(t, "root", withFiles(map[string]string{
					"delete.txt": "contents",
				}))

				if f, ok = b.Get("delete.txt"); !ok {
					t.Fatal("Get() ok = false for seeded file")
				}

				// Ensure the stream buffer is initialized before delete so Close() is exercised.
				if err = f.Append(func(w io.Writer) error {
					_, err = w.Write([]byte("more"))
					return err
				}); err != nil {
					t.Fatalf("Append() error = %v", err)
				}

				return b
			},
			key: "delete.txt",
			assert: func(t *testing.T, b *Bucket, err error) {
				var (
					ok       bool
					fullpath string
					statErr  error
				)

				t.Helper()
				if err != nil {
					t.Fatalf("Delete() error = %v", err)
				}

				if _, ok = b.Get("delete.txt"); ok {
					t.Fatal("Get() ok = true, want false after delete")
				}

				fullpath = filepath.Join(b.filepath(), "delete.txt")
				_, statErr = os.Stat(fullpath)
				if !errors.Is(statErr, os.ErrNotExist) {
					t.Fatalf("Stat(%q) error = %v, want %v", fullpath, statErr, os.ErrNotExist)
				}
			},
		},
		{
			name: "returns error when indexed file is missing on disk",
			initBucket: func(t *testing.T) *Bucket {
				var (
					b        *Bucket
					fullpath string
					err      error
				)

				b = newTestBucket(t, "root", withFiles(map[string]string{
					"missing-on-disk.txt": "contents",
				}))
				fullpath = filepath.Join(b.filepath(), "missing-on-disk.txt")
				if err = os.Remove(fullpath); err != nil {
					t.Fatalf("remove seeded file %q: %v", fullpath, err)
				}

				return b
			},
			key:       "missing-on-disk.txt",
			wantErr:   nil,
			expectErr: false,
			assert: func(t *testing.T, b *Bucket, err error) {
				var (
					out *File
					ok  bool
				)

				t.Helper()

				if out, ok = b.Get("missing-on-disk.txt"); ok || out != nil {
					t.Fatal("Delete() unexpectedly found in-memory file index on remove")
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var (
				b   = test.initBucket(t)
				err error
			)

			err = b.Delete(test.key)

			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("Delete() error = %v, want %v", err, test.wantErr)
				}
			} else if test.expectErr && err == nil {
				t.Fatal("Delete() error = nil, want non-nil error")
			} else if !test.expectErr && err != nil {
				t.Fatalf("Delete() error = %v", err)
			}

			if test.assert != nil {
				test.assert(t, b, err)
			}
		})
	}
}

func TestBucketCursor(t *testing.T) {
	var errCursorSentinel = errors.New("cursor callback error")

	tests := []struct {
		name       string
		initBucket func(t *testing.T) *Bucket
		wantErr    error
		assert     func(t *testing.T, b *Bucket, err error)
	}{
		{
			name: "iterates files in key order through cursor",
			initBucket: func(t *testing.T) *Bucket {
				return newTestBucket(t, "root", withFiles(map[string]string{
					"b.txt": "b",
					"a.txt": "a",
					"c.txt": "c",
				}))
			},
			assert: func(t *testing.T, b *Bucket, err error) {
				var (
					keys []string
					f    *File
					ok   bool
				)

				t.Helper()
				if err != nil {
					t.Fatalf("Cursor() error = %v", err)
				}

				if err = b.Cursor(func(c *Cursor) error {
					if f, ok = c.First(); !ok {
						t.Fatal("First() ok = false, want true")
					}

					keys = append(keys, f.Key())
					for {
						if f, ok = c.Next(); !ok {
							break
						}

						keys = append(keys, f.Key())
					}

					return nil
				}); err != nil {
					t.Fatalf("Cursor() error = %v", err)
				}

				if len(keys) != 3 {
					t.Fatalf("cursor keys length = %d, want 3", len(keys))
				}

				if keys[0] != "a.txt" || keys[1] != "b.txt" || keys[2] != "c.txt" {
					t.Fatalf("cursor keys = %v, want [a.txt b.txt c.txt]", keys)
				}
			},
		},
		{
			name: "returns cursor for empty bucket",
			initBucket: func(t *testing.T) *Bucket {
				return newTestBucket(t, "root")
			},
			assert: func(t *testing.T, b *Bucket, err error) {
				var (
					f  *File
					ok bool
				)

				t.Helper()
				if err != nil {
					t.Fatalf("Cursor() error = %v", err)
				}

				if err = b.Cursor(func(c *Cursor) error {
					if f, ok = c.First(); ok {
						t.Fatalf("First() file = %#v, want nil with ok=false", f)
					}
					return nil
				}); err != nil {
					t.Fatalf("Cursor() error = %v", err)
				}
			},
		},
		{
			name: "returns callback error",
			initBucket: func(t *testing.T) *Bucket {
				return newTestBucket(t, "root", withFiles(map[string]string{"a.txt": "a"}))
			},
			wantErr: errCursorSentinel,
			assert: func(t *testing.T, b *Bucket, err error) {
				t.Helper()
				if !errors.Is(err, errCursorSentinel) {
					t.Fatalf("Cursor() error = %v, want %v", err, errCursorSentinel)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var (
				b   = test.initBucket(t)
				err error
			)

			err = b.Cursor(func(c *Cursor) error {
				if test.wantErr != nil {
					return test.wantErr
				}

				return nil
			})

			if test.assert != nil {
				test.assert(t, b, err)
			}
		})
	}
}

func TestBucketForEach(t *testing.T) {
	var errForEachSentinel = errors.New("foreach callback error")

	tests := []struct {
		name       string
		initBucket func(t *testing.T) *Bucket
		assert     func(t *testing.T, b *Bucket)
	}{
		{
			name: "iterates files in key order",
			initBucket: func(t *testing.T) *Bucket {
				return newTestBucket(t, "root", withFiles(map[string]string{
					"c.txt": "c",
					"a.txt": "a",
					"b.txt": "b",
				}))
			},
			assert: func(t *testing.T, b *Bucket) {
				var (
					keys []string
					err  error
				)

				t.Helper()
				if err = b.ForEach(func(f *File) error {
					keys = append(keys, f.Key())
					return nil
				}); err != nil {
					t.Fatalf("ForEach() error = %v", err)
				}

				if len(keys) != 3 {
					t.Fatalf("ForEach() keys length = %d, want 3", len(keys))
				}

				if keys[0] != "a.txt" || keys[1] != "b.txt" || keys[2] != "c.txt" {
					t.Fatalf("ForEach() keys = %v, want [a.txt b.txt c.txt]", keys)
				}
			},
		},
		{
			name: "stops on callback error",
			initBucket: func(t *testing.T) *Bucket {
				return newTestBucket(t, "root", withFiles(map[string]string{
					"a.txt": "a",
					"b.txt": "b",
					"c.txt": "c",
				}))
			},
			assert: func(t *testing.T, b *Bucket) {
				var (
					calls int
					err   error
				)

				t.Helper()
				err = b.ForEach(func(f *File) error {
					calls++
					return errForEachSentinel
				})
				if !errors.Is(err, errForEachSentinel) {
					t.Fatalf("ForEach() error = %v, want %v", err, errForEachSentinel)
				}
				if calls != 1 {
					t.Fatalf("ForEach() callback calls = %d, want 1", calls)
				}
			},
		},
		{
			name: "does not call callback for empty bucket",
			initBucket: func(t *testing.T) *Bucket {
				return newTestBucket(t, "root")
			},
			assert: func(t *testing.T, b *Bucket) {
				var (
					calls int
					err   error
				)

				t.Helper()
				err = b.ForEach(func(f *File) error {
					calls++
					return nil
				})
				if err != nil {
					t.Fatalf("ForEach() error = %v", err)
				}
				if calls != 0 {
					t.Fatalf("ForEach() callback calls = %d, want 0", calls)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			b := test.initBucket(t)
			test.assert(t, b)
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

func ExampleBucket_Delete() {
	var err error
	if err = exampleBucket.Delete("my_file"); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Deleted my_file")
}

func ExampleBucket_Cursor() {
	var err error
	if err = exampleBucket.Cursor(func(c *Cursor) error {
		var (
			f  *File
			ok bool
		)

		if f, ok = c.First(); !ok {
			return errors.New("empty bucket")
		}

		fmt.Println("First file", f)
		return nil
	}); err != nil {
		log.Fatal(err)
	}
}

func ExampleBucket_ForEach() {
	var err error
	if err = exampleBucket.ForEach(func(f *File) error {
		fmt.Println("File", f)
		return nil
	}); err != nil {
		log.Fatal(err)
	}
}
