package bot

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestFileOffsetStorePersistsAcrossInstances(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "update.offset")
	store := NewFileOffsetStore(path)
	if offset, err := store.Load(); err != nil || offset != 0 {
		t.Fatalf("initial Load() = %d, %v; want 0, nil", offset, err)
	}
	if err := store.Save(12345); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(12346); err != nil {
		t.Fatal(err)
	}
	restarted := NewFileOffsetStore(path)
	if offset, err := restarted.Load(); err != nil || offset != 12346 {
		t.Fatalf("restarted Load() = %d, %v; want 12346, nil", offset, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("offset permissions = %o, want 600", info.Mode().Perm())
	}
}

func TestFileOffsetStoreRejectsCorruptState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update.offset")
	if err := os.WriteFile(path, []byte("not-a-number\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFileOffsetStore(path).Load(); err == nil {
		t.Fatal("Load() error = nil, want corrupt state error")
	}
}
