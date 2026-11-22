package config

import (
	"os"
	"regexp"
	"runtime"
	"strings"
	"time"
)

const (
	TokenStorageKeychain = "keychain"
	TokenStorageFile     = "file"

	EnvCredentialStorage = "METORIAL_CREDENTIAL_STORAGE"
	KeychainService      = "com.metorial.credentials"
)

type Secret struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type Secrets interface {
	Get(profileID string) (Secret, bool, error)
	Set(profileID string, secret Secret) error
	Delete(profileID string) error
}

func (s Secret) empty() bool {
	return s.AccessToken == "" && s.RefreshToken == ""
}

func (s Secret) equal(other Secret) bool {
	return s.AccessToken == other.AccessToken &&
		s.RefreshToken == other.RefreshToken &&
		s.ExpiresAt.Equal(other.ExpiresAt)
}

func preferFileTokenStorage() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(EnvCredentialStorage))) {
	case TokenStorageFile:
		return true
	case TokenStorageKeychain:
		return false
	}
	return runtime.GOOS != "darwin" && runtime.GOOS != "windows"
}

var unsafeLockName = regexp.MustCompile(`[^A-Za-z0-9._-]`)

func refreshLockPath(profileID string) (string, error) {
	dir, err := DefaultCLIDir()
	if err != nil {
		return "", err
	}
	return dir + string(os.PathSeparator) + "refresh-" + unsafeLockName.ReplaceAllString(profileID, "_") + ".lock", nil
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
