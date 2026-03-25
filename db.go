package iodb

import "os"

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

type DB struct {
	*Bucket
}
