package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateDataDirMovesLegacyWithoutOverwriting(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, "old", "reels")
	current := filepath.Join(root, "new", "termireels")
	if err := os.MkdirAll(legacy, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "profile"), []byte("kept"), 0600); err != nil {
		t.Fatal(err)
	}

	if got := migrateDataDir(legacy, current); got != current {
		t.Fatalf("migrateDataDir() = %q, want %q", got, current)
	}
	data, err := os.ReadFile(filepath.Join(current, "profile"))
	if err != nil || string(data) != "kept" {
		t.Fatalf("migrated profile = %q, %v", data, err)
	}

	otherLegacy := filepath.Join(root, "other-old")
	if err := os.MkdirAll(otherLegacy, 0755); err != nil {
		t.Fatal(err)
	}
	if got := migrateDataDir(otherLegacy, current); got != current {
		t.Fatalf("existing destination was not preferred: %q", got)
	}
	if _, err := os.Stat(otherLegacy); err != nil {
		t.Fatalf("legacy was modified despite existing destination: %v", err)
	}
}
