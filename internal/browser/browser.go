package browser

import (
	"fmt"
	"os/exec"
	"runtime"
)

func opener() (string, []string, bool) {
	switch runtime.GOOS {
	case "darwin":
		if _, err := exec.LookPath("open"); err == nil {
			return "open", nil, true
		}
	case "linux":
		if _, err := exec.LookPath("xdg-open"); err == nil {
			return "xdg-open", nil, true
		}
	case "windows":
		if _, err := exec.LookPath("rundll32"); err == nil {
			return "rundll32", []string{"url.dll,FileProtocolHandler"}, true
		}
	}

	return "", nil, false
}

func Supported() bool {
	_, _, ok := opener()
	return ok
}

func Open(target string) error {
	command, args, ok := opener()
	if !ok {
		return fmt.Errorf("opening a browser is not supported on this system")
	}

	args = append(args, target)

	if err := exec.Command(command, args...).Start(); err != nil {
		return fmt.Errorf("failed to open browser: %w", err)
	}

	return nil
}
