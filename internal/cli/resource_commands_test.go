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

	if target != "/providers?id=prov_1&id=prov_2&limit=10" {
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

	if target != "/providers?limit=5" {
		t.Fatalf("buildResourceTarget() = %q", target)
	}
}

func TestBuildResourceBodyMergesExplicitJSONAndFlags(t *testing.T) {
	t.Parallel()

	resource := resourcecmd.ProviderDeploymentsResource()
	operation, ok := resource.Operation(resourcecmd.OperationCreate)
	if !ok {
		t.Fatalf("ProviderDeploymentsResource() missing create operation")
	}

	command, err := newPublicResourceAction(&app.App{}, &rootOptions{}, resource, operation)
	if err != nil {
		t.Fatalf("newPublicResourceAction() error = %v", err)
	}

	if err := command.Flags().Set("body", `{"metadata":{"team":"cli"}}`); err != nil {
		t.Fatalf("Set(body) error = %v", err)
	}
	if err := command.Flags().Set("provider-id", "prov_123"); err != nil {
		t.Fatalf("Set(provider-id) error = %v", err)
	}
	if err := command.Flags().Set("locked-provider-version-id", "ver_123"); err != nil {
		t.Fatalf("Set(locked-provider-version-id) error = %v", err)
	}

	body, err := buildResourceBody(command, operation)
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
