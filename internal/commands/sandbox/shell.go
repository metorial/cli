package sandbox

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/metorial/cli/internal/commandutil"
	"github.com/metorial/cli/internal/sandboxaccess"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type shellFlags struct {
	cwd        string
	env        []string
	tty        bool
	noTTY      bool
	ttlSeconds int
}

func newShellCommand(ctx commandutil.Context) *cobra.Command {
	flags := &shellFlags{}

	command := &cobra.Command{
		Use:   "shell <machine-id> [-- command [args...]]",
		Short: "Open a shell on a sandbox machine",
		Long: strings.TrimSpace(`
Open an interactive shell on a sandbox machine, or run a single command on it.

Without a command this starts a login shell with a pseudo-terminal, like ssh. With a
command after --, the command runs without a pseudo-terminal: stdout and stderr stay
separate, stdin is forwarded, and the CLI exits with the command's exit code.

Dropped connections are resumed automatically; closing the shell ends the session.`),
		Example: strings.TrimSpace(`
  metorial-admin sandbox machines shell sbm_123
  metorial-admin sandbox machines shell sbm_123 --cwd /workspace
  metorial-admin sandbox machines shell sbm_123 -- ls -la /workspace
  metorial-admin sandbox machines shell sbm_123 -t -- htop
  cat script.sh | metorial-admin sandbox machines shell sbm_123 -- bash -s`),
		Args: cobra.MinimumNArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			return runShell(command, ctx, flags, args[0], args[1:])
		},
	}

	command.Flags().StringVar(&flags.cwd, "cwd", "", "Working directory on the machine")
	command.Flags().StringArrayVarP(&flags.env, "env", "e", nil, "Environment variable as KEY=VALUE (repeatable)")
	command.Flags().BoolVarP(&flags.tty, "tty", "t", false, "Force a pseudo-terminal, even when running a command")
	command.Flags().BoolVarP(&flags.noTTY, "no-tty", "T", false, "Disable the pseudo-terminal")
	command.Flags().IntVar(&flags.ttlSeconds, "ttl-seconds", 0, "Lifetime of the access token in seconds (default 3600, max 86400)")

	return command
}

func runShell(command *cobra.Command, ctx commandutil.Context, flags *shellFlags, machineID string, remoteCommand []string) error {
	if !shellSupported {
		return errors.New("metorial: sandbox shells are not available in the browser shell")
	}
	if flags.tty && flags.noTTY {
		return errors.New("metorial: --tty and --no-tty cannot be combined")
	}

	env, err := parseEnv(flags.env)
	if err != nil {
		return err
	}

	stdin := ctx.App.Stdin
	stdout := command.OutOrStdout()
	stderr := command.ErrOrStderr()

	stdinFile, stdinTTY := terminalFile(stdin)
	stdoutFile, stdoutTTY := terminalFile(stdout)

	useTTY := len(remoteCommand) == 0 && stdinTTY && stdoutTTY
	if flags.tty {
		useTTY = true
	}
	if flags.noTTY {
		useTTY = false
	}
	rawMode := useTTY && stdinTTY

	cols, rows := 80, 24
	if stdoutTTY {
		if width, height, err := term.GetSize(int(stdoutFile.Fd())); err == nil {
			cols, rows = width, height
		}
	}

	source, err := newSource(ctx, machineID, sandboxaccess.ScopeShell, flags.ttlSeconds)
	if err != nil {
		return err
	}

	runCtx, stop := context.WithCancel(command.Context())
	defer stop()

	notice := func(message string) {
		if rawMode {
			fmt.Fprintf(stderr, "\r\n[metorial] %s\r\n", message)
		} else {
			fmt.Fprintf(stderr, "metorial: %s\n", message)
		}
	}

	shell, err := sandboxaccess.OpenShell(runCtx, source, sandboxaccess.ShellOptions{
		TTY:     useTTY,
		Cols:    cols,
		Rows:    rows,
		Command: remoteCommand,
		Cwd:     flags.cwd,
		Env:     env,
		Stdout:  stdout,
		Stderr:  stderr,
		Notice:  notice,
	})
	if err != nil {
		return err
	}

	if rawMode {
		state, err := term.MakeRaw(int(stdinFile.Fd()))
		if err != nil {
			_ = shell.Close()
			return fmt.Errorf("metorial: failed to switch the terminal to raw mode: %w", err)
		}
		defer term.Restore(int(stdinFile.Fd()), state)
	}
	if useTTY && stdoutTTY {
		watchResize(runCtx, stdoutFile, func(cols, rows int) {
			_ = shell.Resize(cols, rows)
		})
	}
	if !rawMode {
		forwardSignals(runCtx, func(name string) {
			_ = shell.Signal(name)
		})
	}
	if stdin != nil {
		go pumpStdin(stdin, shell)
	}

	exit, err := shell.Wait()
	if err != nil {
		return err
	}
	if code := exit.ExitCode(); code != 0 {
		return &commandutil.ExitCodeError{Code: code}
	}
	return nil
}

func pumpStdin(stdin io.Reader, shell *sandboxaccess.Shell) {
	buffer := make([]byte, 32<<10)
	for {
		n, err := stdin.Read(buffer)
		if n > 0 {
			if _, writeErr := shell.Write(buffer[:n]); writeErr != nil {
				return
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				_ = shell.CloseStdin()
			}
			return
		}
	}
}

func terminalFile(stream any) (*os.File, bool) {
	file, ok := stream.(*os.File)
	if !ok || file == nil {
		return nil, false
	}
	return file, term.IsTerminal(int(file.Fd()))
}

func parseEnv(values []string) (map[string]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	env := make(map[string]string, len(values))
	for _, value := range values {
		key, val, ok := strings.Cut(value, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return nil, fmt.Errorf("metorial: invalid --env value %q, expected KEY=VALUE", value)
		}
		env[key] = val
	}
	return env, nil
}
