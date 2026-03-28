package iodb

import "os"

// New creates or opens a database rooted at dbPath.
func New(dbPath string) (out *DB, err error) {
	if err = os.MkdirAll(dbPath, 0755); err != nil {
		return nil, err
	}

	var db DB
	if db.Bucket, err = newBucket(dbPath, ""); err != nil {
		return
	}

	return &db, nil
}

// DB is the root container for buckets and files under a database path.
type DB struct {
	*Bucket
}
