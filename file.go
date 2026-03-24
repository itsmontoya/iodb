package iodb

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/itsmontoya/streambuf"
)

func newFile(dir string, e os.DirEntry) (out *File) {
	var f File
	f.dir = dir
	f.key = e.Name()
	return &f
}

// File represents a single persisted value in the database.
type File struct {
	entry

	// Transaction mutex, used to guard stream initialization and writes
	tmux sync.Mutex

	b *streambuf.Buffer
}

// Read opens a reader for the file and passes it to fn.
// The reader is closed after fn returns.
func (f *File) Read(fn func(io.Reader) error) (err error) {
	var b *streambuf.Buffer
	if b, err = f.getBuffer(); err != nil {
		return
	}

	var rc io.ReadCloser
	if rc, err = b.Reader(); err != nil {
		return
	}
	defer rc.Close()
	return fn(rc)
}

// Update writes file contents through fn and atomically replaces the file.
func (f *File) Update(fn func(io.Writer) error) (err error) {
	var tempFilepath string
	if tempFilepath, err = tempFile(f.filepath(), fn); err != nil {
		return err
	}

	if err = f.updateFromTemp(tempFilepath); err != nil {
		return err
	}

	return nil
}

func (f *File) Append(fn func(io.Writer) error) (err error) {
	var b *streambuf.Buffer
	if b, err = f.getBuffer(); err != nil {
		return err
	}

	if err = fn(b); err != nil {
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

	if f.b != nil {
		go f.b.CloseAndWait(context.Background())
		f.b = nil
	}

	return nil
}

func (f *File) getBuffer() (b *streambuf.Buffer, err error) {
	f.tmux.Lock()
	defer f.tmux.Unlock()
	if f.b != nil {
		return f.b, nil
	}

	f.b, err = streambuf.New(f.filepath())
	return f.b, err
}

func (f *File) filepath() (out string) {
	return filepath.Join(f.dir, f.key)
}
