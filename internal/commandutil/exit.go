package commandutil

import (
	"errors"
	"fmt"
)

// ExitCodeError makes the CLI exit with Code without printing an error, for
// commands that relay another process's exit status.
type ExitCodeError struct {
	Code int
}

func (e *ExitCodeError) Error() string {
	return fmt.Sprintf("exit status %d", e.Code)
}

func ExitCode(err error) (int, bool) {
	var exitErr *ExitCodeError
	if errors.As(err, &exitErr) {
		return exitErr.Code, true
	}
	return 0, false
}
