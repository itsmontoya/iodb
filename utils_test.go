package iodb

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestTempFile(t *testing.T) {
	tests := []struct {
		name         string
		originalPath string
		wantErr      error
	}{
		{
			name:         "returns error when create temp fails",
			originalPath: filepath.Join(t.TempDir(), "missing", "target.txt"),
			wantErr:      os.ErrNotExist,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var (
				tempPath string
				err      error
				called   bool
			)

			tempPath, err = tempFile(test.originalPath, func(w io.Writer) error {
				called = true
				return nil
			})

			if !errors.Is(err, test.wantErr) {
				t.Fatalf("tempFile() error = %v, want %v", err, test.wantErr)
			}

			if tempPath != "" {
				t.Fatalf("tempFile() tempPath = %q, want empty path", tempPath)
			}

			if called {
				t.Fatal("tempFile() callback was called, want callback not called when CreateTemp fails")
			}
		})
	}
}

func TestValidateKey(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr error
	}{
		{
			name:    "accepts rfc3339 style log filename",
			key:     "2026-03-30T21:53:39-07:00.log",
			wantErr: nil,
		},
		{
			name:    "rejects empty key",
			key:     "",
			wantErr: ErrEmptyKey,
		},
		{
			name:    "rejects path separator",
			key:     "bad/key.log",
			wantErr: ErrInvalidKeyFormat,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateKey(test.key)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("validateKey() error = %v, want %v", err, test.wantErr)
			}
		})
	}
}
