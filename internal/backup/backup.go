package backup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

var (
	ErrBackupSourceRequired      = errors.New("backup source path is required")
	ErrRestoreSourceRequired     = errors.New("restore source path is required")
	ErrRestoreDestinationMissing = errors.New("restore destination path is required")
	ErrCopyPathRequired          = errors.New("copy source and destination are required")
)

const (
	privateDirMode  = 0o700
	privateFileMode = 0o600
)

type Checkpointer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

type Result struct {
	Path      string
	SizeBytes int64
}

func Database(ctx context.Context, db Checkpointer, sourcePath, destPath string, at time.Time) (Result, error) {
	if sourcePath == "" {
		return Result{}, fmt.Errorf("%w", ErrBackupSourceRequired)
	}
	if destPath == "" {
		if at.IsZero() {
			at = time.Now().UTC()
		}
		destPath = sourcePath + "." + at.UTC().Format("20060102T150405Z") + ".bak"
	}
	if db != nil {
		if _, err := db.ExecContext(ctx, `PRAGMA wal_checkpoint(FULL)`); err != nil {
			return Result{}, err
		}
	}
	size, err := CopyFile(sourcePath, destPath)
	if err != nil {
		return Result{}, err
	}
	return Result{Path: destPath, SizeBytes: size}, nil
}

func Restore(sourcePath, destPath string, at time.Time) (string, error) {
	if sourcePath == "" {
		return "", fmt.Errorf("%w: --from", ErrRestoreSourceRequired)
	}
	if destPath == "" {
		return "", fmt.Errorf("%w", ErrRestoreDestinationMissing)
	}
	if _, err := os.Stat(sourcePath); err != nil {
		return "", err
	}
	preRestore := ""
	if _, err := os.Stat(destPath); err == nil {
		if at.IsZero() {
			at = time.Now().UTC()
		}
		preRestore = destPath + ".pre-restore-" + at.UTC().Format("20060102T150405Z")
		if _, err := CopyFile(destPath, preRestore); err != nil {
			return "", err
		}
	}
	_, err := CopyFile(sourcePath, destPath)
	return preRestore, err
}

func CopyFile(from, to string) (int64, error) {
	if from == "" || to == "" {
		return 0, fmt.Errorf("%w", ErrCopyPathRequired)
	}
	if err := ensureBackupOutputDir(filepath.Dir(to)); err != nil {
		return 0, err
	}
	in, err := os.Open(from)
	if err != nil {
		return 0, err
	}
	defer in.Close()
	out, err := os.OpenFile(to, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, privateFileMode)
	if err != nil {
		return 0, err
	}
	if err := out.Chmod(privateFileMode); err != nil {
		_ = out.Close()
		return 0, err
	}
	n, copyErr := io.Copy(out, in)
	syncErr := out.Sync()
	closeErr := out.Close()
	if copyErr != nil {
		return n, copyErr
	}
	if syncErr != nil {
		return n, syncErr
	}
	return n, closeErr
}

func ensureBackupOutputDir(dir string) error {
	clean := filepath.Clean(dir)
	if clean == "." || clean == string(os.PathSeparator) {
		return nil
	}
	existed := true
	if _, err := os.Stat(clean); errors.Is(err, os.ErrNotExist) {
		existed = false
	} else if err != nil {
		return err
	}
	if err := os.MkdirAll(clean, privateDirMode); err != nil {
		return err
	}
	// Newly created backup directories use 0700. Also tighten the standard
	// automatic "backups" directory when upgrading from older wider defaults,
	// without chmodding arbitrary existing parents such as /tmp.
	if !existed || filepath.Base(clean) == "backups" {
		_ = os.Chmod(clean, privateDirMode)
	}
	return nil
}
