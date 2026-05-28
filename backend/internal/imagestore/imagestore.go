// Package imagestore provides a small interface for persisting uploaded
// images plus a local-disk implementation. The interface is intentionally
// narrow so an S3 (or other) backend can be swapped in later without
// touching call sites.
package imagestore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// MaxImageSize is the largest image (in bytes) the disk store will accept.
const MaxImageSize int64 = 5 << 20 // 5 MiB

// ErrUnsupportedType is returned when the uploaded content type is not in the
// allow-list.
var ErrUnsupportedType = errors.New("unsupported image type")

// ErrTooLarge is returned when the uploaded bytes exceed MaxImageSize.
var ErrTooLarge = errors.New("image too large")

// Store persists an uploaded image and returns the public URL where it will
// be served.
type Store interface {
	// Save persists the uploaded file and returns the public URL it will be
	// served at (e.g. "/uploads/ab/cdef1234.jpg"). filename is the original
	// upload name (used for the extension fallback); contentType is the MIME
	// type from the upload header.
	Save(ctx context.Context, filename string, contentType string, r io.Reader) (string, error)
}

// extByContentType maps an allowed MIME type to a canonical file extension.
var extByContentType = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
	"image/gif":  ".gif",
}

// Disk is a Store that writes images to a local directory. Files are
// content-addressed (sha256), so identical uploads are de-duplicated.
type Disk struct {
	root      string
	urlPrefix string
}

// NewDisk constructs a Disk store rooted at root that serves files under the
// given URL prefix (e.g. "/uploads"). The root directory is created on first
// write (per upload).
func NewDisk(root, urlPrefix string) *Disk {
	if urlPrefix == "" {
		urlPrefix = "/uploads"
	}
	urlPrefix = strings.TrimRight(urlPrefix, "/")
	return &Disk{root: root, urlPrefix: urlPrefix}
}

// Save implements Store. It enforces a 5 MiB size cap and a content-type
// allow-list (jpeg / png / webp / gif) before writing.
func (d *Disk) Save(_ context.Context, filename, contentType string, r io.Reader) (string, error) {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	// Some clients send "image/jpg" instead of the canonical "image/jpeg".
	if ct == "image/jpg" {
		ct = "image/jpeg"
	}
	ext, ok := extByContentType[ct]
	if !ok {
		// Fall back to the filename extension if it is in the allow-list — this
		// keeps the store working when a client sends an empty/unknown
		// content-type but a sensible filename.
		fallback := strings.ToLower(filepath.Ext(filename))
		switch fallback {
		case ".jpg", ".jpeg":
			ext = ".jpg"
		case ".png":
			ext = ".png"
		case ".webp":
			ext = ".webp"
		case ".gif":
			ext = ".gif"
		default:
			return "", ErrUnsupportedType
		}
	}

	// Read into memory with a +1 cap so we can detect oversize input without
	// silently truncating it.
	limited := io.LimitReader(r, MaxImageSize+1)
	buf, err := io.ReadAll(limited)
	if err != nil {
		return "", fmt.Errorf("read image: %w", err)
	}
	if int64(len(buf)) > MaxImageSize {
		return "", ErrTooLarge
	}

	sum := sha256.Sum256(buf)
	hash := hex.EncodeToString(sum[:])
	bucket := hash[:2]
	relDir := bucket
	relPath := filepath.Join(relDir, hash+ext)

	dir := filepath.Join(d.root, relDir)
	if err = os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir uploads: %w", err)
	}
	full := filepath.Join(d.root, relPath)
	// Skip the write if a file with this hash already exists — content is
	// identical by construction (sha256 collision aside).
	if _, statErr := os.Stat(full); statErr != nil {
		if err = os.WriteFile(full, buf, 0o644); err != nil {
			return "", fmt.Errorf("write image: %w", err)
		}
	}

	// Always use forward slashes in URLs even on Windows.
	url := d.urlPrefix + "/" + bucket + "/" + hash + ext
	return url, nil
}

// Serve returns an http.Handler that serves files from root under the
// "/uploads/" URL prefix. It is intended to be wired alongside the Disk
// store's urlPrefix.
func Serve(root string) http.Handler {
	return http.StripPrefix("/uploads/", http.FileServer(http.Dir(root)))
}
