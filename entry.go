package iodb

import "path/filepath"

type entry struct {
	key string
	dir string
}

func (e entry) Key() (out string) {
	return e.key
}

func (e *entry) filepath() (out string) {
	return filepath.Join(e.dir, e.key)
}
