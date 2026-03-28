package iodb

import "github.com/itsmontoya/bst"

type Cursor struct {
	*bst.Cursor[*File]
}
