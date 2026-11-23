package api

import (
	"log"
	"os"
	"path/filepath"
	"waystone-web/db"
)

// removeAttachmentFiles deletes stored attachment files after their row or page is gone; failures are only logged.
func removeAttachmentFiles(files []string) {
	dir, err := db.AttachmentDir()
	if err != nil {
		log.Printf("cannot remove attachments: %v", err)
		return
	}
	for _, f := range files {
		if !db.AttachmentFileRe.MatchString(f) {
			continue
		}
		if err := os.Remove(filepath.Join(dir, f)); err != nil && !os.IsNotExist(err) {
			log.Printf("failed to remove attachment %s: %v", f, err)
		}
	}
}
