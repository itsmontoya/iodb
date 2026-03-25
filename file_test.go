package iodb

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/itsmontoya/streambuf"
)

func TestFileKey(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantKey string
	}{
		{
			name:    "returns entry key",
			key:     "alpha.txt",
			wantKey: "alpha.txt",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var (
				dir = t.TempDir()
				f   = newFile(dir, test.key)
			)

			if got := f.Key(); got != test.wantKey {
				t.Fatalf("Key() = %q, want %q", got, test.wantKey)
			}
		})
	}
}

func TestFileRead(t *testing.T) {
	errReadCallback := errors.New("read callback failed")

	tests := []struct {
		name      string
		initFile  func(t *testing.T) *File
		cb        func(t *testing.T, r io.Reader) error
		wantErr   error
		expectErr bool
	}{
		{
			name: "reads file contents",
			initFile: func(t *testing.T) *File {
				t.Helper()
				return newTestFile(t, "read.txt", "hello world")
			},
			cb: func(t *testing.T, r io.Reader) (err error) {
				var b []byte
				if b, err = readAllAllowWrappedEOF(r); err != nil {
					return err
				}

				if string(b) != "hello world" {
					t.Fatalf("Read() content = %q, want %q", string(b), "hello world")
				}

				return nil
			},
		},
		{
			name: "propagates callback error",
			initFile: func(t *testing.T) *File {
				t.Helper()
				return newTestFile(t, "read.txt", "hello world")
			},
			cb: func(t *testing.T, r io.Reader) (err error) {
				return errReadCallback
			},
			wantErr: errReadCallback,
		},
		{
			name: "returns empty read when file removed before first read",
			initFile: func(t *testing.T) *File {
				var f *File
				t.Helper()
				f = newTestFile(t, "read.txt", "hello world")
				if err := os.Remove(f.filepath()); err != nil {
					t.Fatalf("remove file before read: %v", err)
				}
				return f
			},
			cb: func(t *testing.T, r io.Reader) (err error) {
				var b []byte
				if b, err = readAllAllowWrappedEOF(r); err != nil {
					return err
				}

				if string(b) != "" {
					t.Fatalf("Read() content = %q, want %q", string(b), "")
				}

				return nil
			},
		},
		{
			name: "reads from initialized buffer after file removal",
			initFile: func(t *testing.T) *File {
				var f *File
				t.Helper()
				f = newTestFile(t, "read.txt", "hello world")
				if err := f.Read(func(r io.Reader) (err error) {
					_, err = readAllAllowWrappedEOF(r)
					return err
				}); err != nil {
					t.Fatalf("prime buffer via read: %v", err)
				}

				if err := os.Remove(f.filepath()); err != nil {
					t.Fatalf("remove file after priming buffer: %v", err)
				}

				return f
			},
			cb: func(t *testing.T, r io.Reader) (err error) {
				var b []byte
				if b, err = readAllAllowWrappedEOF(r); err != nil {
					return err
				}

				if string(b) != "hello world" {
					t.Fatalf("Read() content = %q, want %q", string(b), "hello world")
				}

				return nil
			},
		},
		{
			name: "returns close error when backing buffer is closed",
			initFile: func(t *testing.T) *File {
				var (
					f   *File
					b   *streambuf.Buffer
					err error
				)
				t.Helper()
				f = newTestFile(t, "read.txt", "hello world")
				if b, err = f.getBuffer(); err != nil {
					t.Fatalf("getBuffer() error = %v", err)
				}

				if err = b.CloseAndWait(context.Background()); err != nil {
					t.Fatalf("CloseAndWait() error = %v", err)
				}

				return f
			},
			cb: func(t *testing.T, r io.Reader) (err error) {
				return nil
			},
			wantErr: streambuf.ErrIsClosed,
		},
		{
			name: "returns error when getBuffer fails",
			initFile: func(t *testing.T) *File {
				var (
					dir = filepath.Join(t.TempDir(), "missing")
				)
				t.Helper()
				return newFile(dir, "read.txt")
			},
			cb: func(t *testing.T, r io.Reader) (err error) {
				t.Fatal("read callback should not be called when getBuffer fails")
				return nil
			},
			expectErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var (
				f   = test.initFile(t)
				err error
			)

			err = f.Read(func(r io.Reader) error {
				return test.cb(t, r)
			})

			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("Read() error = %v, want %v", err, test.wantErr)
				}
				return
			}

			if test.expectErr {
				if err == nil {
					t.Fatal("Read() error = nil, want non-nil error")
				}
				return
			}

			if err != nil {
				t.Fatalf("Read() error = %v", err)
			}
		})
	}
}

func TestFileStreamingRead(t *testing.T) {
	errStreamCallback := errors.New("stream callback failed")

	tests := []struct {
		name      string
		initFile  func(t *testing.T) *File
		initCtx   func() (context.Context, context.CancelFunc)
		cb        func(t *testing.T, f *File, r io.Reader, cancel context.CancelFunc) error
		wantErr   error
		expectErr bool
	}{
		{
			name: "reads appended bytes",
			initFile: func(t *testing.T) *File {
				t.Helper()
				return newTestFile(t, "stream.txt", "")
			},
			cb: func(t *testing.T, f *File, r io.Reader, cancel context.CancelFunc) (err error) {
				var (
					buf = make([]byte, len("payload"))
					n   int
				)

				if err = f.Append(func(w io.Writer) error {
					_, err = w.Write([]byte("payload"))
					return err
				}); err != nil {
					return err
				}

				if n, err = io.ReadFull(r, buf); err != nil {
					return err
				}

				if string(buf[:n]) != "payload" {
					t.Fatalf("StreamingRead() value = %q, want %q", string(buf[:n]), "payload")
				}

				return nil
			},
		},
		{
			name: "propagates callback error",
			initFile: func(t *testing.T) *File {
				t.Helper()
				return newTestFile(t, "stream.txt", "")
			},
			cb: func(t *testing.T, f *File, r io.Reader, cancel context.CancelFunc) (err error) {
				return errStreamCallback
			},
			wantErr: errStreamCallback,
		},
		{
			name: "returns close error when context is canceled while blocked",
			initFile: func(t *testing.T) *File {
				t.Helper()
				return newTestFile(t, "stream.txt", "")
			},
			initCtx: func() (context.Context, context.CancelFunc) {
				return context.WithCancel(context.Background())
			},
			cb: func(t *testing.T, f *File, r io.Reader, cancel context.CancelFunc) (err error) {
				var (
					buf = make([]byte, 1)
				)
				go cancel()
				_, err = r.Read(buf)
				return err
			},
			wantErr: streambuf.ErrIsClosed,
		},
		{
			name: "returns error when underlying buffer is already closed",
			initFile: func(t *testing.T) *File {
				var (
					f   = newTestFile(t, "stream.txt", "")
					b   *streambuf.Buffer
					err error
				)
				t.Helper()

				if b, err = streambuf.New(f.filepath()); err != nil {
					t.Fatalf("streambuf.New() error = %v", err)
				}
				if err = b.CloseAndWait(context.Background()); err != nil {
					t.Fatalf("CloseAndWait() error = %v", err)
				}

				f.b = b
				return f
			},
			cb: func(t *testing.T, f *File, r io.Reader, cancel context.CancelFunc) (err error) {
				return nil
			},
			wantErr: streambuf.ErrIsClosed,
		},
		{
			name: "returns error when getBuffer fails",
			initFile: func(t *testing.T) *File {
				var (
					dir = filepath.Join(t.TempDir(), "missing")
				)
				t.Helper()
				return newFile(dir, "stream.txt")
			},
			cb: func(t *testing.T, f *File, r io.Reader, cancel context.CancelFunc) (err error) {
				return nil
			},
			expectErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var (
				f      = test.initFile(t)
				ctx    = context.Background()
				cancel = func() {}
				err    error
			)
			if test.initCtx != nil {
				ctx, cancel = test.initCtx()
			}
			defer cancel()

			err = f.StreamingRead(ctx, func(r io.Reader) error {
				return test.cb(t, f, r, cancel)
			})

			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("StreamingRead() error = %v, want %v", err, test.wantErr)
				}
				return
			}

			if test.expectErr {
				if err == nil {
					t.Fatal("StreamingRead() error = nil, want non-nil error")
				}
				return
			}

			if err != nil {
				t.Fatalf("StreamingRead() error = %v", err)
			}
		})
	}
}

func TestFileUpdate(t *testing.T) {
	errUpdateCallback := errors.New("update callback failed")

	tests := []struct {
		name           string
		initFile       func(t *testing.T) *File
		updateCB       func(io.Writer) error
		wantErr        error
		expectErr      bool
		assertContents bool
		wantContents   string
	}{
		{
			name: "replaces file contents",
			initFile: func(t *testing.T) *File {
				t.Helper()
				return newTestFile(t, "update.txt", "before")
			},
			updateCB: func(w io.Writer) (err error) {
				_, err = w.Write([]byte("after"))
				return err
			},
			assertContents: true,
			wantContents:   "after",
		},
		{
			name: "propagates callback error and preserves existing contents",
			initFile: func(t *testing.T) *File {
				t.Helper()
				return newTestFile(t, "update.txt", "before")
			},
			updateCB: func(w io.Writer) (err error) {
				return errUpdateCallback
			},
			wantErr:        errUpdateCallback,
			assertContents: true,
			wantContents:   "before",
		},
		{
			name: "returns error when updateFromTemp fails",
			initFile: func(t *testing.T) *File {
				var (
					dir = t.TempDir()
				)
				t.Helper()
				return newFile(dir, "")
			},
			updateCB: func(w io.Writer) (err error) {
				_, err = w.Write([]byte("after"))
				return err
			},
			expectErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var (
				f   = test.initFile(t)
				err error
			)

			err = f.Update(test.updateCB)
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("Update() error = %v, want %v", err, test.wantErr)
				}
			} else if err != nil && !test.expectErr {
				t.Fatalf("Update() error = %v", err)
			}
			if test.expectErr {
				if err == nil {
					t.Fatal("Update() error = nil, want non-nil error")
				}
				return
			}

			if test.assertContents {
				if err = assertFileReadContent(f, test.wantContents); err != nil {
					t.Fatalf("Read() content assertion error = %v", err)
				}
			}
		})
	}
}

func TestFileUpdateFromTemp(t *testing.T) {
	const updatedContent = "after"

	tests := []struct {
		name            string
		init            func(t *testing.T) (fileDir, destPath string, f *File)
		wantErr         error
		assertBufferNil bool
		wantContent     string
	}{
		{
			name: "closes and clears existing buffer on success",
			init: func(t *testing.T) (fileDir, destPath string, f *File) {
				var (
					key = "update_from_temp.txt"
					b   *streambuf.Buffer
					err error
				)

				fileDir = t.TempDir()
				destPath = filepath.Join(fileDir, key)
				f = newFile(fileDir, key)

				if err = os.WriteFile(destPath, []byte("before"), 0o644); err != nil {
					t.Fatalf("write initial file: %v", err)
				}

				if b, err = streambuf.New(destPath); err != nil {
					t.Fatalf("streambuf.New() error = %v", err)
				}

				f.b = b
				return fileDir, destPath, f
			},
			assertBufferNil: true,
			wantContent:     updatedContent,
		},
		{
			name: "returns error when syncDir fails after successful rename",
			init: func(t *testing.T) (fileDir, destPath string, f *File) {
				var (
					root       = t.TempDir()
					destDir    = filepath.Join(root, "dest")
					missingDir = filepath.Join(root, "missing")
					key        = filepath.Join("..", "dest", "syncdir_fail.txt")
					err        error
				)

				fileDir = missingDir
				destPath = filepath.Join(destDir, "syncdir_fail.txt")

				if err = os.MkdirAll(destDir, 0o755); err != nil {
					t.Fatalf("create destination dir: %v", err)
				}

				f = newFile(missingDir, key)
				return fileDir, destPath, f
			},
			wantErr:     os.ErrNotExist,
			wantContent: updatedContent,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var (
				fileDir, destPath string
				f                 *File
				content           []byte
				tempPath          string
				err               error
			)

			fileDir, destPath, f = test.init(t)
			if tempPath, err = tempFile(destPath, func(w io.Writer) (err error) {
				_, err = w.Write([]byte(updatedContent))
				return err
			}); err != nil {
				t.Fatalf("tempFile() error = %v", err)
			}

			err = f.updateFromTemp(tempPath)

			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("updateFromTemp() error = %v, want %v (fileDir=%q)", err, test.wantErr, fileDir)
				}
			} else if err != nil {
				t.Fatalf("updateFromTemp() error = %v (fileDir=%q)", err, fileDir)
			}

			if test.assertBufferNil && f.b != nil {
				t.Fatal("f.b was not cleared after updateFromTemp()")
			}

			if content, err = os.ReadFile(destPath); err != nil {
				t.Fatalf("read destination file %q: %v", destPath, err)
			}
			if string(content) != test.wantContent {
				t.Fatalf("destination content = %q, want %q", string(content), test.wantContent)
			}
		})
	}
}

func TestFileAppend(t *testing.T) {
	errAppendCallback := errors.New("append callback failed")

	tests := []struct {
		name         string
		initFile     func(t *testing.T) *File
		appendCB     func(io.Writer) error
		wantErr      error
		wantContents string
	}{
		{
			name: "appends to current contents",
			initFile: func(t *testing.T) *File {
				t.Helper()
				return newTestFile(t, "append.txt", "before")
			},
			appendCB: func(w io.Writer) (err error) {
				_, err = w.Write([]byte("-after"))
				return err
			},
			wantContents: "before-after",
		},
		{
			name: "propagates callback error",
			initFile: func(t *testing.T) *File {
				t.Helper()
				return newTestFile(t, "append.txt", "before")
			},
			appendCB: func(w io.Writer) (err error) {
				return errAppendCallback
			},
			wantErr:      errAppendCallback,
			wantContents: "before",
		},
		{
			name: "returns error when getBuffer fails",
			initFile: func(t *testing.T) *File {
				var (
					dir = filepath.Join(t.TempDir(), "missing")
				)
				t.Helper()
				return newFile(dir, "append.txt")
			},
			appendCB: func(w io.Writer) (err error) {
				_, err = w.Write([]byte("-after"))
				return err
			},
			wantErr: os.ErrNotExist,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var (
				f   = test.initFile(t)
				err error
			)

			err = f.Append(test.appendCB)
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("Append() error = %v, want %v", err, test.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("Append() error = %v", err)
			}

			if err = assertFileReadContent(f, test.wantContents); err != nil {
				t.Fatalf("Read() content assertion error = %v", err)
			}
		})
	}
}

// These are behavior/contract tests (instead of strict unit tests) because they
// intentionally coordinate goroutines and depend on streambuf v0.7.0 close
// semantics under concurrent access.
func TestFileConcurrentUpdateContracts(t *testing.T) {
	t.Run("append that captured old buffer can observe ErrIsClosed after update rotation", func(t *testing.T) {
		var (
			f           = newTestFile(t, "concurrent_append.txt", "before")
			appendErrCh = make(chan error, 1)
			cbStarted   = make(chan struct{})
			releaseCB   = make(chan struct{})
			updateErr   error
			appendErr   error
		)

		go func() {
			appendErrCh <- f.Append(func(w io.Writer) (err error) {
				var (
					timeout = time.After(2 * time.Second)
				)

				close(cbStarted)
				<-releaseCB

				for {
					_, err = w.Write([]byte("x"))
					if errors.Is(err, streambuf.ErrIsClosed) {
						return err
					}

					if err != nil {
						return err
					}

					select {
					case <-timeout:
						return errors.New("append callback timed out waiting for closed buffer")
					default:
						time.Sleep(2 * time.Millisecond)
					}
				}
			})
		}()

		<-cbStarted

		updateErr = f.Update(func(w io.Writer) (err error) {
			_, err = w.Write([]byte("after"))
			return err
		})
		if updateErr != nil {
			t.Fatalf("Update() error = %v", updateErr)
		}

		close(releaseCB)

		appendErr = <-appendErrCh
		if !errors.Is(appendErr, streambuf.ErrIsClosed) {
			t.Fatalf("Append() error = %v, want %v", appendErr, streambuf.ErrIsClosed)
		}
	})

	t.Run("update returns before old CloseAndWait completes and new ops see updated buffer", func(t *testing.T) {
		var (
			f       = newTestFile(t, "concurrent_update_return.txt", "before")
			oldBuf  *streambuf.Buffer
			oldRead io.ReadCloser
			err     error
		)

		if oldBuf, err = f.getBuffer(); err != nil {
			t.Fatalf("getBuffer() error = %v", err)
		}

		if oldRead, err = oldBuf.Reader(); err != nil {
			t.Fatalf("old buffer Reader() error = %v", err)
		}
		defer oldRead.Close()

		done := make(chan error, 1)
		go func() {
			done <- f.Update(func(w io.Writer) (err error) {
				_, err = w.Write([]byte("after"))
				return err
			})
		}()

		select {
		case err = <-done:
			if err != nil {
				t.Fatalf("Update() error = %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Update() did not return while old reader remained open")
		}

		if err = assertFileReadContent(f, "after"); err != nil {
			t.Fatalf("post-Update Read() content assertion error = %v", err)
		}

		if err = f.Append(func(w io.Writer) (err error) {
			_, err = w.Write([]byte("-next"))
			return err
		}); err != nil {
			t.Fatalf("post-Update Append() error = %v", err)
		}

		if err = assertFileReadContent(f, "after-next"); err != nil {
			t.Fatalf("post-Append Read() content assertion error = %v", err)
		}
	})

	t.Run("active non-streaming reader can drain old buffer bytes during update rotation", func(t *testing.T) {
		var (
			f       = newTestFile(t, "concurrent_reader_drain.txt", "before")
			oldBuf  *streambuf.Buffer
			oldRead io.ReadCloser
			got     string
			err     error
		)

		if oldBuf, err = f.getBuffer(); err != nil {
			t.Fatalf("getBuffer() error = %v", err)
		}

		if oldRead, err = oldBuf.Reader(); err != nil {
			t.Fatalf("old buffer Reader() error = %v", err)
		}
		defer oldRead.Close()

		if err = f.Update(func(w io.Writer) (err error) {
			_, err = w.Write([]byte("after"))
			return err
		}); err != nil {
			t.Fatalf("Update() error = %v", err)
		}

		if got, err = readAllAllowClosed(oldRead); err != nil {
			t.Fatalf("drain old reader error = %v", err)
		}

		if got != "before" {
			t.Fatalf("old reader drained %q, want %q", got, "before")
		}
	})
}

// Contract test pinned to streambuf v0.7.0: close initiation should unblock a
// waiting streaming read with ErrIsClosed, and subsequent writes/new readers are
// rejected with ErrIsClosed.
func TestStreambufV070CloseInitiationContract(t *testing.T) {
	var (
		b         = streambuf.NewMemory()
		r         io.ReadSeekCloser
		readErrCh = make(chan error, 1)
		closeErr  error
		writeErr  error
		readerErr error
		streamErr error
	)

	if r, closeErr = b.StreamingReader(); closeErr != nil {
		t.Fatalf("StreamingReader() setup error = %v", closeErr)
	}
	defer r.Close()

	go func() {
		var (
			buf = make([]byte, 1)
			err error
		)
		_, err = r.Read(buf)
		readErrCh <- err
	}()

	if closeErr = b.Close(); closeErr != nil {
		t.Fatalf("Close() error = %v", closeErr)
	}

	select {
	case err := <-readErrCh:
		if !errors.Is(err, streambuf.ErrIsClosed) {
			t.Fatalf("streaming read error = %v, want %v", err, streambuf.ErrIsClosed)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("streaming read did not unblock after Close()")
	}

	_, writeErr = b.Write([]byte("x"))
	if !errors.Is(writeErr, streambuf.ErrIsClosed) {
		t.Fatalf("Write() error = %v, want %v", writeErr, streambuf.ErrIsClosed)
	}

	if _, readerErr = b.Reader(); !errors.Is(readerErr, streambuf.ErrIsClosed) {
		t.Fatalf("Reader() error = %v, want %v", readerErr, streambuf.ErrIsClosed)
	}

	if _, streamErr = b.StreamingReader(); !errors.Is(streamErr, streambuf.ErrIsClosed) {
		t.Fatalf("StreamingReader() error = %v, want %v", streamErr, streambuf.ErrIsClosed)
	}
}

func assertFileReadContent(f *File, want string) (err error) {
	err = f.Read(func(r io.Reader) (err error) {
		var b []byte
		if b, err = readAllAllowWrappedEOF(r); err != nil {
			return err
		}

		if string(b) != want {
			return errors.New("unexpected file content")
		}

		return nil
	})
	if err != nil {
		return err
	}

	return nil
}

func newTestFile(t *testing.T, key, initial string) (f *File) {
	var (
		dir      = t.TempDir()
		fullpath = filepath.Join(dir, key)
		err      error
	)

	t.Helper()

	if err = os.WriteFile(fullpath, []byte(initial), 0o644); err != nil {
		t.Fatalf("write initial file %q: %v", fullpath, err)
	}

	return newFile(dir, key)
}

func readAllAllowWrappedEOF(r io.Reader) (out []byte, err error) {
	var (
		b   bytes.Buffer
		buf = make([]byte, 256)
		n   int
	)

	for {
		n, err = r.Read(buf)
		if n > 0 {
			if _, err = b.Write(buf[:n]); err != nil {
				return nil, err
			}
		}

		if err != nil {
			if errors.Is(err, io.EOF) {
				return b.Bytes(), nil
			}

			return nil, err
		}
	}
}

func readAllAllowClosed(r io.Reader) (out string, err error) {
	var (
		b   bytes.Buffer
		buf = make([]byte, 256)
		n   int
	)

	for {
		n, err = r.Read(buf)
		if n > 0 {
			if _, err = b.Write(buf[:n]); err != nil {
				return "", err
			}
		}

		if err != nil {
			switch {
			case errors.Is(err, io.EOF):
				return b.String(), nil
			case errors.Is(err, streambuf.ErrIsClosed):
				return b.String(), nil
			default:
				return "", err
			}
		}
	}
}
