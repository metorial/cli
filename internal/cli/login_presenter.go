package cli

import (
	"fmt"
	"io"

	"github.com/metorial/cli/internal/output"
	"github.com/metorial/cli/internal/terminal"
)

func renderLoginWaitingScreenWithFeatures(writer io.Writer, features terminal.Features) {
	colors := terminal.NewColorizer(features)

	_, _ = fmt.Fprintln(writer, colors.Bold("Welcome to the Metorial CLI!"))
	_, _ = fmt.Fprintln(writer)
}

func renderLoginAuthScreen(writer io.Writer, features terminal.Features, authURL, userCode string, browserOpenSupported bool) {
	colors := terminal.NewColorizer(features)

	_, _ = fmt.Fprintln(writer, colors.Bold("Welcome to the Metorial CLI!"))
	_, _ = fmt.Fprintln(writer)
	_, _ = fmt.Fprintln(writer, colors.Notice("Please sign in to your Metorial account."))
	_, _ = fmt.Fprintln(writer)
	_ = output.RenderBox(writer, []string{
		authURL,
		"Verification Code: " + userCode,
	}, output.BoxOptions{
		MaxWidth: features.Width,
		Unicode:  features.HasUnicode,
	})
	_, _ = fmt.Fprintln(writer)

	if browserOpenSupported {
		_, _ = fmt.Fprintln(writer, colors.Muted("Opening browser for Metorial authentication."))
	}
}

func renderLoginSuccess(writer io.Writer, features terminal.Features) {
	colors := terminal.NewColorizer(features)

	_, _ = fmt.Fprintln(writer, colors.Success("Logged in to Metorial successfully."))
	_, _ = fmt.Fprintln(writer)
	_, _ = fmt.Fprintln(writer, colors.Bold("Get started by running `metorial`."))
}
