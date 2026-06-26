package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"reasonix/internal/fileutil"
)

const maxLocalBackups = 10

// RotateBackups rotates local backups of a config file before writing.
// Backups are stored in a config-backups/ directory next to the file.
// If newContent is provided and byte-identical to the current file, rotation
// is skipped (no-op dedup).
func RotateBackups(path string, newContent []byte) error {
	backupDir := filepath.Join(filepath.Dir(path), "config-backups")
	base := filepath.Base(path)

	// Dedup: skip if content wouldn't change.
	if len(newContent) > 0 {
		if existing, err := os.ReadFile(path); err == nil && string(existing) == string(newContent) {
			return nil
		}
	}

	if err := os.MkdirAll(backupDir, 0700); err != nil {
		return fmt.Errorf("backup dir: %w", err)
	}

	// Shift rotation: .9 → .10 (discard), .8 → .9, ... .0 → .1
	for i := maxLocalBackups - 1; i >= 0; i-- {
		old := filepath.Join(backupDir, backupName(base, i))
		if i == maxLocalBackups-1 {
			os.Remove(old)
			continue
		}
		new := filepath.Join(backupDir, backupName(base, i+1))
		os.Rename(old, new)
	}

	// Copy current file to .0
	current, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	slot0 := filepath.Join(backupDir, backupName(base, 0))
	if err := fileutil.AtomicWriteFile(slot0, current, 0600); err != nil {
		return fmt.Errorf("backup slot 0: %w", err)
	}
	return nil
}

// TryRecoverBackup attempts to recover a config file from its most recent backup.
// Returns the recovered content or an error if no backup is available.
func TryRecoverBackup(path string) ([]byte, error) {
	backupDir := filepath.Join(filepath.Dir(path), "config-backups")
	base := filepath.Base(path)

	for i := 0; i < maxLocalBackups; i++ {
		slot := filepath.Join(backupDir, backupName(base, i))
		data, err := os.ReadFile(slot)
		if err != nil {
			continue
		}
		if len(data) > 0 {
			return data, nil
		}
	}
	return nil, fmt.Errorf("no backup found for %s", path)
}

func backupName(base string, slot int) string {
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	return name + ext + "." + strconv.Itoa(slot)
}
