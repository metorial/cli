package app

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/manifoldco/promptui"
	"github.com/metorial/cli/internal/auth"
	"github.com/metorial/cli/internal/config"
	"github.com/metorial/cli/internal/terminal"
	instancesresource "github.com/metorial/metorial-go/v1/resources/instances"
)

type App struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	stdoutFeatures terminal.Features
	stderrFeatures terminal.Features
}

func New() *App {
	return &App{
		Stdin:          os.Stdin,
		Stdout:         os.Stdout,
		Stderr:         os.Stderr,
		stdoutFeatures: terminal.Detect(os.Stdout),
		stderrFeatures: terminal.Detect(os.Stderr),
	}
}

func (a *App) ResolveConfig(apiKeyFlag, apiHostFlag, profileFlag, instanceFlag string) (config.Runtime, error) {
	runtime, err := a.ResolveAuthConfig(apiKeyFlag, apiHostFlag, profileFlag)
	if err != nil {
		return config.Runtime{}, err
	}

	return a.resolveInstance(runtime, instanceFlag)
}

func (a *App) ResolveAuthConfig(apiKeyFlag, apiHostFlag, profileFlag string) (config.Runtime, error) {
	platformURL, err := config.ResolvePlatformURL()
	if err != nil {
		return config.Runtime{}, err
	}

	store, err := config.OpenStore()
	if err != nil {
		return config.Runtime{}, err
	}

	selectedProfileID := strings.TrimSpace(profileFlag)
	var profile *config.Profile
	var ok bool
	if selectedProfileID != "" {
		profile, ok = store.ProfileByID(selectedProfileID)
		if !ok {
			return config.Runtime{}, fmt.Errorf(
				"metorial: profile %q does not exist.\nRun \"metorial profiles list\" to see available profiles or \"metorial login\" to add a new one",
				selectedProfileID,
			)
		}
	} else {
		profile, ok = store.CurrentProfile()
	}

	defaultHost := store.Settings().DefaultAPIHost
	if ok && strings.TrimSpace(profile.APIHost) != "" && strings.TrimSpace(apiHostFlag) == "" && strings.TrimSpace(os.Getenv(config.EnvAPIHost)) == "" {
		defaultHost = profile.APIHost
	}

	apiHostURL, err := config.ResolveAPIHostWithDefault(apiHostFlag, defaultHost)
	if err != nil {
		return config.Runtime{}, err
	}

	apiKey := strings.TrimSpace(apiKeyFlag)
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv(config.EnvAPIKey))
	}
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv(config.EnvToken))
	}

	if apiKey != "" {
		return config.Runtime{
			APIKey:      apiKey,
			APIHost:     apiHostURL.String(),
			APIHostURL:  apiHostURL,
			PlatformURL: platformURL,
			Profile:     profile,
		}, nil
	}

	if !ok {
		return config.Runtime{
			APIHost:     apiHostURL.String(),
			APIHostURL:  apiHostURL,
			PlatformURL: platformURL,
		}, nil
	}

	runtime := config.Runtime{
		APIKey:      profile.AccessToken,
		APIHost:     apiHostURL.String(),
		APIHostURL:  apiHostURL,
		PlatformURL: platformURL,
		Profile:     profile,
	}

	runtime.Refresh = func(force bool) (config.Runtime, error) {
		return a.refreshProfileRuntime(apiHostFlag, profile.ID, force)
	}

	if profile.Expired(time.Now().UTC().Add(30 * time.Second)) {
		return runtime.Refresh(true)
	}

	return runtime, nil
}

func (a *App) refreshProfileRuntime(apiHostFlag, profileID string, force bool) (config.Runtime, error) {
	platformURL, err := config.ResolvePlatformURL()
	if err != nil {
		return config.Runtime{}, err
	}

	store, err := config.OpenStore()
	if err != nil {
		return config.Runtime{}, err
	}

	profile, ok := store.ProfileByID(profileID)
	if !ok {
		apiHostURL, err := config.ResolveAPIHostWithDefault(apiHostFlag, store.Settings().DefaultAPIHost)
		if err != nil {
			return config.Runtime{}, err
		}
		return config.Runtime{}, config.Runtime{
			APIHost:     apiHostURL.String(),
			APIHostURL:  apiHostURL,
			PlatformURL: platformURL,
		}.RequireAPIKey()
	}

	defaultHost := store.Settings().DefaultAPIHost
	if strings.TrimSpace(profile.APIHost) != "" && strings.TrimSpace(apiHostFlag) == "" && strings.TrimSpace(os.Getenv(config.EnvAPIHost)) == "" {
		defaultHost = profile.APIHost
	}

	apiHostURL, err := config.ResolveAPIHostWithDefault(apiHostFlag, defaultHost)
	if err != nil {
		return config.Runtime{}, err
	}

	if !force && !profile.Expired(time.Now().UTC().Add(30*time.Second)) {
		runtime := config.Runtime{
			APIKey:      profile.AccessToken,
			APIHost:     apiHostURL.String(),
			APIHostURL:  apiHostURL,
			PlatformURL: platformURL,
			Profile:     profile,
		}
		runtime.Refresh = func(force bool) (config.Runtime, error) {
			return a.refreshProfileRuntime(apiHostFlag, profile.ID, force)
		}
		return runtime, nil
	}

	if strings.TrimSpace(profile.RefreshToken) == "" || strings.TrimSpace(profile.ClientID) == "" {
		return config.Runtime{}, fmt.Errorf(
			"The current profile cannot be refreshed because it has expired.\nRun \"metorial login\" to sign in again or \"metorial profiles list\" to switch profiles.",
		)
	}

	client := auth.NewClient(apiHostURL)
	response, err := client.RefreshToken(profile.ClientID, profile.RefreshToken)
	if err != nil {
		apiError := &auth.Error{}
		if errors.As(err, &apiError) {
			switch apiError.ErrorCode {
			case "invalid_grant":
				return config.Runtime{}, fmt.Errorf(
					"metorial: the current profile %s has expired or was revoked.\nRun \"metorial login\" to sign in again or \"metorial profiles list\" to switch profiles",
					profile.ID,
				)
			default:
				if strings.TrimSpace(apiError.ErrorMessage) != "" {
					return config.Runtime{}, fmt.Errorf(
						"metorial: failed to refresh the current profile %s: %s\nRun \"metorial login\" to sign in again or \"metorial profiles list\" to switch profiles",
						profile.ID,
						apiError.ErrorMessage,
					)
				}
			}
		}
		return config.Runtime{}, err
	}

	now := time.Now().UTC()
	profile.AccessToken = response.AccessToken
	if strings.TrimSpace(response.RefreshToken) != "" {
		profile.RefreshToken = response.RefreshToken
	}
	if strings.TrimSpace(response.ClientID) != "" {
		profile.ClientID = response.ClientID
	}
	if strings.TrimSpace(response.TokenType) != "" {
		profile.TokenType = response.TokenType
	}
	if response.ExpiresIn > 0 {
		profile.ExpiresAt = now.Add(time.Duration(response.ExpiresIn) * time.Second)
	}
	if strings.TrimSpace(response.Organization.ID) != "" {
		profile.OrgID = response.Organization.ID
		profile.OrgName = response.Organization.Name
	}
	if strings.TrimSpace(response.User.ID) != "" {
		profile.UserID = response.User.ID
		profile.UserName = response.User.Name
		profile.UserEmail = response.User.Email
	}

	if err := store.UpsertProfile(*profile, true); err != nil {
		return config.Runtime{}, err
	}

	runtime := config.Runtime{
		APIKey:      profile.AccessToken,
		APIHost:     apiHostURL.String(),
		APIHostURL:  apiHostURL,
		PlatformURL: platformURL,
		Profile:     profile,
	}
	runtime.Refresh = func(force bool) (config.Runtime, error) {
		return a.refreshProfileRuntime(apiHostFlag, profile.ID, force)
	}

	return runtime, nil
}

func (a *App) resolveInstance(runtime config.Runtime, instanceFlag string) (config.Runtime, error) {
	return a.resolveInstanceSelection(runtime, instanceFlag, tokenNeedsInstanceHeader(runtime.APIKey))
}

func (a *App) ResolveExampleInstance(runtime config.Runtime, instanceFlag string) (config.Runtime, error) {
	return a.resolveInstanceSelection(runtime, instanceFlag, true)
}

func (a *App) resolveInstanceSelection(runtime config.Runtime, instanceFlag string, requireSelection bool) (config.Runtime, error) {
	if strings.TrimSpace(runtime.APIKey) == "" {
		return runtime, nil
	}

	if strings.TrimSpace(instanceFlag) != "" {
		runtime.InstanceID = strings.TrimSpace(instanceFlag)
		return preserveInstanceOnRefresh(runtime), nil
	}

	cwd, err := os.Getwd()
	if err == nil {
		projectStore, err := config.OpenProjectStore(cwd)
		if err == nil {
			if instanceID, ok := projectStore.SelectedInstance(instanceSelectionKey(runtime)); ok {
				runtime.InstanceID = instanceID
				return preserveInstanceOnRefresh(runtime), nil
			}
		}
	}

	if !requireSelection {
		return runtime, nil
	}

	sdk, err := runtime.BareSDK()
	if err != nil {
		return config.Runtime{}, err
	}

	instances, err := sdk.Instances.List()
	if err != nil {
		return config.Runtime{}, err
	}

	if len(instances.Items) == 0 {
		return config.Runtime{}, fmt.Errorf("metorial: this token does not have access to any instances")
	}

	if len(instances.Items) == 1 {
		runtime.InstanceID = instances.Items[0].Id
		return preserveInstanceOnRefresh(runtime), nil
	}

	if !a.stdoutFeatures.IsTTY {
		return config.Runtime{}, fmt.Errorf(
			"metorial: this token can access multiple instances, so an instance must be selected.\nUse --instance <instance-id> or run \"metorial instances list\" to see available instances",
		)
	}

	selectedInstance, err := a.promptForInstance(instances.Items)
	if err != nil {
		return config.Runtime{}, err
	}

	runtime.InstanceID = selectedInstance.Id

	cwd, err = os.Getwd()
	if err == nil {
		projectStore, err := config.OpenProjectStore(cwd)
		if err == nil {
			if err := projectStore.SetSelectedInstance(instanceSelectionKey(runtime), selectedInstance.Id); err == nil {
				_, _ = fmt.Fprintf(
					a.Stdout,
					"Using instance %s (%s). Saved to %s\n",
					selectedInstance.Name,
					selectedInstance.Id,
					projectStore.Path(),
				)
			}
		}
	}

	return preserveInstanceOnRefresh(runtime), nil
}

func (a *App) promptForInstance(instances []instancesresource.InstancesListOutputItems) (*instancesresource.InstancesListOutputItems, error) {
	items := make([]string, 0, len(instances))
	for _, instance := range instances {
		items = append(items, fmt.Sprintf("%s (%s)", firstNonEmpty(instance.Name, instance.Slug), instance.Id))
	}

	prompt := promptui.Select{
		Label: "Select an instance",
		Items: items,
		Stdin: readerCloser{
			Reader: a.Stdin,
		},
		Stdout: writerCloser{
			Writer: a.Stdout,
		},
	}

	index, _, err := prompt.Run()
	if err != nil {
		return nil, fmt.Errorf("metorial: instance selection was cancelled. Use --instance <instance-id> or run \"metorial instances list\" to see available instances")
	}

	selected := instances[index]
	return &selected, nil
}

func tokenNeedsInstanceHeader(token string) bool {
	value := strings.TrimSpace(token)
	for _, prefix := range []string{"metorial_sk_", "metorial_fk_", "metorial_pk"} {
		if strings.HasPrefix(value, prefix) {
			return false
		}
	}
	return true
}

func instanceSelectionKey(runtime config.Runtime) string {
	if runtime.Profile != nil {
		return "profile:" + runtime.APIHost + ":" + runtime.Profile.ID
	}

	hash := sha256.Sum256([]byte(runtime.APIKey))
	return "token:" + runtime.APIHost + ":" + hex.EncodeToString(hash[:8])
}

func preserveInstanceOnRefresh(runtime config.Runtime) config.Runtime {
	if runtime.Refresh == nil || strings.TrimSpace(runtime.InstanceID) == "" {
		return runtime
	}

	instanceID := runtime.InstanceID
	originalRefresh := runtime.Refresh
	runtime.Refresh = func(force bool) (config.Runtime, error) {
		refreshed, err := originalRefresh(force)
		if err != nil {
			return config.Runtime{}, err
		}
		refreshed.InstanceID = instanceID
		return refreshed, nil
	}

	return runtime
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

type writerCloser struct {
	io.Writer
}

func (writerCloser) Close() error {
	return nil
}

type readerCloser struct {
	io.Reader
}

func (readerCloser) Close() error {
	return nil
}

func (a *App) StdoutFeatures() terminal.Features {
	return a.stdoutFeatures
}

func (a *App) StderrFeatures() terminal.Features {
	return a.stderrFeatures
}
