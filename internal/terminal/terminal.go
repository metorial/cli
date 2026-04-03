package terminal

import (
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

type Features struct {
	IsTTY      bool
	HasColor   bool
	HasUnicode bool
	Width      int
}

func Detect(file *os.File) Features {
	isTTY := term.IsTerminal(int(file.Fd()))
	width := 0
	if isTTY {
		if detectedWidth, _, err := term.GetSize(int(file.Fd())); err == nil {
			width = detectedWidth
		}
	}

	return Features{
		IsTTY:      isTTY,
		HasColor:   isTTY && os.Getenv("NO_COLOR") == "",
		HasUnicode: os.Getenv("LANG") != "C",
		Width:      width,
	}
}

func DetectWriter(writer io.Writer) Features {
	file, ok := writer.(*os.File)
	if !ok {
		return Features{
			HasUnicode: os.Getenv("LANG") != "C",
		}
	}

	return Detect(file)
}

func SupportsHyperlinks() bool {
	if os.Getenv("FORCE_HYPERLINK") != "" {
		return true
	}

	if os.Getenv("NO_COLOR") != "" {
		return false
	}

	termProgram := os.Getenv("TERM_PROGRAM")
	switch termProgram {
	case "iTerm.app", "WezTerm", "vscode":
		return true
	}

	vt := strings.ToLower(os.Getenv("VTE_VERSION"))
	if vt >= "5000" {
		return true
	}

	return false
}

func Link(label, href string) string {
	if SupportsHyperlinks() {
		return fmt.Sprintf("\x1b]8;;%s\x1b\\%s\x1b]8;;\x1b\\", href, label)
	}

	return href
}
