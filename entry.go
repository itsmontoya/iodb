package iodb

import "path/filepath"

type entry struct {
	key string
	dir string
}

// Key returns the entry key used as the on-disk file or directory name.
func (e entry) Key() (out string) {
	return e.key
}

func (e *entry) filepath() (out string) {
	return filepath.Join(e.dir, e.key)
}
