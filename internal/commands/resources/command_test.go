package resources

import (
	"bytes"
	"strings"
	"testing"

	"github.com/metorial/cli/internal/app"
	"github.com/metorial/cli/internal/commandutil"
	"github.com/metorial/cli/internal/resourcecmd"
	"github.com/spf13/cobra"
)

func sampleListOperation() resourcecmd.OperationSpec {
	return resourcecmd.OperationSpec{
		Name:   resourcecmd.OperationList,
		Method: "GET",
		Path:   "/providers",
		Short:  "List providers",
		Flags: []resourcecmd.FlagSpec{
			{Name: "limit", Type: resourcecmd.FlagFloat, Target: "query.limit", Usage: "Limit"},
			{Name: "id", Type: resourcecmd.FlagStringSlice, Target: "query.id", Usage: "Filter by id", Repeated: true},
			{Name: "provider-auth-method-id", Type: resourcecmd.FlagString, Target: "query.provider_auth_method_id", Usage: "Filter by auth method"},
		},
	}
}

func sampleCreateOperation() resourcecmd.OperationSpec {
	return resourcecmd.OperationSpec{
		Name:   resourcecmd.OperationCreate,
		Method: "POST",
		Path:   "/provider-deployments",
		Short:  "Create a deployment",
		Args: []resourcecmd.ArgumentSpec{
			{Name: "provider-id", Target: "body.provider_id", Required: true, Description: "Provider ID"},
			{Name: "name", Target: "body.name", Required: false, Description: "Name"},
		},
		Flags: []resourcecmd.FlagSpec{
			{Name: "locked-provider-version-id", Type: resourcecmd.FlagString, Target: "body.locked_provider_version_id", Usage: "Pin a version"},
		},
	}
}

func sampleResource() resourcecmd.ResourceSpec {
	return resourcecmd.ResourceSpec{
		Plural:     "providers",
		Singular:   "providers",
		Short:      "Browse providers",
		Operations: []resourcecmd.OperationSpec{sampleListOperation()},
	}
}

func TestRootHelpSeparatesResourceCommands(t *testing.T) {
	t.Parallel()

	stdout := &bytes.Buffer{}
	application := &app.App{
		Stdout: stdout,
		Stderr: &bytes.Buffer{},
	}

	command, err := newTestRootCommand(application)
	if err != nil {
		t.Fatalf("newTestRootCommand() error = %v", err)
	}

	if err := command.Help(); err != nil {
		t.Fatalf("Help() error = %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Commands:\n") {
		t.Fatalf("help output missing Commands section:\n%s", output)
	}
	if !strings.Contains(output, "Resource Admin Commands:\n") {
		t.Fatalf("help output missing Resource Admin Commands section:\n%s", output)
	}
	if !strings.Contains(output, "Resource Admin Commands:\n  providers") &&
		!strings.Contains(output, "Resource Admin Commands:\n          providers") {
		if !strings.Contains(output, "providers") {
			t.Fatalf("help output missing providers resource:\n%s", output)
		}
	}
	if strings.Contains(output, "\nCommands:\n  providers") {
		t.Fatalf("providers unexpectedly listed in general Commands section:\n%s", output)
	}
}

func newTestRootCommand(application *app.App) (*cobra.Command, error) {
	commandutil.RegisterTemplateFuncs()
	commandutil.ConfigureHelpFeatures(application.StdoutFeatures())
	ctx := commandutil.NewContext(application, &commandutil.RootOptions{Format: "structured"})

	command := &cobra.Command{
		Use:   "metorial-admin",
		Short: "Admin CLI for the Metorial Magnetar API",
	}
	command.SetOut(application.Stdout)
	command.SetErr(application.Stderr)
	commandutil.ConfigureCommand(command)

	builder := func(resource resourcecmd.ResourceSpec, operation resourcecmd.OperationSpec) (*cobra.Command, error) {
		return newPublicResourceAction(application, newRootOptionsView(ctx.Options), resource, operation)
	}

	resource := sampleResource()
	resourceCommand, err := resourcecmd.NewResourceCommand(resource, builder)
	if err != nil {
		return nil, err
	}
	commandutil.SetCommandCategory(resourceCommand, commandutil.CommandCategoryResource)
	commandutil.ConfigureCommand(resourceCommand)
	command.AddCommand(resourceCommand)

	return command, nil
}

func TestBuildResourceTargetIncludesRepeatedQueryValues(t *testing.T) {
	t.Parallel()

	resource := sampleResource()
	operation := sampleListOperation()
	command, err := newPublicResourceAction(&app.App{}, &rootOptionsView{}, resource, operation)
	if err != nil {
		t.Fatalf("newPublicResourceAction() error = %v", err)
	}

	if err := command.Flags().Set("limit", "10"); err != nil {
		t.Fatalf("Set(limit) error = %v", err)
	}
	if err := command.Flags().Set("id", "prov_1,prov_2"); err != nil {
		t.Fatalf("Set(id) error = %v", err)
	}

	target, err := buildResourceTarget(command, resource, operation, nil)
	if err != nil {
		t.Fatalf("buildResourceTarget() error = %v", err)
	}

	if target != "/providers?id=prov_1&id=prov_2&limit=10" {
		t.Fatalf("buildResourceTarget() = %q", target)
	}
}

func TestBuildResourceTargetAppliesDefaultListLimit(t *testing.T) {
	t.Parallel()

	resource := sampleResource()
	operation := sampleListOperation()
	command, err := newPublicResourceAction(&app.App{}, &rootOptionsView{}, resource, operation)
	if err != nil {
		t.Fatalf("newPublicResourceAction() error = %v", err)
	}

	target, err := buildResourceTarget(command, resource, operation, nil)
	if err != nil {
		t.Fatalf("buildResourceTarget() error = %v", err)
	}

	if target != "/providers?limit=15" {
		t.Fatalf("buildResourceTarget() = %q", target)
	}
}

func TestBuildResourceBodyMergesExplicitJSONAndFlags(t *testing.T) {
	t.Parallel()

	resource := resourcecmd.ResourceSpec{Plural: "deployments", Singular: "deployments"}
	operation := sampleCreateOperation()

	command, err := newPublicResourceAction(&app.App{}, &rootOptionsView{}, resource, operation)
	if err != nil {
		t.Fatalf("newPublicResourceAction() error = %v", err)
	}

	if err := command.Flags().Set("body", `{"metadata":{"team":"cli"}}`); err != nil {
		t.Fatalf("Set(body) error = %v", err)
	}
	if err := command.Flags().Set("locked-provider-version-id", "ver_123"); err != nil {
		t.Fatalf("Set(locked-provider-version-id) error = %v", err)
	}

	body, err := buildResourceBody(command, resource, operation, []string{"prov_123", "Production"})
	if err != nil {
		t.Fatalf("buildResourceBody() error = %v", err)
	}

	if body["provider_id"] != "prov_123" {
		t.Fatalf("provider_id = %#v", body["provider_id"])
	}
	if body["locked_provider_version_id"] != "ver_123" {
		t.Fatalf("locked_provider_version_id = %#v", body["locked_provider_version_id"])
	}
	if _, ok := body["metadata"]; !ok {
		t.Fatalf("metadata missing from merged body: %#v", body)
	}
	if body["name"] != "Production" {
		t.Fatalf("name = %#v", body["name"])
	}
}

func TestApplyOperationPathSubstitutesPathParams(t *testing.T) {
	t.Parallel()

	operation := resourcecmd.OperationSpec{
		Name:   resourcecmd.OperationGet,
		Method: "GET",
		Path:   "/magic-mcp-servers/:magicMcpServerId/tools",
		Args: []resourcecmd.ArgumentSpec{
			{Name: "server-id", Target: "path.magicMcpServerId", Required: true},
		},
	}

	path, err := applyOperationPath(operation, []string{"mcp_123"})
	if err != nil {
		t.Fatalf("applyOperationPath() error = %v", err)
	}
	if path != "/magic-mcp-servers/mcp_123/tools" {
		t.Fatalf("applyOperationPath() = %q", path)
	}
}

func TestCamelToSnakeHandlesSDKFieldNames(t *testing.T) {
	t.Parallel()

	if got := camelToSnake("ProviderDeploymentId"); got != "provider_deployment_id" {
		t.Fatalf("camelToSnake() = %q", got)
	}
}

func TestResourceOperationArgsRejectsExtraArguments(t *testing.T) {
	t.Parallel()

	operation := resourcecmd.OperationSpec{
		Name: resourcecmd.OperationGet,
		Args: []resourcecmd.ArgumentSpec{
			{Name: "provider-id", Required: true},
		},
	}

	err := resourceOperationArgs(operation)(nil, []string{"prov_1", "extra"})
	if err == nil || !strings.Contains(err.Error(), "accepts at most 1 arg") {
		t.Fatalf("resourceOperationArgs() error = %v", err)
	}
}

func TestBuildResourceTargetUsesSnakeCaseQueryKeys(t *testing.T) {
	t.Parallel()

	resource := sampleResource()
	operation := sampleListOperation()
	command, err := newPublicResourceAction(&app.App{}, &rootOptionsView{}, resource, operation)
	if err != nil {
		t.Fatalf("newPublicResourceAction() error = %v", err)
	}

	if err := command.Flags().Set("provider-auth-method-id", "pam_123"); err != nil {
		t.Fatalf("Set(provider-auth-method-id) error = %v", err)
	}

	target, err := buildResourceTarget(command, resource, operation, nil)
	if err != nil {
		t.Fatalf("buildResourceTarget() error = %v", err)
	}

	if !strings.Contains(target, "provider_auth_method_id=pam_123") {
		t.Fatalf("buildResourceTarget() = %q", target)
	}
}

func TestRootOptionsViewTracksUpdatedFormat(t *testing.T) {
	t.Parallel()

	options := &commandutil.RootOptions{Format: "structured"}
	view := newRootOptionsView(options)

	options.Format = "json"

	if got := view.format(); got != "json" {
		t.Fatalf("view.format() = %q, want %q", got, "json")
	}
}

func TestResourceCommandHelpIncludesArgumentsSection(t *testing.T) {
	t.Parallel()

	stdout := &bytes.Buffer{}
	resource := resourcecmd.ResourceSpec{Plural: "identities", Singular: "identities"}
	operation := resourcecmd.OperationSpec{
		Name:   resourcecmd.OperationCreate,
		Method: "POST",
		Path:   "/identities",
		Short:  "Create an identity",
		Args: []resourcecmd.ArgumentSpec{
			{Name: "actor-id", Target: "body.actor_id", Required: true, Description: "Actor ID"},
		},
	}
	command, err := newPublicResourceAction(&app.App{Stdout: stdout, Stderr: &bytes.Buffer{}}, &rootOptionsView{}, resource, operation)
	if err != nil {
		t.Fatalf("newPublicResourceAction() error = %v", err)
	}
	command.SetOut(stdout)
	command.SetErr(&bytes.Buffer{})

	if err := command.Help(); err != nil {
		t.Fatalf("Help() error = %v", err)
	}

	rendered := stdout.String()
	if !strings.Contains(rendered, "Arguments:\n") {
		t.Fatalf("help output missing Arguments section:\n%s", rendered)
	}
	if !strings.Contains(rendered, "actor-id") {
		t.Fatalf("help output missing actor-id argument:\n%s", rendered)
	}
}
