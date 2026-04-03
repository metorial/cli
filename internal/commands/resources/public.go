package resources

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"unicode"

	"github.com/metorial/cli/internal/app"
	"github.com/metorial/cli/internal/commandutil"
	"github.com/metorial/cli/internal/fetch"
	"github.com/metorial/cli/internal/output"
	"github.com/metorial/cli/internal/resourcecmd"
	"github.com/spf13/cobra"
)

type rootOptionsView struct {
	apiKey   string
	apiHost  string
	instance string
	profile  string
	format   string
}

func newRootOptionsView(options *commandutil.RootOptions) *rootOptionsView {
	if options == nil {
		return &rootOptionsView{}
	}

	return &rootOptionsView{
		apiKey:   options.APIKey,
		apiHost:  options.APIHost,
		instance: options.Instance,
		profile:  options.Profile,
		format:   options.Format,
	}
}

func AddPublicCommands(
	root *cobra.Command,
	ctx commandutil.Context,
) error {
	application := ctx.App
	rootOptions := newRootOptionsView(ctx.Options)
	plan := resourcecmd.PublicResourcePlan()

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

			commandutil.SetCommandCategory(command, commandutil.CommandCategoryResource)
			commandutil.ConfigureCommand(command)
			root.AddCommand(command)
		}
	}

	return nil
}

func newPublicResourceAction(
	application *app.App,
	rootOptions *rootOptionsView,
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
		Use:     use,
		Short:   short,
		Long:    operation.Long,
		Example: strings.Join(operation.Examples, "\n"),
		Args:    resourceOperationArgs(operation),
		Annotations: map[string]string{
			"metorial:command-category": commandutil.CommandCategoryResource,
			"metorial:arguments":        formatArgumentSection(operation.Args),
			"metorial:see-also":         formatSeeAlsoSection(operation.SeeAlso),
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
				if err := transformResourceResponse(response, resource, operation); err != nil {
					return err
				}
				writer := command.OutOrStdout()
				if requestErr != nil {
					writer = command.ErrOrStderr()
				}
				if err := output.WriteResponse(writer, response, output.RenderOptions{
					Format: format,
					Colors: application.StdoutFeatures(),
				}); err != nil {
					return err
				}
			}

			return requestErr
		},
	}
	commandutil.ConfigureCommand(command)

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
		case resourcecmd.FlagJSON, resourcecmd.FlagJSONFile:
			command.Flags().StringP(flag.Name, flag.Shorthand, "", flag.Usage)
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
		body, err := buildResourceBody(command, resource, operation, args)
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
	if resource.Plural == "providers" && operation.Name == resourcecmd.OperationGet {
		if len(args) == 0 {
			return "", fmt.Errorf("metorial: missing required provider identifier")
		}
		values.Add("provider_id", args[0])
		values.Add("limit", "1")
	}

	for index, arg := range operation.Args {
		if !strings.HasPrefix(arg.Target, "params.") {
			continue
		}
		if index >= len(args) {
			continue
		}

		values.Add(paramFlagKey(arg.Target), args[index])
	}

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
			values.Add(paramFlagKey(flag.Target), value)
		case resourcecmd.FlagBool:
			value, err := command.Flags().GetBool(flag.Name)
			if err != nil {
				return "", err
			}
			values.Add(paramFlagKey(flag.Target), strconv.FormatBool(value))
		case resourcecmd.FlagInt:
			value, err := command.Flags().GetInt(flag.Name)
			if err != nil {
				return "", err
			}
			values.Add(paramFlagKey(flag.Target), strconv.Itoa(value))
		case resourcecmd.FlagFloat:
			value, err := command.Flags().GetFloat64(flag.Name)
			if err != nil {
				return "", err
			}
			values.Add(paramFlagKey(flag.Target), strconv.FormatFloat(value, 'f', -1, 64))
		case resourcecmd.FlagStringSlice:
			valuesSlice, err := command.Flags().GetStringSlice(flag.Name)
			if err != nil {
				return "", err
			}
			for _, value := range valuesSlice {
				values.Add(paramFlagKey(flag.Target), value)
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

func buildResourceBody(command *cobra.Command, resource resourcecmd.ResourceSpec, operation resourcecmd.OperationSpec, args []string) (map[string]any, error) {
	body, err := readExplicitBody(command)
	if err != nil {
		return nil, err
	}

	for index, arg := range operation.Args {
		if !strings.HasPrefix(arg.Target, "body.") {
			continue
		}
		if index >= len(args) {
			continue
		}

		body[bodyFlagKey(arg.Target)] = args[index]
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
		case resourcecmd.FlagJSON, resourcecmd.FlagJSONFile:
			value, err := readJSONFlagValue(command, flag)
			if err != nil {
				return nil, err
			}
			body[key] = value
		}
	}

	if err := applySpecialCreateBody(command, resource, operation, body); err != nil {
		return nil, err
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

		values.Add("limit", "15")
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

func paramFlagKey(target string) string {
	field := strings.TrimPrefix(target, "params.")
	if field == target {
		return target
	}

	return camelToSnake(field)
}

func resourceOperationPath(resource resourcecmd.ResourceSpec, operation resourcecmd.OperationSpec, args []string) (string, error) {
	if resource.Plural == "providers" {
		return providerOperationPath(operation, args)
	}

	pathPlural := resource.APIPathPlural()

	switch operation.Name {
	case resourcecmd.OperationList, resourcecmd.OperationCreate:
		return "/" + pathPlural, nil
	case resourcecmd.OperationGet, resourcecmd.OperationUpdate, resourcecmd.OperationDelete:
		if resource.Plural == "instance" {
			return "/instance", nil
		}
		if len(args) == 0 {
			return "", fmt.Errorf("metorial: missing required resource identifier")
		}
		return "/" + pathPlural + "/" + args[0], nil
	case resourcecmd.OperationGetSchema:
		if pathPlural == "provider-configs" {
			return "/provider-config-schema", nil
		}
	}

	return "", fmt.Errorf("metorial: %s %s is not implemented yet", resource.Plural, operation.Name)
}

func providerOperationPath(operation resourcecmd.OperationSpec, args []string) (string, error) {
	switch operation.Name {
	case resourcecmd.OperationList:
		return "/provider-listings", nil
	case resourcecmd.OperationGet:
		if len(args) == 0 {
			return "", fmt.Errorf("metorial: missing required provider identifier")
		}
		return "/provider-listings", nil
	default:
		return "", fmt.Errorf("metorial: providers %s is not implemented yet", operation.Name)
	}
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
			if command != nil {
				_ = command.Help()
			}
			return cobra.MinimumNArgs(required)(command, args)
		}
		if len(args) > len(operation.Args) {
			if command != nil {
				_ = command.Help()
			}
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

func formatArgumentSection(arguments []resourcecmd.ArgumentSpec) string {
	if len(arguments) == 0 {
		return ""
	}

	lines := make([]string, 0, len(arguments))
	for _, argument := range arguments {
		description := strings.TrimSpace(argument.Description)
		if description == "" {
			description = "Argument"
		}
		if argument.Required {
			description = "Required. " + description
		} else {
			description = "Optional. " + description
		}
		lines = append(lines, fmt.Sprintf("  %-16s %s", argument.Name, description))
	}

	return strings.Join(lines, "\n")
}

func formatSeeAlsoSection(values []string) string {
	if len(values) == 0 {
		return ""
	}

	lines := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		lines = append(lines, "  "+value)
	}

	return strings.Join(lines, "\n")
}

func readJSONFlagValue(command *cobra.Command, flag resourcecmd.FlagSpec) (any, error) {
	raw, err := command.Flags().GetString(flag.Name)
	if err != nil {
		return nil, err
	}

	var payload []byte
	switch flag.Type {
	case resourcecmd.FlagJSON:
		payload = []byte(raw)
	case resourcecmd.FlagJSONFile:
		payload, err = os.ReadFile(raw)
		if err != nil {
			return nil, fmt.Errorf("metorial: failed to read %s %q: %w", flag.Name, raw, err)
		}
	default:
		return nil, fmt.Errorf("metorial: unsupported JSON flag type %q", flag.Type)
	}

	var value any
	if err := json.Unmarshal(payload, &value); err != nil {
		return nil, fmt.Errorf("metorial: %s must be valid JSON: %w", flag.Name, err)
	}

	return value, nil
}

func transformResourceResponse(response *fetch.Response, resource resourcecmd.ResourceSpec, operation resourcecmd.OperationSpec) error {
	if response == nil || resource.Plural != "providers" {
		return nil
	}

	switch operation.Name {
	case resourcecmd.OperationList:
		var payload map[string]any
		if err := json.Unmarshal(response.Body, &payload); err != nil {
			return nil
		}

		items, ok := payload["items"].([]any)
		if !ok {
			return nil
		}

		providers := make([]any, 0, len(items))
		for _, item := range items {
			object, ok := item.(map[string]any)
			if !ok {
				continue
			}
			provider, ok := object["provider"]
			if !ok {
				continue
			}
			providers = append(providers, provider)
		}
		payload["items"] = providers

		body, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		response.Body = body
	case resourcecmd.OperationGet:
		var payload map[string]any
		if err := json.Unmarshal(response.Body, &payload); err != nil {
			return nil
		}

		if provider, ok := payload["provider"]; ok {
			body, err := json.Marshal(provider)
			if err != nil {
				return err
			}
			response.Body = body
			return nil
		}

		items, ok := payload["items"].([]any)
		if !ok || len(items) == 0 {
			return fmt.Errorf("metorial: provider not found")
		}

		first, ok := items[0].(map[string]any)
		if !ok {
			return nil
		}
		provider, ok := first["provider"]
		if !ok {
			return nil
		}

		body, err := json.Marshal(provider)
		if err != nil {
			return err
		}
		response.Body = body
	}

	return nil
}

func applySpecialCreateBody(command *cobra.Command, resource resourcecmd.ResourceSpec, operation resourcecmd.OperationSpec, body map[string]any) error {
	if operation.Name != resourcecmd.OperationCreate {
		return nil
	}

	switch resource.Plural {
	case "sessions":
		providers, err := readProviderSpecs(command)
		if err != nil {
			return err
		}
		if len(providers) > 0 {
			body["providers"] = providers
		}
	case "session-templates":
		providers, err := readProviderSpecs(command)
		if err != nil {
			return err
		}
		if len(providers) > 0 {
			body["providers"] = providers
		}
	}

	return nil
}

func readProviderSpecs(command *cobra.Command) ([]any, error) {
	if command.Flags().Lookup("provider") == nil {
		return nil, nil
	}

	inlineProviders, err := command.Flags().GetStringSlice("provider")
	if err != nil {
		return nil, err
	}
	providerFile, err := command.Flags().GetString("provider-file")
	if err != nil {
		return nil, err
	}

	providers := make([]any, 0, len(inlineProviders))
	for _, raw := range inlineProviders {
		value, err := parseProviderSpec(raw)
		if err != nil {
			return nil, err
		}
		providers = append(providers, value)
	}

	if strings.TrimSpace(providerFile) != "" {
		payload, err := os.ReadFile(providerFile)
		if err != nil {
			return nil, fmt.Errorf("metorial: failed to read provider file %q: %w", providerFile, err)
		}

		var value []any
		if err := json.Unmarshal(payload, &value); err != nil {
			return nil, fmt.Errorf("metorial: provider-file must contain a JSON array: %w", err)
		}
		providers = append(providers, value...)
	}

	return providers, nil
}

func parseProviderSpec(raw string) (map[string]any, error) {
	provider := map[string]any{}

	for _, part := range strings.Split(raw, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			return nil, fmt.Errorf("metorial: provider must use key=value pairs, got %q", raw)
		}

		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			return nil, fmt.Errorf("metorial: provider must use non-empty key=value pairs, got %q", raw)
		}

		switch key {
		case "deployment":
			provider["provider_deployment_id"] = value
		case "config":
			provider["provider_config_id"] = value
		case "config-vault":
			provider["provider_config_vault_id"] = value
		case "auth-config":
			provider["provider_auth_config_id"] = value
		default:
			return nil, fmt.Errorf("metorial: unsupported provider key %q", key)
		}
	}

	return provider, nil
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
