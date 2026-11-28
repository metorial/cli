package config

import (
	"path/filepath"
	"regexp"
)

var unsafeLockName = regexp.MustCompile(`[^A-Za-z0-9._-]`)

func refreshLockPath(profileID string) (string, error) {
	dir, err := DefaultCLIDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "refresh-"+unsafeLockName.ReplaceAllString(profileID, "_")+".lock"), nil
}

// LockRefresh serializes token refreshes for one profile across every process sharing the
// config file (the CLI and Metorial Desktop), since a refresh rotates the shared refresh token.
func LockRefresh(profileID string) (func(), error) {
	path, err := refreshLockPath(profileID)
	if err != nil {
		return nil, err
	}
	return lockPath(path, refreshLockTimeout)
}
