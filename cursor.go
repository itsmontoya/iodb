package iodb

import "github.com/itsmontoya/bst"

// Cursor iterates files in key order for a bucket snapshot.
type Cursor struct {
	*bst.Cursor[*File]
}
