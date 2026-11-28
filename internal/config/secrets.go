package config

import (
	"errors"
	"fmt"
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

var ErrInvalidProfileID = errors.New("invalid profile id")

var profileIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}:[A-Za-z0-9_-]{1,128}$`)

func ValidateProfileID(id string) error {
	if !profileIDPattern.MatchString(id) {
		return fmt.Errorf("metorial: %w %q", ErrInvalidProfileID, id)
	}
	return nil
}

var newSecrets = defaultSecrets

func SetSecretsBackend(secrets Secrets) (restore func()) {
	previous := newSecrets
	newSecrets = func() Secrets { return secrets }
	return func() { newSecrets = previous }
}

func (s Secret) empty() bool {
	return s.AccessToken == "" && s.RefreshToken == ""
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
