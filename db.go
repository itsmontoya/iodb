package iodb

func New(dbPath string) (out *DB, err error) {
	var db DB
	if db.Bucket, err = newBucket(dbPath, ""); err != nil {
		return
	}

	return &db, nil
}

type DB struct {
	*Bucket
}
