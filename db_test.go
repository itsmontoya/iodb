package iodb

import (
	"log"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

var exampleDB *DB

func TestNew(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T) (dbPath string)
		expectErr bool
		assert    func(t *testing.T, dbPath string, db *DB)
	}{
		{
			name: "creates missing directory path",
			setup: func(t *testing.T) (dbPath string) {
				t.Helper()
				return filepath.Join(t.TempDir(), "missing", "db")
			},
			assert: func(t *testing.T, dbPath string, db *DB) {
				var (
					info os.FileInfo
					err  error
				)

				t.Helper()
				if info, err = os.Stat(dbPath); err != nil {
					t.Fatalf("stat dbPath %q: %v", dbPath, err)
				}

				if !info.IsDir() {
					t.Fatalf("dbPath %q is not a directory", dbPath)
				}

				if db == nil {
					t.Fatal("New() returned nil DB")
				}

				if db.Bucket == nil {
					t.Fatal("New() returned DB with nil Bucket")
				}
			},
		},
		{
			name: "loads existing files and buckets from directory",
			setup: func(t *testing.T) (dbPath string) {
				var (
					err error
				)

				t.Helper()
				dbPath = t.TempDir()

				if err = os.Mkdir(filepath.Join(dbPath, "child"), 0o755); err != nil {
					t.Fatalf("create child bucket dir: %v", err)
				}

				if err = os.WriteFile(filepath.Join(dbPath, "alpha.txt"), []byte("alpha"), 0o644); err != nil {
					t.Fatalf("create file alpha.txt: %v", err)
				}

				return dbPath
			},
			assert: func(t *testing.T, dbPath string, db *DB) {
				var (
					f  *File
					b  *Bucket
					ok bool
				)

				t.Helper()
				if db == nil {
					t.Fatal("New() returned nil DB")
				}

				if f, ok = db.Get("alpha.txt"); !ok {
					t.Fatal("Get(alpha.txt) returned ok=false, want true")
				}
				if f == nil {
					t.Fatal("Get(alpha.txt) returned nil file")
				}

				if b, ok = db.GetBucket("child"); !ok {
					t.Fatal("GetBucket(child) returned ok=false, want true")
				}
				if b == nil {
					t.Fatal("GetBucket(child) returned nil bucket")
				}
			},
		},
		{
			name: "returns error when newBucket cannot read db directory",
			setup: func(t *testing.T) (dbPath string) {
				var (
					err error
				)

				t.Helper()
				if runtime.GOOS == "windows" {
					t.Skip("directory permission semantics differ on windows")
				}

				dbPath = filepath.Join(t.TempDir(), "restricted-db")
				if err = os.MkdirAll(dbPath, 0o755); err != nil {
					t.Fatalf("create dbPath %q: %v", dbPath, err)
				}

				if err = os.Chmod(dbPath, 0o000); err != nil {
					t.Fatalf("chmod dbPath %q: %v", dbPath, err)
				}

				t.Cleanup(func() {
					_ = os.Chmod(dbPath, 0o755)
				})

				return dbPath
			},
			expectErr: true,
		},
		{
			name: "returns error when directory path cannot be created",
			setup: func(t *testing.T) (dbPath string) {
				var (
					parent = t.TempDir()
					taken  = filepath.Join(parent, "taken")
					err    error
				)

				t.Helper()
				if err = os.WriteFile(taken, []byte("x"), 0o644); err != nil {
					t.Fatalf("create blocking file %q: %v", taken, err)
				}

				return filepath.Join(taken, "db")
			},
			expectErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var (
				dbPath = test.setup(t)
				db     *DB
				err    error
			)

			db, err = New(dbPath)

			if test.expectErr {
				if err == nil {
					t.Fatal("New() error = nil, want non-nil error")
				}
				if db != nil {
					t.Fatalf("New() DB = %#v, want nil on error", db)
				}
				return
			}

			if err != nil {
				t.Fatalf("New() error = %v", err)
			}

			if test.assert != nil {
				test.assert(t, dbPath, db)
			}
		})
	}
}

func ExampleNew() {
	var err error
	if exampleDB, err = New("path/to/dir"); err != nil {
		log.Fatal(err)
	}
}
