package iodb

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/itsmontoya/streambuf"
)

func newFile(dir, name string) (out *File) {
	var f File
	f.dir = dir
	f.key = name
	return &f
}

// File represents a single persisted value in the database.
type File struct {
	entry

	// Transaction mutex, used to guard stream initialization and writes
	tmux sync.Mutex

	b *streambuf.Buffer
}

// Read opens a non-streaming reader for the current stream buffer and passes it
// to fn. The reader is closed after fn returns.
//
// If Read acquires a reader before or during Update, it may continue reading
// from the previous buffer.
//
// Reads that start after Update returns observe the updated file contents.
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

// StreamingRead opens a tail-style streaming reader for the current stream
// buffer and passes it to fn.
//
// The reader is closed when fn returns or when ctx is canceled, whichever
// happens first.
//
// While active, the reader can observe new data appended through Append against
// the same active buffer.
//
// Like Read, if StreamingRead acquires a reader before or during Update, it may
// continue reading from the previous buffer according to streambuf close
// semantics. It does not automatically move to the new buffer created by Update.
//
// When no bytes are available and the streaming reader or backing buffer is
// closed, streambuf returns streambuf.ErrIsClosed.
func (f *File) StreamingRead(ctx context.Context, fn func(io.Reader) error) (err error) {
	var b *streambuf.Buffer
	if b, err = f.getBuffer(); err != nil {
		return
	}

	var rc io.ReadCloser
	if rc, err = b.StreamingReader(); err != nil {
		return
	}
	defer rc.Close()

	go func() {
		<-ctx.Done()
		_ = rc.Close()
	}()

	return fn(rc)
}

// Update writes file contents through fn and atomically replaces the file.
//
// Update replaces the on-disk file via rename, syncs the parent directory for
// durability, then rotates the in-memory stream buffer.
//
// While rotation is in progress, in-flight readers on the previous buffer
// continue using that buffer. Non-streaming readers drain available bytes and
// then reach EOF-equivalent behavior, while streaming readers unblock and end
// according to streambuf close semantics. Appends using a buffer being closed
// may fail with streambuf.ErrIsClosed.
//
// On successful return from Update, newly started Read and StreamingRead calls
// observe updated contents. In-flight reads and appends may still complete
// against the previous buffer according to streambuf close semantics.
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

// Append writes to the current stream buffer via fn.
//
// During a concurrent Update, append operations may fail with
// streambuf.ErrIsClosed if they target a buffer being rotated out.
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

	if err = syncDir(f.dir); err != nil {
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
