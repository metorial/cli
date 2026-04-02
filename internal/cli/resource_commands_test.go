package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/metorial/cli/internal/app"
	"github.com/metorial/cli/internal/resourcecmd"
)

func TestNewRootCommandRegistersProvidersResource(t *testing.T) {
	t.Parallel()

	command, err := newRootCommand(&app.App{})
	if err != nil {
		t.Fatalf("newRootCommand() error = %v", err)
	}

	found := false
	for _, child := range command.Commands() {
		if child.Name() == "providers" {
			found = true
			break
		}
	}

	if !found {
		t.Fatalf("root command did not register providers resource command")
	}
}

func TestRootHelpSeparatesResourceCommands(t *testing.T) {
	t.Parallel()

	stdout := &bytes.Buffer{}
	application := &app.App{
		Stdout: stdout,
		Stderr: &bytes.Buffer{},
	}

	command, err := newRootCommand(application)
	if err != nil {
		t.Fatalf("newRootCommand() error = %v", err)
	}

	if err := command.Help(); err != nil {
		t.Fatalf("Help() error = %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Commands:\n") {
		t.Fatalf("help output missing Commands section:\n%s", output)
	}
	if !strings.Contains(output, "Resource Commands:\n") {
		t.Fatalf("help output missing Resource Commands section:\n%s", output)
	}
	if strings.Contains(output, "Commands:\n  providers") {
		t.Fatalf("providers unexpectedly listed in general Commands section:\n%s", output)
	}
}

func TestBuildResourceTargetIncludesRepeatedQueryValues(t *testing.T) {
	t.Parallel()

	command, err := newPublicResourceAction(&app.App{}, &rootOptions{}, resourcecmd.ProvidersResource(), resourcecmd.ProvidersResource().Operations[0])
	if err != nil {
		t.Fatalf("newPublicResourceAction() error = %v", err)
	}

	if err := command.Flags().Set("limit", "10"); err != nil {
		t.Fatalf("Set(limit) error = %v", err)
	}
	if err := command.Flags().Set("id", "prov_1,prov_2"); err != nil {
		t.Fatalf("Set(id) error = %v", err)
	}

	target, err := buildResourceTarget(command, resourcecmd.ProvidersResource(), resourcecmd.ProvidersResource().Operations[0], nil)
	if err != nil {
		t.Fatalf("buildResourceTarget() error = %v", err)
	}

	if target != "/provider-listings?id=prov_1&id=prov_2&limit=10" {
		t.Fatalf("buildResourceTarget() = %q", target)
	}
}

func TestBuildResourceTargetAppliesDefaultListLimit(t *testing.T) {
	t.Parallel()

	command, err := newPublicResourceAction(&app.App{}, &rootOptions{}, resourcecmd.ProvidersResource(), resourcecmd.ProvidersResource().Operations[0])
	if err != nil {
		t.Fatalf("newPublicResourceAction() error = %v", err)
	}

	target, err := buildResourceTarget(command, resourcecmd.ProvidersResource(), resourcecmd.ProvidersResource().Operations[0], nil)
	if err != nil {
		t.Fatalf("buildResourceTarget() error = %v", err)
	}

	if target != "/provider-listings?limit=15" {
		t.Fatalf("buildResourceTarget() = %q", target)
	}
}

func TestBuildResourceBodyMergesExplicitJSONAndFlags(t *testing.T) {
	t.Parallel()

	resource := resourcecmd.DeploymentsResource()
	operation, ok := resource.Operation(resourcecmd.OperationCreate)
	if !ok {
		t.Fatalf("DeploymentsResource() missing create operation")
	}

	command, err := newPublicResourceAction(&app.App{}, &rootOptions{}, resource, operation)
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

func TestResourceOperationArgsRendersHelpOnMissingArgs(t *testing.T) {
	t.Parallel()

	stdout := &bytes.Buffer{}
	command, err := newPublicResourceAction(&app.App{}, &rootOptions{}, resourcecmd.ActorsResource(), resourcecmd.ActorsResource().Operations[2])
	if err != nil {
		t.Fatalf("newPublicResourceAction() error = %v", err)
	}
	command.SetOut(stdout)
	command.SetErr(&bytes.Buffer{})

	err = command.Args(command, nil)
	if err == nil {
		t.Fatal("expected missing args error")
	}

	rendered := stdout.String()
	if !strings.Contains(rendered, "Usage:\n") {
		t.Fatalf("help output missing usage:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Arguments:\n") {
		t.Fatalf("help output missing arguments:\n%s", rendered)
	}
	if !strings.Contains(rendered, "type") || !strings.Contains(rendered, "name") {
		t.Fatalf("help output missing expected args:\n%s", rendered)
	}
}

func TestBuildResourceTargetUsesSnakeCaseQueryKeys(t *testing.T) {
	t.Parallel()

	resource := resourcecmd.AuthConfigsResource()
	operation, ok := resource.Operation(resourcecmd.OperationList)
	if !ok {
		t.Fatalf("AuthConfigsResource() missing list operation")
	}

	command, err := newPublicResourceAction(&app.App{}, &rootOptions{}, resource, operation)
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

func TestResourceCommandHelpIncludesArgumentsSection(t *testing.T) {
	t.Parallel()

	stdout := &bytes.Buffer{}
	command, err := newPublicResourceAction(&app.App{Stdout: stdout, Stderr: &bytes.Buffer{}}, &rootOptions{}, resourcecmd.IdentitiesResource(), resourcecmd.IdentitiesResource().Operations[2])
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
