package cli

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"unicode"

	"github.com/metorial/cli/internal/app"
	"github.com/metorial/cli/internal/fetch"
	"github.com/metorial/cli/internal/output"
	"github.com/metorial/cli/internal/resourcecmd"
	"github.com/spf13/cobra"
)

func addPublicResourceCommands(
	root *cobra.Command,
	application *app.App,
	rootOptions *rootOptions,
	plan []resourcecmd.ResourceGroup,
) error {
	for _, group := range plan {
		for _, resource := range group.Resources {
			if resource.Plural == "instance" {
				continue
			}

			command, err := resourcecmd.NewResourceCommand(resource, func(resource resourcecmd.ResourceSpec, operation resourcecmd.OperationSpec) (*cobra.Command, error) {
				return newPublicResourceAction(application, rootOptions, resource, operation)
			})
				if err != nil {
					return err
				}

				command.Annotations = map[string]string{
					"metorial:command-category": commandCategoryResource,
				}
				root.AddCommand(command)
			}
		}

	return nil
}

func newPublicResourceAction(
	application *app.App,
	rootOptions *rootOptions,
	resource resourcecmd.ResourceSpec,
	operation resourcecmd.OperationSpec,
) (*cobra.Command, error) {
	use := operation.Use
	if use == "" {
		use = defaultResourceOperationUse(operation)
	}

	short := operation.Short
	if strings.TrimSpace(short) == "" {
		short = fmt.Sprintf("%s %s", operation.Name, resource.Plural)
	}

	command := &cobra.Command{
		Use:   use,
		Short: short,
		Args:  resourceOperationArgs(operation),
		Annotations: map[string]string{
			"metorial:command-category": commandCategoryResource,
		},
		RunE: func(command *cobra.Command, args []string) error {
			runtime, err := application.ResolveConfig(rootOptions.apiKey, rootOptions.apiHost, rootOptions.profile, rootOptions.instance)
			if err != nil {
				return err
			}

			options, err := buildResourceFetchOptions(command, resource, operation, args)
			if err != nil {
				return err
			}

			format, err := output.ParseFormat(rootOptions.format)
			if err != nil {
				return err
			}

			response, requestErr := fetch.Execute(runtime, options, application.Stdin)
			if response != nil {
				writer := command.OutOrStdout()
				if requestErr != nil {
					writer = command.ErrOrStderr()
				}
				if err := output.WriteResponse(writer, response, output.RenderOptions{
					Format: format,
				}); err != nil {
					return err
				}
			}

			return requestErr
		},
	}

	addOperationFlags(command, operation)

	return command, nil
}

func addOperationFlags(command *cobra.Command, operation resourcecmd.OperationSpec) {
	for _, flag := range operation.Flags {
		switch flag.Type {
		case resourcecmd.FlagString:
			command.Flags().StringP(flag.Name, flag.Shorthand, "", flag.Usage)
		case resourcecmd.FlagBool:
			command.Flags().BoolP(flag.Name, flag.Shorthand, false, flag.Usage)
		case resourcecmd.FlagInt:
			command.Flags().IntP(flag.Name, flag.Shorthand, 0, flag.Usage)
		case resourcecmd.FlagFloat:
			command.Flags().Float64P(flag.Name, flag.Shorthand, 0, flag.Usage)
		case resourcecmd.FlagStringSlice:
			command.Flags().StringSliceP(flag.Name, flag.Shorthand, nil, flag.Usage)
		default:
			continue
		}

		if flag.Required {
			_ = command.MarkFlagRequired(flag.Name)
		}
	}

	if operationUsesBody(operation) {
		command.Flags().String("body", "", "Inline JSON object to merge into the request body")
		command.Flags().String("body-file", "", "Read a JSON object from a file and merge it into the request body")
	}
}

func buildResourceFetchOptions(
	command *cobra.Command,
	resource resourcecmd.ResourceSpec,
	operation resourcecmd.OperationSpec,
	args []string,
) (fetch.Options, error) {
	method, err := resourceOperationMethod(operation.Name)
	if err != nil {
		return fetch.Options{}, err
	}

	target, err := buildResourceTarget(command, resource, operation, args)
	if err != nil {
		return fetch.Options{}, err
	}

	options := fetch.Options{
		Method: method,
		Target: target,
	}

	if operationUsesBody(operation) {
		body, err := buildResourceBody(command, operation)
		if err != nil {
			return fetch.Options{}, err
		}
		if len(body) > 0 {
			payload, err := json.Marshal(body)
			if err != nil {
				return fetch.Options{}, fmt.Errorf("metorial: failed to encode request body: %w", err)
			}
			options.Data = string(payload)
		}
	}

	return options, nil
}

func buildResourceTarget(
	command *cobra.Command,
	resource resourcecmd.ResourceSpec,
	operation resourcecmd.OperationSpec,
	args []string,
) (string, error) {
	path, err := resourceOperationPath(resource, operation, args)
	if err != nil {
		return "", err
	}

	values := url.Values{}
	for _, flag := range operation.Flags {
		if !strings.HasPrefix(flag.Target, "params.") {
			continue
		}
		if !command.Flags().Changed(flag.Name) {
			continue
		}

		switch flag.Type {
		case resourcecmd.FlagString:
			value, err := command.Flags().GetString(flag.Name)
			if err != nil {
				return "", err
			}
			values.Add(flag.Name, value)
		case resourcecmd.FlagBool:
			value, err := command.Flags().GetBool(flag.Name)
			if err != nil {
				return "", err
			}
			values.Add(flag.Name, strconv.FormatBool(value))
		case resourcecmd.FlagInt:
			value, err := command.Flags().GetInt(flag.Name)
			if err != nil {
				return "", err
			}
			values.Add(flag.Name, strconv.Itoa(value))
		case resourcecmd.FlagFloat:
			value, err := command.Flags().GetFloat64(flag.Name)
			if err != nil {
				return "", err
			}
			values.Add(flag.Name, strconv.FormatFloat(value, 'f', -1, 64))
		case resourcecmd.FlagStringSlice:
			valuesSlice, err := command.Flags().GetStringSlice(flag.Name)
			if err != nil {
				return "", err
			}
			for _, value := range valuesSlice {
				values.Add(flag.Name, value)
			}
		}
	}

	if err := applyDefaultListLimit(command, operation, values); err != nil {
		return "", err
	}

	if encoded := values.Encode(); encoded != "" {
		return path + "?" + encoded, nil
	}

	return path, nil
}

func buildResourceBody(command *cobra.Command, operation resourcecmd.OperationSpec) (map[string]any, error) {
	body, err := readExplicitBody(command)
	if err != nil {
		return nil, err
	}

	for _, flag := range operation.Flags {
		if !strings.HasPrefix(flag.Target, "body.") {
			continue
		}
		if !command.Flags().Changed(flag.Name) {
			continue
		}

		key := bodyFlagKey(flag.Target)
		switch flag.Type {
		case resourcecmd.FlagString:
			value, err := command.Flags().GetString(flag.Name)
			if err != nil {
				return nil, err
			}
			body[key] = value
		case resourcecmd.FlagBool:
			value, err := command.Flags().GetBool(flag.Name)
			if err != nil {
				return nil, err
			}
			body[key] = value
		case resourcecmd.FlagInt:
			value, err := command.Flags().GetInt(flag.Name)
			if err != nil {
				return nil, err
			}
			body[key] = value
		case resourcecmd.FlagFloat:
			value, err := command.Flags().GetFloat64(flag.Name)
			if err != nil {
				return nil, err
			}
			body[key] = value
		case resourcecmd.FlagStringSlice:
			value, err := command.Flags().GetStringSlice(flag.Name)
			if err != nil {
				return nil, err
			}
			body[key] = value
		}
	}

	return body, nil
}

func applyDefaultListLimit(command *cobra.Command, operation resourcecmd.OperationSpec, values url.Values) error {
	if operation.Name != resourcecmd.OperationList {
		return nil
	}

	for _, flag := range operation.Flags {
		if flag.Name != "limit" || flag.Target != "params.Limit" {
			continue
		}
		if command.Flags().Changed("limit") {
			return nil
		}

		values.Add("limit", "5")
		return nil
	}

	return nil
}

func readExplicitBody(command *cobra.Command) (map[string]any, error) {
	inlineBody, err := command.Flags().GetString("body")
	if err != nil {
		return nil, err
	}

	bodyFile, err := command.Flags().GetString("body-file")
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(inlineBody) != "" && strings.TrimSpace(bodyFile) != "" {
		return nil, fmt.Errorf("metorial: use either --body or --body-file, not both")
	}

	if strings.TrimSpace(inlineBody) == "" && strings.TrimSpace(bodyFile) == "" {
		return map[string]any{}, nil
	}

	var payload []byte
	if strings.TrimSpace(bodyFile) != "" {
		payload, err = os.ReadFile(bodyFile)
		if err != nil {
			return nil, fmt.Errorf("metorial: failed to read body file %q: %w", bodyFile, err)
		}
	} else {
		payload = []byte(inlineBody)
	}

	var body map[string]any
	if err := json.Unmarshal(payload, &body); err != nil {
		return nil, fmt.Errorf("metorial: request body must be a JSON object: %w", err)
	}

	if body == nil {
		body = map[string]any{}
	}

	return body, nil
}

func bodyFlagKey(target string) string {
	field := strings.TrimPrefix(target, "body.")
	if field == target {
		return target
	}
	return camelToSnake(field)
}

func resourceOperationPath(resource resourcecmd.ResourceSpec, operation resourcecmd.OperationSpec, args []string) (string, error) {
	switch operation.Name {
	case resourcecmd.OperationList, resourcecmd.OperationCreate:
		return "/" + resource.Plural, nil
	case resourcecmd.OperationGet, resourcecmd.OperationUpdate, resourcecmd.OperationDelete:
		if resource.Plural == "instance" {
			return "/instance", nil
		}
		if len(args) == 0 {
			return "", fmt.Errorf("metorial: missing required resource identifier")
		}
		return "/" + resource.Plural + "/" + args[0], nil
	case resourcecmd.OperationGetSchema:
		if resource.Plural == "provider-configs" {
			return "/provider-config-schema", nil
		}
	}

	return "", fmt.Errorf("metorial: %s %s is not implemented yet", resource.Plural, operation.Name)
}

func resourceOperationMethod(name resourcecmd.OperationName) (string, error) {
	switch name {
	case resourcecmd.OperationList, resourcecmd.OperationGet, resourcecmd.OperationGetSchema:
		return "GET", nil
	case resourcecmd.OperationCreate:
		return "POST", nil
	case resourcecmd.OperationUpdate:
		return "PATCH", nil
	case resourcecmd.OperationDelete:
		return "DELETE", nil
	default:
		return "", fmt.Errorf("metorial: operation %q is not implemented yet", name)
	}
}

func operationUsesBody(operation resourcecmd.OperationSpec) bool {
	switch operation.Name {
	case resourcecmd.OperationCreate, resourcecmd.OperationUpdate:
		return true
	default:
		return false
	}
}

func resourceOperationArgs(operation resourcecmd.OperationSpec) cobra.PositionalArgs {
	required := 0
	for _, arg := range operation.Args {
		if arg.Required {
			required++
		}
	}

	return func(command *cobra.Command, args []string) error {
		if len(args) < required {
			return cobra.MinimumNArgs(required)(command, args)
		}
		if len(args) > len(operation.Args) {
			return cobra.MaximumNArgs(len(operation.Args))(command, args)
		}
		return nil
	}
}

func defaultResourceOperationUse(operation resourcecmd.OperationSpec) string {
	parts := []string{string(operation.Name)}
	for _, arg := range operation.Args {
		name := strings.TrimSpace(arg.Name)
		if name == "" {
			continue
		}
		if arg.Required {
			parts = append(parts, "<"+name+">")
		} else {
			parts = append(parts, "["+name+"]")
		}
	}
	return strings.Join(parts, " ")
}

func camelToSnake(value string) string {
	if value == "" {
		return ""
	}

	var builder strings.Builder
	for i, r := range value {
		if unicode.IsUpper(r) {
			if i > 0 {
				builder.WriteByte('_')
			}
			builder.WriteRune(unicode.ToLower(r))
			continue
		}
		builder.WriteRune(r)
	}

	return builder.String()
}
