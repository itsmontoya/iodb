package iodb

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		dbPath  string
		want    *DB
		wantErr bool
	}{
		{
			name:   "loads existing db path",
			dbPath: "./test",
		},
		{
			name:    "returns error for missing db path",
			dbPath:  filepath.Join(t.TempDir(), "missing"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, gotErr := New(tt.dbPath)
			if gotErr != nil {
				if tt.wantErr && !errors.Is(gotErr, os.ErrNotExist) {
					t.Errorf("New() error = %v, want %v", gotErr, os.ErrNotExist)
				}
				if !tt.wantErr {
					t.Errorf("New() failed: %v", gotErr)
				}
				return
			}

			var (
				f  *File
				ok bool
			)
			if f, ok = db.files.Get("1.txt"); !ok {
				t.Error("file does not exist")
			}

			f.Update(func(w io.Writer) error {
				_, err := w.Write([]byte("foo bar"))
				return err
			})

			if tt.wantErr {
				t.Fatal("New() succeeded unexpectedly")
			}
		})
	}
}
