package app

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveAttachmentFileWritesBodyComputesSha256AndPermissions(t *testing.T) {
	dataDir := t.TempDir()
	body := []byte("hello, attachment world\n")
	path, size, sha, err := SaveAttachmentFile(dataDir, "att-1", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dataDir, "attachments", "att-1")
	if path != want {
		t.Fatalf("path=%q want %q", path, want)
	}
	if size != int64(len(body)) {
		t.Fatalf("size=%d want %d", size, len(body))
	}
	hash := sha256.Sum256(body)
	if sha != hex.EncodeToString(hash[:]) {
		t.Fatalf("sha mismatch: got %q want %q", sha, hex.EncodeToString(hash[:]))
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("file perm=%#o want 0600", got)
	}
	parent, _ := os.Stat(filepath.Join(dataDir, "attachments"))
	if got := parent.Mode().Perm(); got != 0o700 {
		t.Fatalf("dir perm=%#o want 0700", got)
	}
	content, _ := os.ReadFile(path)
	if string(content) != string(body) {
		t.Fatalf("body mismatch: %q vs %q", content, body)
	}
}

func TestSaveAttachmentFileRejectsOversizedBody(t *testing.T) {
	dataDir := t.TempDir()
	// 10MB + 1 byte = right over the cap.
	body := strings.Repeat("x", int(AttachmentMaxBytes)+1)
	_, _, _, err := SaveAttachmentFile(dataDir, "att-big", bytes.NewReader([]byte(body)))
	if !errors.Is(err, ErrAttachmentTooLarge) {
		t.Fatalf("expected ErrAttachmentTooLarge, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dataDir, "attachments", "att-big")); !os.IsNotExist(statErr) {
		t.Fatalf("oversized upload should not leave a partial file on disk: err=%v", statErr)
	}
}

func TestSaveAttachmentFileRejectsExistingID(t *testing.T) {
	dataDir := t.TempDir()
	if _, _, _, err := SaveAttachmentFile(dataDir, "att-dup", bytes.NewReader([]byte("first"))); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := SaveAttachmentFile(dataDir, "att-dup", bytes.NewReader([]byte("second"))); err == nil {
		t.Fatalf("expected create error on duplicate ID, got nil")
	}
}

func TestRemoveAttachmentFileIsIdempotent(t *testing.T) {
	dataDir := t.TempDir()
	path, _, _, err := SaveAttachmentFile(dataDir, "att-rm", bytes.NewReader([]byte("bye")))
	if err != nil {
		t.Fatal(err)
	}
	if err := RemoveAttachmentFile(path); err != nil {
		t.Fatalf("first remove: %v", err)
	}
	// Removing a missing path is a no-op.
	if err := RemoveAttachmentFile(path); err != nil {
		t.Fatalf("second remove (idempotent): %v", err)
	}
	// Empty path is a no-op (so transactional rollback paths don't have to
	// guard against zero-value cleanup).
	if err := RemoveAttachmentFile(""); err != nil {
		t.Fatalf("empty path remove: %v", err)
	}
}
