package imagestore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pngBytes is the smallest possible valid-ish PNG payload: it is enough that
// the disk store hashes it and writes it back out unchanged. The store does
// not validate PNG structure itself.
var pngBytes = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, // PNG signature
	0x00, 0x00, 0x00, 0x0d, // IHDR length
	'I', 'H', 'D', 'R',
	0x00, 0x00, 0x00, 0x01, // width = 1
	0x00, 0x00, 0x00, 0x01, // height = 1
	0x08, 0x00, 0x00, 0x00, 0x00,
}

func TestDiskSaveWritesFileAndReturnsExpectedURL(t *testing.T) {
	dir := t.TempDir()
	store := NewDisk(dir, "/uploads")

	url, err := store.Save(context.Background(), "tiny.png", "image/png", bytes.NewReader(pngBytes))
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	sum := sha256.Sum256(pngBytes)
	hash := hex.EncodeToString(sum[:])
	wantURL := "/uploads/" + hash[:2] + "/" + hash + ".png"
	if url != wantURL {
		t.Fatalf("unexpected URL\n got: %q\nwant: %q", url, wantURL)
	}

	want := filepath.Join(dir, hash[:2], hash+".png")
	got, err := os.ReadFile(want)
	if err != nil {
		t.Fatalf("expected file at %s: %v", want, err)
	}
	if !bytes.Equal(got, pngBytes) {
		t.Fatalf("file contents differ from input")
	}
}

func TestDiskSaveRejectsUnsupportedType(t *testing.T) {
	store := NewDisk(t.TempDir(), "/uploads")

	_, err := store.Save(context.Background(), "evil.exe", "application/octet-stream", bytes.NewReader([]byte("hello")))
	if !errors.Is(err, ErrUnsupportedType) {
		t.Fatalf("expected ErrUnsupportedType, got %v", err)
	}
}

func TestDiskSaveRejectsOversizedInput(t *testing.T) {
	store := NewDisk(t.TempDir(), "/uploads")

	big := bytes.NewReader(make([]byte, MaxImageSize+10))
	_, err := store.Save(context.Background(), "big.png", "image/png", big)
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("expected ErrTooLarge, got %v", err)
	}
}

func TestDiskSaveDeduplicates(t *testing.T) {
	dir := t.TempDir()
	store := NewDisk(dir, "/uploads")

	url1, err := store.Save(context.Background(), "a.png", "image/png", bytes.NewReader(pngBytes))
	if err != nil {
		t.Fatalf("first Save: %v", err)
	}
	url2, err := store.Save(context.Background(), "b.png", "image/png", bytes.NewReader(pngBytes))
	if err != nil {
		t.Fatalf("second Save: %v", err)
	}
	if url1 != url2 {
		t.Fatalf("expected dedupe to return same URL, got %q vs %q", url1, url2)
	}
	if !strings.HasPrefix(url1, "/uploads/") {
		t.Fatalf("expected /uploads/ prefix, got %q", url1)
	}
}
