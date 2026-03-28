package iodb

import "github.com/itsmontoya/bst"

func makeCursor(b bst.BST[*File]) (c Cursor) {
	c.Cursor = b.Cursor()
	return c
}

// Cursor iterates files in key order for a bucket snapshot.
type Cursor struct {
	*bst.Cursor[*File]
}

func (c *Cursor) cleanup() {
	// Invalidate underlyhing bst.Cursor so it cannot be used outside of the mutex lock
	c.Cursor = nil
}
