package main

import (
	"os"
	"path/filepath"
)

type appPaths struct {
	configDir   string
	cacheDir    string
	userDataDir string
	logDir      string
}

func resolveAppPaths(home string) appPaths {
	return appPaths{
		configDir: migrateDataDir(
			filepath.Join(home, ".config", "reels"),
			filepath.Join(home, ".config", "termireels"),
		),
		cacheDir: migrateDataDir(
			filepath.Join(home, ".cache", "reels"),
			filepath.Join(home, ".cache", "termireels"),
		),
		userDataDir: filepath.Join(migrateDataDir(
			filepath.Join(home, ".local", "share", "reels"),
			filepath.Join(home, ".local", "share", "termireels"),
		), "chrome-data"),
		logDir: migrateDataDir(
			filepath.Join(home, ".local", "state", "reels"),
			filepath.Join(home, ".local", "state", "termireels"),
		),
	}
}

// migrateDataDir performs a no-overwrite rename. If the destination already
// exists, or migration is impossible, it never merges or deletes data.
func migrateDataDir(legacy, current string) string {
	if _, err := os.Stat(current); err == nil {
		return current
	}
	if _, err := os.Stat(legacy); err != nil {
		return current
	}
	if err := os.MkdirAll(filepath.Dir(current), 0755); err != nil {
		return legacy
	}
	if err := os.Rename(legacy, current); err != nil {
		return legacy
	}
	return current
}
