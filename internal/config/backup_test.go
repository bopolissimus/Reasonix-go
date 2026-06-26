// REX-65: Config backup rotation + recovery tests.
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRotateBackups(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	// Write initial config
	if err := os.WriteFile(path, []byte(`{"version":1}`), 0600); err != nil {
		t.Fatal(err)
	}

	// Rotate
	if err := RotateBackups(path, []byte(`{"version":2}`)); err != nil {
		t.Fatal(err)
	}

	// Check backup .0 exists
	backupDir := filepath.Join(dir, "config-backups")
	slot0 := filepath.Join(backupDir, "config.json.0")
	if _, err := os.Stat(slot0); err != nil {
		t.Fatalf("backup slot 0 missing: %v", err)
	}

	data, err := os.ReadFile(slot0)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"version":1}` {
		t.Errorf("backup content = %q, want %q", string(data), `{"version":1}`)
	}
}

func TestRotateBackupsShift(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	// Write 3 versions with rotation between each
	for i := 1; i <= 3; i++ {
		if err := os.WriteFile(path, []byte(`version`), 0600); err != nil {
			t.Fatal(err)
		}
		if err := RotateBackups(path, nil); err != nil {
			t.Fatal(err)
		}
	}

	backupDir := filepath.Join(dir, "config-backups")
	// .0, .1, .2 should all exist
	for i := 0; i < 3; i++ {
		slot := filepath.Join(backupDir, "config.json."+string(rune('0'+i)))
		if _, err := os.Stat(slot); err != nil {
			t.Errorf("backup slot %d missing: %v", i, err)
		}
	}
	// .3 should not exist (only 3 writes)
	if _, err := os.Stat(filepath.Join(backupDir, "config.json.3")); err == nil {
		t.Error("backup slot 3 should not exist")
	}
}

func TestRotateBackupsDedup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	content := []byte(`{"version":1}`)
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}

	// First rotate creates backup
	if err := RotateBackups(path, []byte(`{"version":2}`)); err != nil {
		t.Fatal(err)
	}

	backupDir := filepath.Join(dir, "config-backups")
	slot0 := filepath.Join(backupDir, "config.json.0")
	data1, _ := os.ReadFile(slot0)

	// Second rotate with same new content → dedup, no-op
	if err := RotateBackups(path, []byte(`{"version":2}`)); err != nil {
		t.Fatal(err)
	}

	data2, _ := os.ReadFile(slot0)
	if string(data1) != string(data2) {
		t.Error("dedup should not have changed backup slot 0")
	}
}

func TestTryRecoverBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	// Write + rotate to create a backup
	if err := os.WriteFile(path, []byte(`{"version":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := RotateBackups(path, nil); err != nil {
		t.Fatal(err)
	}

	// Delete the original
	os.Remove(path)

	// Recover
	data, err := TryRecoverBackup(path)
	if err != nil {
		t.Fatalf("recover: %v", err)
	}
	if string(data) != `{"version":1}` {
		t.Errorf("recovered = %q, want %q", string(data), `{"version":1}`)
	}
}

func TestTryRecoverBackupNone(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.json")
	_, err := TryRecoverBackup(path)
	if err == nil {
		t.Error("expected error for nonexistent backup")
	}
}
