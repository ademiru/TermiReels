package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreatorProviderScriptFollowsInstalledExecutable(t *testing.T) {
	root := t.TempDir()
	versionDir := filepath.Join(root, "versions", "1.0.0")
	script := filepath.Join(versionDir, "creator-provider", "dist", "index.js")
	if err := os.MkdirAll(filepath.Dir(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(script, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(versionDir, "termireels")
	if err := os.WriteFile(executable, []byte(""), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "termireels")
	if err := os.Symlink(executable, link); err != nil {
		t.Fatal(err)
	}
	if got := creatorProviderScriptForExecutable(link); got != script {
		t.Fatalf("creator provider script = %q, want %q", got, script)
	}
}

func TestCreatorProviderScriptFallsBackForSourceBuild(t *testing.T) {
	got := creatorProviderScriptForExecutable(filepath.Join(t.TempDir(), "termireels"))
	if got != "creator-provider/dist/index.js" {
		t.Fatalf("creator provider fallback = %q", got)
	}
}
