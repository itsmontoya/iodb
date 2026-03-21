package iodb

import (
	"context"
	"io"
	"os"
	"path"
	"sync"

	"github.com/itsmontoya/streambuf"
)

func newFile(dir string, e os.DirEntry) (out *File) {
	var f File
	f.dir = dir
	f.key = e.Name()
	return &f
}

type File struct {
	entry

	// Transaction mutex, only used for writes
	tmux sync.Mutex
	s    *streambuf.Stream
}

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

func (f *File) Update(fn func(io.Writer) error) (err error) {
	var tempFilepath string
	if tempFilepath, err = tempFile(fn); err != nil {
		return err
	}

	if err = f.updateFromTemp(tempFilepath); err != nil {
		return err
	}

	return nil
}

func (f *File) updateFromTemp(tempFilepath string) (err error) {
	f.tmux.Lock()
	defer f.tmux.Unlock()
	if err = os.Rename(tempFilepath, f.filepath()); err != nil {
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

	f.s, err = streambuf.NewStream(path.Join(f.dir, f.key))
	return f.s, nil
}

func (f *File) filepath() (out string) {
	return path.Join(f.dir, f.key)
}

func (f *File) String() string {
	return f.key
}
