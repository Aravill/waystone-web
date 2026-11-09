package db

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sync"
)

// dbPath is the resolved database file; the default upload directory sits next to it.
var dbPath string

// UploadFileRe is the only shape of file name stored in, served from, removed from and mirrored
// out of the upload directory.
var UploadFileRe = regexp.MustCompile(`^[0-9a-f-]{36}\.(png|jpg|webp)$`)

var (
	uploadDirOnce sync.Once
	uploadDir     string
	uploadDirErr  error
)

// UploadDir is where Codex map images are stored: UPLOAD_DIR if set, otherwise an "uploads"
// folder next to the database file (so it lives on the same Docker volume). It is created on first use.
func UploadDir() (string, error) {
	uploadDirOnce.Do(func() {
		uploadDir = os.Getenv("UPLOAD_DIR")
		if uploadDir == "" {
			uploadDir = filepath.Join(filepath.Dir(dbPath), "uploads")
		}
		uploadDirErr = os.MkdirAll(uploadDir, 0o755)
	})
	return uploadDir, uploadDirErr
}

// CreateUploadTemp opens a temp file in the upload directory. Finish it with CommitUpload,
// or drop it with DiscardUpload.
func CreateUploadTemp() (*os.File, error) {
	dir, err := UploadDir()
	if err != nil {
		return nil, err
	}
	return os.CreateTemp(dir, ".upload-*")
}

// DiscardUpload closes and removes a temp file.
func DiscardUpload(tmp *os.File) {
	tmp.Close()
	os.Remove(tmp.Name())
}

// CommitUpload syncs and closes the temp file and renames it to name in the upload directory.
// The temp file is removed on failure.
func CommitUpload(tmp *os.File, name string) error {
	dir, err := UploadDir()
	if err == nil {
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(tmp.Name(), 0o644)
	}
	if err == nil {
		err = os.Rename(tmp.Name(), filepath.Join(dir, name))
	}
	if err != nil {
		os.Remove(tmp.Name())
	}
	return err
}

// WriteUpload stores what write produces under name, via a temp file and a rename.
func WriteUpload(name string, write func(io.Writer) error) error {
	tmp, err := CreateUploadTemp()
	if err != nil {
		return err
	}
	if err := write(tmp); err != nil {
		DiscardUpload(tmp)
		return err
	}
	return CommitUpload(tmp, name)
}
