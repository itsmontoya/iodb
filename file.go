package iodb

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/itsmontoya/streambuf"
)

func newFile(dbPath, dir string, e os.DirEntry) (out *File) {
	var f File
	f.dbPath = dbPath
	f.dir = dir
	f.key = e.Name()
	return &f
}

// File represents a single persisted value in the database.
type File struct {
	entry

	// Transaction mutex, used to guard stream initialization and writes
	tmux sync.Mutex

	s *streambuf.Stream
}

// Read opens a reader for the file and passes it to fn.
// The reader is closed after fn returns.
func (f *File) Read(fn func(io.Reader) error) (err error) {
	var s *streambuf.Stream
	if s, err = f.getStream(); err != nil {
		return
	}

	var rc io.ReadCloser
	if rc, err = s.Reader(); err != nil {
		return
	}
	defer rc.Close()
	return fn(rc)
}

// Update writes file contents through fn and atomically replaces the file.
func (f *File) Update(fn func(io.Writer) error) (err error) {
	var tempFilepath string
	if tempFilepath, err = tempFile(f.dbPath, fn); err != nil {
		return err
	}

	if err = f.updateFromTemp(tempFilepath); err != nil {
		return err
	}

	return nil
}

func (f *File) updateFromTemp(tempPath string) (err error) {
	f.tmux.Lock()
	defer f.tmux.Unlock()
	if err = os.Rename(tempPath, f.filepath()); err != nil {
		return err
	}

	if f.s != nil {
		go f.s.CloseAndWait(context.Background())
		f.s = nil
	}

	return nil
}

func (f *File) getStream() (s *streambuf.Stream, err error) {
	f.tmux.Lock()
	defer f.tmux.Unlock()
	if f.s != nil {
		return f.s, nil
	}

	f.s, err = streambuf.NewStream(filepath.Join(f.dir, f.key))
	return f.s, err
}

func (f *File) filepath() (out string) {
	return filepath.Join(f.dir, f.key)
}

// String returns the file key.
func (f *File) String() string {
	return f.key
}
