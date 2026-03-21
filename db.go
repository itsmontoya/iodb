package iodb

import (
	"fmt"
)

func New(dbPath string) (out *DB, err error) {
	var db DB
	if db.buckets, db.files, err = getBucketsAndFiles(dbPath); err != nil {
		return nil, err
	}

	fmt.Println("DB", &db)
	return &db, nil
}

type DB struct {
	Bucket
}

func (db *DB) String() string {
	return fmt.Sprintf("Bucket<Buckets: %v Files: %v>", db.buckets, db.files)
}
