package iodb

type entry struct {
	key string
	dir string
}

func (e entry) Key() (out string) {
	return e.key
}
