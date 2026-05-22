package app

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// AttachmentMaxBytes caps how large a single uploaded attachment can be.
// 10 MB matches the README guidance — bumping it requires touching the body
// cap in the HTTP layer too so a streamed upload is bounded at every step.
const AttachmentMaxBytes int64 = 10 * 1024 * 1024

// ErrAttachmentTooLarge is returned by SaveAttachmentFile when the streamed
// body exceeds AttachmentMaxBytes.
var ErrAttachmentTooLarge = errors.New("attachment exceeds maximum size")

// SaveAttachmentFile writes the streamed body to <dataDir>/attachments/<id>
// (0600 perms, parent dir 0700). On success it returns the absolute storage
// path, the number of bytes written, and the SHA-256 hex digest so the
// caller can record them in the database row.
//
// The reader is read with a hard cap of AttachmentMaxBytes + 1 bytes — one
// extra byte over the limit and the function aborts (and removes the partial
// file) with ErrAttachmentTooLarge so a misbehaving client cannot fill the
// disk by lying about Content-Length.
func SaveAttachmentFile(dataDir, attachmentID string, body io.Reader) (path string, size int64, sha string, err error) {
	if dataDir == "" || attachmentID == "" {
		return "", 0, "", errors.New("attachment: dataDir and attachmentID required")
	}
	dir := filepath.Join(dataDir, "attachments")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", 0, "", fmt.Errorf("attachment: mkdir %s: %w", dir, err)
	}
	path = filepath.Join(dir, attachmentID)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", 0, "", fmt.Errorf("attachment: create %s: %w", path, err)
	}
	hasher := sha256.New()
	// LimitReader cap is +1 so we can detect "over the limit" rather than
	// silently truncate at exactly the cap.
	limited := io.LimitReader(body, AttachmentMaxBytes+1)
	written, copyErr := io.Copy(io.MultiWriter(f, hasher), limited)
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(path)
		return "", 0, "", fmt.Errorf("attachment: write %s: %w", path, copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(path)
		return "", 0, "", fmt.Errorf("attachment: close %s: %w", path, closeErr)
	}
	if written > AttachmentMaxBytes {
		_ = os.Remove(path)
		return "", 0, "", ErrAttachmentTooLarge
	}
	return path, written, hex.EncodeToString(hasher.Sum(nil)), nil
}

// RemoveAttachmentFile deletes the on-disk file. A missing file is treated
// as success so the operation is idempotent across crash recovery.
func RemoveAttachmentFile(path string) error {
	if path == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("attachment: remove %s: %w", path, err)
	}
	return nil
}
