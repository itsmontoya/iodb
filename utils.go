package iodb

import (
	"io"
	"os"

	"github.com/itsmontoya/bst"
)

func getBucketsAndFiles(dir string) (bs bst.BST[*Bucket], fs bst.BST[*File], err error) {
	var es []os.DirEntry
	if es, err = os.ReadDir(dir); err != nil {
		return nil, nil, err
	}

	for _, e := range es {
		switch {
		case e.IsDir():
			var b *Bucket
			if b, err = newBucket(dir, e); err != nil {
				return
			}

			bs.Insert(b)
		default:
			fs.Insert(newFile(dir, e))
		}
	}

	return
}

func tempFile(fn func(io.Writer) error) (filepath string, err error) {
	var tf *os.File
	if tf, err = os.CreateTemp("", "sifty_"); err != nil {
		return "", err
	}

	filepath = tf.Name()
	err = fn(tf)
	_ = tf.Close()

	if err != nil {
		_ = os.Remove(tf.Name())
		return "", err
	}

	return filepath, nil
}
