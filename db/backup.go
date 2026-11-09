package db

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultBackupKeep = 14
	backupInterval    = 24 * time.Hour
)

// StartBackups writes a database snapshot to BACKUP_DIR now and every 24h until
// ctx is done. Disabled when BACKUP_DIR is unset. Errors are logged, never fatal.
func StartBackups(ctx context.Context) {
	dir := os.Getenv("BACKUP_DIR")
	if dir == "" {
		log.Println("Backups disabled (BACKUP_DIR unset)")
		return
	}

	keep := defaultBackupKeep
	if v := os.Getenv("BACKUP_KEEP"); v != "" {
		if n, err := strconv.Atoi(v); err != nil || n < 1 {
			log.Printf("WARNING: invalid BACKUP_KEEP %q, using %d", v, defaultBackupKeep)
		} else {
			keep = n
		}
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Printf("Backups disabled: cannot create %s: %v", dir, err)
		return
	}

	runBackup(dir, keep)
	go func() {
		ticker := time.NewTicker(backupInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				runBackup(dir, keep)
			}
		}
	}()
}

func runBackup(dir string, keep int) {
	name := fmt.Sprintf("waystone-%s.db", time.Now().UTC().Format("20060102-150405"))
	final := filepath.Join(dir, name)
	// Temp name does not match waystone-*.db, so pruning never sees partial files.
	tmp := filepath.Join(dir, ".tmp-"+name)
	_ = os.Remove(tmp)

	if err := Backup(tmp); err != nil {
		log.Printf("Backup failed: %v", err)
		_ = os.Remove(tmp)
		return
	}
	if err := os.Rename(tmp, final); err != nil {
		log.Printf("Backup failed: %v", err)
		_ = os.Remove(tmp)
		return
	}
	log.Printf("Backup written: %s", final)
	mirrorUploads(filepath.Join(dir, "uploads"))

	files, err := filepath.Glob(filepath.Join(dir, "waystone-*.db"))
	if err != nil {
		log.Printf("Backup prune failed: %v", err)
		return
	}
	sort.Strings(files)
	for len(files) > keep {
		if err := os.Remove(files[0]); err != nil {
			log.Printf("Backup prune failed: %v", err)
		}
		files = files[1:]
	}
}

// mirrorUploads copies Codex map images that are not yet in the mirror directory. Uploaded files
// never change (their names are random), so a name that exists there is already backed up. It
// never deletes from the mirror: images of deleted pages stay until you clear them by hand.
func mirrorUploads(mirror string) {
	src, err := UploadDir()
	if err != nil {
		log.Printf("Upload backup failed: %v", err)
		return
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		log.Printf("Upload backup failed: %v", err)
		return
	}
	if err := os.MkdirAll(mirror, 0o755); err != nil {
		log.Printf("Upload backup failed: %v", err)
		return
	}
	copied := 0
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		dst := filepath.Join(mirror, e.Name())
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		if err := copyFile(filepath.Join(src, e.Name()), dst); err != nil {
			log.Printf("Upload backup failed for %s: %v", e.Name(), err)
			continue
		}
		copied++
	}
	if copied > 0 {
		log.Printf("Upload backup: copied %d new file(s) to %s", copied, mirror)
	}
}

// copyFile copies via a temp name so a half-written file is never taken for a finished one.
func copyFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := filepath.Join(filepath.Dir(to), ".tmp-"+filepath.Base(to))
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, to)
}
