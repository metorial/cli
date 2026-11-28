package config

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	SetSecretsBackend(nil)
	os.Exit(m.Run())
}
