package iodb

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

var isValidKey = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9._-]{0,254})$`)

func removeTempFile(dir, name string) {
	fullpath := filepath.Join(dir, name)
	_ = os.Remove(fullpath)
}

func tempFile(originalFilepath string, fn func(io.Writer) error) (tempPath string, err error) {
	var tf *os.File
	dir := filepath.Dir(originalFilepath)
	name := filepath.Base(originalFilepath)
	if tf, err = os.CreateTemp(dir, ".tmp_"+name); err != nil {
		return "", err
	}

	tempPath = tf.Name()
	err = errors.Join(fn(tf), tf.Sync())
	_ = tf.Close()

	if err != nil {
		_ = os.Remove(tf.Name())
		return "", err
	}

	return tempPath, nil
}

func touchFile(dir, key string) (err error) {
	var f *os.File
	fullpath := filepath.Join(dir, key)
	if f, err = os.Create(fullpath); err != nil {
		return err
	}
	_ = f.Close()
	return nil
}

func syncDir(dir string) (err error) {
	var d *os.File
	if d, err = os.Open(dir); err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

func validateKey(key string) (err error) {
	switch {
	case key == "":
		return ErrEmptyKey
	case !isValidKey.MatchString(key):
		return ErrInvalidKeyFormat
	default:
		return nil
	}
}
