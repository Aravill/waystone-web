package db

import (
	"os"
	"path/filepath"
)

// dbPath is the resolved database file; the default upload directory sits next to it.
var dbPath string

// UploadDir is where Codex map images are stored: UPLOAD_DIR if set, otherwise an "uploads"
// folder next to the database file (so it lives on the same Docker volume). It is created on use.
func UploadDir() (string, error) {
	dir := os.Getenv("UPLOAD_DIR")
	if dir == "" {
		dir = filepath.Join(filepath.Dir(dbPath), "uploads")
	}
	return dir, os.MkdirAll(dir, 0o755)
}
