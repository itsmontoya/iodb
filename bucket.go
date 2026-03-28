package iodb

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/itsmontoya/bst"
)

func newBucket(dir, name string) (out *Bucket, err error) {
	var b Bucket
	b.key = name
	b.dir = dir
	if err = b.populateFromDirPath(); err != nil {
		return nil, err
	}

	return &b, nil
}

// Bucket groups child buckets and files under a shared directory path.
type Bucket struct {
	mux sync.RWMutex

	entry

	buckets bst.BST[*Bucket]
	files   bst.BST[*File]
}

// GetBucket returns the child bucket for key when it exists.
func (b *Bucket) GetBucket(key string) (out *Bucket, ok bool) {
	b.mux.RLock()
	defer b.mux.RUnlock()
	out, ok = b.buckets.Get(key)
	return out, ok
}

// CreateBucket validates key and creates a child bucket when missing.
func (b *Bucket) CreateBucket(key string) (out *Bucket, err error) {
	if err = validateKey(key); err != nil {
		return
	}

	var ok bool
	b.mux.Lock()
	defer b.mux.Unlock()
	if out, ok = b.buckets.Get(key); ok {
		return out, nil
	}

	fullpath := filepath.Join(b.filepath(), key)
	if err = os.MkdirAll(fullpath, 0755); err != nil {
		return nil, err
	}

	if out, err = newBucket(b.filepath(), key); err != nil {
		return
	}

	b.buckets.Insert(out)
	return out, nil
}

// GetOrCreateBucket returns an existing child bucket or creates it.
func (b *Bucket) GetOrCreateBucket(key string) (out *Bucket, err error) {
	var ok bool
	if out, ok = b.GetBucket(key); ok {
		return out, nil
	}

	return b.CreateBucket(key)
}

// Get returns the file for key when it exists.
func (b *Bucket) Get(key string) (out *File, ok bool) {
	b.mux.RLock()
	defer b.mux.RUnlock()
	return b.files.Get(key)
}

// Create validates key and creates a file when it is not already present.
func (b *Bucket) Create(key string) (out *File, err error) {
	if err = validateKey(key); err != nil {
		return
	}

	var ok bool
	b.mux.Lock()
	defer b.mux.Unlock()
	if out, ok = b.files.Get(key); ok {
		return out, nil
	}

	if err = touchFile(b.filepath(), key); err != nil {
		return nil, err
	}

	out = newFile(b.filepath(), key)
	b.files.Insert(out)
	return out, nil
}

// GetOrCreate returns an existing file for key or creates it.
func (b *Bucket) GetOrCreate(key string) (out *File, err error) {
	var ok bool
	if out, ok = b.Get(key); ok {
		return out, nil
	}

	return b.Create(key)
}

// Delete removes key from the bucket if present.
//
// Delete validates key, closes the file buffer when initialized, removes the
// file from disk, and then removes the key from the in-memory index.
//
// If key is not present, Delete returns nil.
func (b *Bucket) Delete(key string) (err error) {
	if err = validateKey(key); err != nil {
		return
	}

	var (
		ok bool
		f  *File
	)

	if f, ok = b.Get(key); !ok {
		return nil
	}

	// Best-effort close: delete is authoritative here, so close errors are ignored and remove decides outcome.
	_ = f.close()

	if err = os.Remove(f.filepath()); err != nil {
		return err
	}

	b.mux.Lock()
	defer b.mux.Unlock()
	b.files.Remove(key)
	return nil
}

// Cursor executes fn with a read-only cursor over files in key order.
func (b *Bucket) Cursor(fn func(*Cursor) error) (err error) {
	b.mux.RLock()
	defer b.mux.RUnlock()
	c := makeCursor(b.files)
	defer c.cleanup()
	return fn(&c)
}

// ForEach calls fn for each file in key order until fn returns an error.
func (b *Bucket) ForEach(fn func(*File) error) (err error) {
	b.mux.RLock()
	defer b.mux.RUnlock()
	for _, f := range b.files {
		if err = fn(f); err != nil {
			return err
		}
	}

	return nil
}

func (b *Bucket) populateFromDirPath() (err error) {
	var es []os.DirEntry
	if es, err = os.ReadDir(b.filepath()); err != nil {
		return err
	}

	for _, e := range es {
		if err = b.insertEntry(e); err != nil {
			return err
		}
	}

	return
}

func (b *Bucket) insertEntry(e os.DirEntry) (err error) {
	switch {
	case e.IsDir():
		var child *Bucket
		if child, err = newBucket(b.filepath(), e.Name()); err != nil {
			return
		}

		b.buckets.Insert(child)
	case strings.Index(e.Name(), ".tmp_") == 0:
		removeTempFile(b.filepath(), e.Name())
	default:
		b.files.Insert(newFile(b.filepath(), e.Name()))
	}

	return
}
