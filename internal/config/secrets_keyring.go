//go:build !js

package config

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/zalando/go-keyring"
)

type keyringSecrets struct{}

var newSecrets = func() Secrets {
	if testing.Testing() {
		return nil
	}
	return keyringSecrets{}
}

func (keyringSecrets) Get(profileID string) (Secret, bool, error) {
	raw, err := keyring.Get(KeychainService, profileID)
	if errors.Is(err, keyring.ErrNotFound) {
		return Secret{}, false, nil
	}
	if err != nil {
		return Secret{}, false, err
	}

	secret := Secret{}
	if err := json.Unmarshal([]byte(raw), &secret); err != nil {
		return Secret{}, false, err
	}
	return secret, true, nil
}

func (keyringSecrets) Set(profileID string, secret Secret) error {
	raw, err := json.Marshal(secret)
	if err != nil {
		return err
	}
	return keyring.Set(KeychainService, profileID, string(raw))
}

func (keyringSecrets) Delete(profileID string) error {
	err := keyring.Delete(KeychainService, profileID)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}
