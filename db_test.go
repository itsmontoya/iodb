package iodb

import (
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
			dbPath: "./test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, gotErr := New(tt.dbPath)
			if gotErr != nil {
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

// newTestDir recreates the repository's test fixture directory structure.
func newTestDir(t *testing.T) (root string) {
	var (
		dirNames = []string{
			"1",
			"2",
			"3",
			"4",
			"5",
		}
		fileNames = []string{
			"1.txt",
			"2.txt",
			"3.txt",
			"4.txt",
			"5.txt",
		}
		nestedFileNames = map[string][]string{
			"1": {
				"1_1.txt",
				"1_2.txt",
				"1_3.txt",
				"1_4.txt",
				"1_5.txt",
			},
			"2": {
				"2_1.txt",
				"2_2.txt",
				"2_3.txt",
				"2_4.txt",
				"2_5.txt",
			},
			"3": {
				"3_1.txt",
				"3_2.txt",
				"3_3.txt",
				"3_4.txt",
				"3_5.txt",
			},
			"4": {
				"4_1.txt",
				"4_2.txt",
				"4_3.txt",
				"4_4.txt",
				"4_5.txt",
			},
			"5": {
				"5_1.txt",
				"5_2.txt",
				"5_3.txt",
				"5_4.txt",
				"5_5.txt",
			},
		}
		err error
	)

	t.Helper()

	root = filepath.Join(t.TempDir(), "test")
	if err = os.Mkdir(root, 0o755); err != nil {
		t.Fatalf("create test root %q: %v", root, err)
	}

	for _, dirName := range dirNames {
		if err = os.Mkdir(filepath.Join(root, dirName), 0o755); err != nil {
			t.Fatalf("create test dir %q: %v", dirName, err)
		}
	}

	for _, fileName := range fileNames {
		if err = os.WriteFile(filepath.Join(root, fileName), []byte(fileName), 0o644); err != nil {
			t.Fatalf("create test file %q: %v", fileName, err)
		}
	}

	for dirName, childFileNames := range nestedFileNames {
		for _, childFileName := range childFileNames {
			if err = os.WriteFile(filepath.Join(root, dirName, childFileName), []byte(childFileName), 0o644); err != nil {
				t.Fatalf("create nested test file %q: %v", filepath.Join(dirName, childFileName), err)
			}
		}
	}

	return root
}
