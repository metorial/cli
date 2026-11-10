package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/metorial/cli/internal/browser"
	"github.com/metorial/cli/internal/commandutil"
	"github.com/metorial/cli/internal/config"
	"github.com/metorial/cli/internal/fetch"
	"github.com/metorial/cli/internal/output"
	"github.com/spf13/cobra"
)

type integrationSetupSession struct {
	ID     string `json:"id"`
	URL    string `json:"url"`
	Status string `json:"status"`
}

func presentIntegrationSetup(command *cobra.Command, response *fetch.Response) {
	var session integrationSetupSession
	if err := json.Unmarshal(response.Body, &session); err != nil || session.ID == "" {
		return
	}

	fmt.Fprintf(command.ErrOrStderr(), "Check status with %s status %s or wait with %s wait %s. Reuse the same profile and instance options.\n",
		command.CommandPath(), session.ID, command.CommandPath(), session.ID)

	noBrowser, _ := command.Flags().GetBool("no-browser")
	link, err := url.Parse(session.URL)
	if !noBrowser && !commandutil.BrowserShellEnabled() && err == nil && link.Host != "" &&
		link.User == nil && (link.Scheme == "https" || link.Scheme == "http") && browser.Supported() {
		_ = browser.Open(session.URL)
	}
}

func newIntegrationSetupWait(cli commandutil.Context) *cobra.Command {
	var timeout time.Duration
	command := &cobra.Command{
		Use:   "wait <setup-session-id>",
		Short: "Wait for an existing integration setup session to finish",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			id := args[0]
			if id == "" || id == "." || id == ".." || strings.Contains(id, "/") {
				return fmt.Errorf("metorial: invalid integration setup session ID")
			}
			if timeout <= 0 {
				return fmt.Errorf("metorial: --timeout must be positive")
			}
			format, err := output.ParseFormat(cli.Options.Format)
			if err != nil {
				return err
			}

			runtime, err := cli.ResolveRuntime()
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(command.Context(), timeout)
			defer cancel()

			for {
				response, err := fetchIntegrationSetup(ctx, runtime, id)
				if err != nil {
					return fmt.Errorf("metorial: could not wait for integration setup session %s: %w", id, err)
				}
				var session integrationSetupSession
				if err := json.Unmarshal(response.Body, &session); err != nil || session.Status == "" {
					return fmt.Errorf("metorial: could not read status for integration setup session %s", id)
				}
				if session.Status != "pending" {
					if err := output.WriteResponse(command.OutOrStdout(), response, output.RenderOptions{
						Format: format, Colors: cli.App.StdoutFeatures(),
					}); err != nil {
						return err
					}
					if session.Status == "successful" {
						return nil
					}
					return fmt.Errorf("metorial: integration setup session %s is %s", id, session.Status)
				}

				timer := time.NewTimer(2 * time.Second)
				select {
				case <-ctx.Done():
					timer.Stop()
					return fmt.Errorf("metorial: stopped waiting for integration setup session %s: %w", id, ctx.Err())
				case <-timer.C:
				}
			}
		},
	}
	command.Flags().DurationVar(&timeout, "timeout", time.Minute, "Maximum time to wait for authorization")
	commandutil.ConfigureCommand(command)
	return command
}

// The SDK bounds HTTP requests but its raw request API does not accept a context.
func fetchIntegrationSetup(ctx context.Context, runtime config.Runtime, id string) (*fetch.Response, error) {
	type result struct {
		response *fetch.Response
		err      error
	}
	done := make(chan result, 1)
	go func() {
		response, err := fetch.Do(runtime, fetch.Request{
			Method: http.MethodGet,
			Target: "/integration-setup-sessions/" + url.PathEscape(id),
		})
		done <- result{response: response, err: err}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case response := <-done:
		return response.response, response.err
	}
}
