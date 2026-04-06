package update

import (
	"strings"
	"testing"
)

func TestDetectInstallFromPathsInfersInstallScriptSymlinkMetadata(t *testing.T) {
	t.Parallel()

	install := detectInstallFromPaths(
		"/usr/local/bin/metorial",
		"/home/demo/.metorial/cli/metorial",
	)

	if install.Method != installMethodInstallSH {
		t.Fatalf("detectInstallFromPaths() method = %q, want %q", install.Method, installMethodInstallSH)
	}

	if install.BinDir != "/usr/local/bin" {
		t.Fatalf("detectInstallFromPaths() bin dir = %q, want %q", install.BinDir, "/usr/local/bin")
	}

	if install.SymlinkPath != "/usr/local/bin/metorial" {
		t.Fatalf("detectInstallFromPaths() symlink path = %q, want %q", install.SymlinkPath, "/usr/local/bin/metorial")
	}

	if install.ManagedBinaryPath != "/home/demo/.metorial/cli/metorial" {
		t.Fatalf("detectInstallFromPaths() managed binary path = %q, want %q", install.ManagedBinaryPath, "/home/demo/.metorial/cli/metorial")
	}
}

func TestInstallSHUpgradeCommandUsesBinDir(t *testing.T) {
	t.Parallel()

	command, args, supported := installSHUpgradeCommand(InstallInfo{
		Method: installMethodInstallSH,
		BinDir: "/usr/local/bin",
	})
	if !supported {
		t.Fatal("installSHUpgradeCommand() supported = false, want true")
	}

	if command != "sh" {
		t.Fatalf("installSHUpgradeCommand() command = %q, want %q", command, "sh")
	}

	if len(args) != 2 {
		t.Fatalf("installSHUpgradeCommand() args len = %d, want 2", len(args))
	}

	if !strings.Contains(args[1], `METORIAL_CLI_BIN_DIR="/usr/local/bin"`) {
		t.Fatalf("installSHUpgradeCommand() args = %q, want bin dir export", args[1])
	}

	if !strings.Contains(args[1], `https://cli.metorial.com/install.sh`) {
		t.Fatalf("installSHUpgradeCommand() args = %q, want install script URL", args[1])
	}
}
