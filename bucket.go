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

type Bucket struct {
	mux sync.RWMutex

	entry

	buckets bst.BST[*Bucket]
	files   bst.BST[*File]
}

func (b *Bucket) GetBucket(key string) (out *Bucket, ok bool) {
	b.mux.RLock()
	defer b.mux.RUnlock()
	out, ok = b.buckets.Get(key)
	return out, ok
}

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

	fullpath := filepath.Join(b.dir, key)
	if err = os.MkdirAll(fullpath, 0755); err != nil {
		return nil, err
	}

	if out, err = newBucket(b.dir, key); err != nil {
		return
	}

	b.buckets.Insert(out)
	return out, nil
}

func (b *Bucket) GetOrCreateBucket(key string) (out *Bucket, err error) {
	var ok bool
	if out, ok = b.GetBucket(key); ok {
		return out, nil
	}

	return b.CreateBucket(key)
}

func (b *Bucket) Get(key string) (out *File, ok bool) {
	b.mux.RLock()
	defer b.mux.RUnlock()
	return b.files.Get(key)
}

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

	if err = touchFile(b.dir, key); err != nil {
		return nil, err
	}

	out = newFile(b.dir, key)
	b.files.Insert(out)
	return out, nil
}

func (b *Bucket) GetOrCreate(key string) (out *File, err error) {
	var ok bool
	if out, ok = b.Get(key); ok {
		return out, nil
	}

	return b.Create(key)
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
