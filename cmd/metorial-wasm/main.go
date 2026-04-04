//go:build js && wasm

package main

import (
	"bytes"
	"os"
	"sync"
	"syscall/js"

	"github.com/metorial/cli/internal/app"
	"github.com/metorial/cli/internal/cli"
	"github.com/metorial/cli/internal/commandutil"
	"github.com/metorial/cli/internal/terminal"
)

var runMu sync.Mutex

func main() {
	commandutil.SetBrowserShellEnabled(true)

	object := js.Global().Get("Object").New()
	object.Set("run", js.FuncOf(runCommand))
	object.Set("completeCommands", js.FuncOf(completeCommands))
	js.Global().Set("metorialBrowser", object)

	select {}
}

func runCommand(this js.Value, args []js.Value) any {
	promise := js.Global().Get("Promise")

	handler := js.FuncOf(func(this js.Value, promiseArgs []js.Value) any {
		resolve := promiseArgs[0]

		go func() {
			runMu.Lock()
			defer runMu.Unlock()

			commandArgs := []string{}
			if len(args) > 0 {
				commandArgs = jsArrayToStrings(args[0])
			}

			envValues := map[string]string{}
			if len(args) > 1 {
				envValues = jsObjectToStringMap(args[1])
			}

			stdout := &bytes.Buffer{}
			stderr := &bytes.Buffer{}
			features := terminal.Features{
				IsTTY:      true,
				HasColor:   true,
				HasUnicode: true,
				Width:      120,
			}

			restoreEnv := applyEnv(envValues)
			exitCode := cli.RunArgs(app.NewWithIO(bytes.NewReader(nil), stdout, stderr, features, features), commandArgs)
			restoreEnv()

			resolve.Invoke(js.ValueOf(map[string]any{
				"exitCode": exitCode,
				"stdout":   stdout.String(),
				"stderr":   stderr.String(),
			}))
		}()

		return nil
	})

	return promise.New(handler)
}

func completeCommands(this js.Value, args []js.Value) any {
	promise := js.Global().Get("Promise")

	handler := js.FuncOf(func(this js.Value, promiseArgs []js.Value) any {
		resolve := promiseArgs[0]

		go func() {
			runMu.Lock()
			defer runMu.Unlock()

			commandArgs := []string{}
			if len(args) > 0 {
				commandArgs = jsArrayToStrings(args[0])
			}

			envValues := map[string]string{}
			if len(args) > 1 {
				envValues = jsObjectToStringMap(args[1])
			}

			stdout := &bytes.Buffer{}
			stderr := &bytes.Buffer{}
			features := terminal.Features{
				IsTTY:      true,
				HasColor:   true,
				HasUnicode: true,
				Width:      120,
			}

			restoreEnv := applyEnv(envValues)
			command, err := cli.NewRootCommand(app.NewWithIO(bytes.NewReader(nil), stdout, stderr, features, features))
			restoreEnv()

			if err != nil {
				resolve.Invoke(js.ValueOf([]any{}))
				return
			}

			target := command
			if len(commandArgs) > 0 {
				found, remaining, findErr := command.Find(commandArgs)
				if findErr == nil && len(remaining) == 0 && found != nil {
					target = found
				}
			}

			values := make([]any, 0, len(target.Commands()))
			for _, child := range target.Commands() {
				if child.Hidden || !child.IsAvailableCommand() || child.IsAdditionalHelpTopicCommand() {
					continue
				}

				values = append(values, child.Name())
			}

			resolve.Invoke(js.ValueOf(values))
		}()

		return nil
	})

	return promise.New(handler)
}

func jsArrayToStrings(value js.Value) []string {
	if value.IsUndefined() || value.IsNull() {
		return nil
	}

	length := value.Length()
	values := make([]string, 0, length)
	for index := 0; index < length; index++ {
		values = append(values, value.Index(index).String())
	}

	return values
}

func jsObjectToStringMap(value js.Value) map[string]string {
	values := map[string]string{}
	if value.IsUndefined() || value.IsNull() {
		return values
	}

	keys := js.Global().Get("Object").Call("keys", value)
	for index := 0; index < keys.Length(); index++ {
		key := keys.Index(index).String()
		values[key] = value.Get(key).String()
	}

	return values
}

func applyEnv(values map[string]string) func() {
	previous := map[string]*string{}

	for key, value := range values {
		if current, ok := os.LookupEnv(key); ok {
			copyValue := current
			previous[key] = &copyValue
		} else {
			previous[key] = nil
		}

		_ = os.Setenv(key, value)
	}

	return func() {
		for key, value := range previous {
			if value == nil {
				_ = os.Unsetenv(key)
				continue
			}

			_ = os.Setenv(key, *value)
		}
	}
}
