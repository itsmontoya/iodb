package iodb

import (
	"fmt"
	"os"
	"path"

	"github.com/itsmontoya/bst"
)

func newBucket(dir string, e os.DirEntry) (out *Bucket, err error) {
	var b Bucket
	b.key = e.Name()
	b.dir = dir
	if b.buckets, b.files, err = getBucketsAndFiles(path.Join(dir, b.key)); err != nil {
		return nil, err
	}

	return &b, nil
}

type Bucket struct {
	entry

	buckets bst.BST[*Bucket]
	files   bst.BST[*File]
}

func (b *Bucket) Read(key string) (err error) {
	_, ok := b.files.Get(key)
	if !ok {
		return fmt.Errorf("file of <%s> was not found", key)
	}

	return nil
}

func (b *Bucket) String() string {
	return fmt.Sprintf("Bucket<Key: %s Buckets: %v Files: %v>", b.key, b.buckets, b.files)
}
