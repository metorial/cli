//go:build !js

package config

import (
	"errors"
	"testing"
)

func TestKeyringRejectsInvalidProfileIDs(t *testing.T) {
	for _, call := range []func() error{
		func() error { _, _, err := (keyringSecrets{}).Get("ins_1:cpr 1"); return err },
		func() error { return (keyringSecrets{}).Set("ins_1:cpr 1", Secret{AccessToken: "a"}) },
		func() error { return (keyringSecrets{}).Delete("ins_1:cpr 1") },
	} {
		if err := call(); !errors.Is(err, ErrInvalidProfileID) {
			t.Fatalf("keyring call error = %v, want ErrInvalidProfileID", err)
		}
	}
}
