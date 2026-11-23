package api

import (
	"archive/zip"
	"bytes"
	"errors"
	"image"
	"io"
	"net/http"
	"os"
	"strings"
)

// maxZipRead caps how much of a file zip.NewReader may read: it loads the whole central directory, so a
// hostile archive could otherwise cost hundreds of MB of memory.
const maxZipRead = 4 << 20

// limitedReaderAt errors once more than left bytes have been read in total.
type limitedReaderAt struct {
	r    io.ReaderAt
	left int64
}

func (l *limitedReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if l.left <= 0 {
		return 0, errors.New("read limit reached")
	}
	n, err := l.r.ReadAt(p, off)
	l.left -= int64(n)
	return n, err
}

var oleMagic = []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}

// attachmentContentOK is a sanity check that the stored file is what its extension says (ext is lower case, with the
// dot). It is not a virus scan: it keeps an HTML or script file from passing as a document, PDF or picture.
func attachmentContentOK(f *os.File, size int64, ext string) bool {
	if size <= 0 {
		return false
	}
	head := make([]byte, 1024)
	n, _ := f.ReadAt(head, 0)
	head = head[:n]
	switch ext {
	case ".pdf":
		return bytes.Contains(head, []byte("%PDF-"))
	case ".png", ".jpg", ".jpeg", ".webp", ".gif":
		_, format, err := image.DecodeConfig(io.NewSectionReader(f, 0, size))
		want := map[string]string{".png": "png", ".jpg": "jpeg", ".jpeg": "jpeg", ".webp": "webp", ".gif": "gif"}[ext]
		return err == nil && format == want
	case ".docx", ".xlsx", ".pptx", ".odt", ".ods", ".odp":
		return zipHas(f, size, ext)
	case ".zip":
		return bytes.HasPrefix(head, []byte("PK\x03\x04")) || bytes.HasPrefix(head, []byte("PK\x05\x06"))
	case ".doc", ".xls", ".ppt":
		return bytes.HasPrefix(head, oleMagic)
	case ".txt", ".md", ".csv":
		if len(head) > 512 {
			head = head[:512]
		}
		return strings.HasPrefix(http.DetectContentType(head), "text/plain")
	case ".mp3":
		if len(head) < 3 {
			return false
		}
		return bytes.HasPrefix(head, []byte("ID3")) || (head[0] == 0xFF && head[1]&0xE0 == 0xE0 && head[1]&0x06 == 0x02)
	}
	return false
}

// zipHas checks the container of an Office Open XML or OpenDocument file.
func zipHas(f *os.File, size int64, ext string) bool {
	zr, err := zip.NewReader(&limitedReaderAt{r: f, left: maxZipRead}, size)
	if err != nil {
		return false
	}
	if want, ok := map[string]string{
		".odt": "application/vnd.oasis.opendocument.text",
		".ods": "application/vnd.oasis.opendocument.spreadsheet",
		".odp": "application/vnd.oasis.opendocument.presentation",
	}[ext]; ok {
		for _, e := range zr.File {
			if e.Name != "mimetype" {
				continue
			}
			rc, err := e.Open()
			if err != nil {
				return false
			}
			defer rc.Close()
			b, err := io.ReadAll(io.LimitReader(rc, 100))
			return err == nil && string(b) == want
		}
		return false
	}
	dir := map[string]string{".docx": "word/", ".xlsx": "xl/", ".pptx": "ppt/"}[ext]
	types, part := false, false
	for _, e := range zr.File {
		if e.Name == "[Content_Types].xml" {
			types = true
		} else if strings.HasPrefix(e.Name, dir) {
			part = true
		}
	}
	return types && part
}
