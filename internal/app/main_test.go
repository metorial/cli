package app

import (
	"os"
	"testing"

	"github.com/metorial/cli/internal/config"
)

func TestMain(m *testing.M) {
	config.SetSecretsBackend(nil)
	os.Exit(m.Run())
}
