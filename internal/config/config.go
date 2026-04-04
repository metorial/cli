package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	metorial "github.com/metorial/metorial-go/v1"
)

const (
	DefaultAPIHost  = "api.metorial.com"
	DefaultAppHost  = "platform.metorial.com"
	DefaultScheme   = "https"
	EnvAPIKey       = "METORIAL_API_KEY"
	EnvToken        = "METORIAL_TOKEN"
	EnvAPIHost      = "METORIAL_API_HOST"
	DefaultFeedback = "https://github.com/metorial/cli"
	ConfigVersion   = 1
)

type Runtime struct {
	APIKey      string
	APIHost     string
	APIHostURL  *url.URL
	PlatformURL string
	Profile     *Profile
	InstanceID  string
	Refresh     func(force bool) (Runtime, error)
}

type File struct {
	Version          int                `mapstructure:"version" json:"version"`
	CurrentProfileID string             `mapstructure:"current_profile_id" json:"current_profile_id"`
	Settings         Settings           `mapstructure:"settings" json:"settings"`
	Profiles         map[string]Profile `mapstructure:"profiles" json:"profiles"`
}

type Settings struct {
	DefaultAPIHost string `mapstructure:"default_api_host" json:"default_api_host"`
	DefaultFormat  string `mapstructure:"default_format" json:"default_format"`
}

type Profile struct {
	ID           string    `mapstructure:"id" json:"id"`
	Name         string    `mapstructure:"name" json:"name"`
	APIHost      string    `mapstructure:"api_host" json:"api_host"`
	ClientID     string    `mapstructure:"client_id" json:"client_id"`
	AccessToken  string    `mapstructure:"access_token" json:"access_token"`
	RefreshToken string    `mapstructure:"refresh_token" json:"refresh_token"`
	TokenType    string    `mapstructure:"token_type" json:"token_type"`
	ExpiresAt    time.Time `mapstructure:"expires_at" json:"expires_at"`
	OrgID        string    `mapstructure:"org_id" json:"org_id"`
	OrgName      string    `mapstructure:"org_name" json:"org_name"`
	UserID       string    `mapstructure:"user_id" json:"user_id"`
	UserName     string    `mapstructure:"user_name" json:"user_name"`
	UserEmail    string    `mapstructure:"user_email" json:"user_email"`
	CreatedAt    time.Time `mapstructure:"created_at" json:"created_at"`
	UpdatedAt    time.Time `mapstructure:"updated_at" json:"updated_at"`
	LastUsedAt   time.Time `mapstructure:"last_used_at" json:"last_used_at"`
}

type Store struct {
	path   string
	cached File
}

type ProjectFile struct {
	Version           int               `json:"version"`
	SelectedInstances map[string]string `json:"selected_instances"`
}

type ProjectStore struct {
	path   string
	cached ProjectFile
}

func ResolveAPIHost(apiHostFlag string) (*url.URL, error) {
	apiHostInput := strings.TrimSpace(apiHostFlag)
	if apiHostInput == "" {
		apiHostInput = strings.TrimSpace(os.Getenv(EnvAPIHost))
	}
	if apiHostInput == "" {
		apiHostInput = DefaultAPIHost
	}

	return NormalizeBaseURL(apiHostInput)
}

func ResolveAPIHostWithDefault(apiHostFlag, defaultHost string) (*url.URL, error) {
	apiHostInput := strings.TrimSpace(apiHostFlag)
	if apiHostInput == "" {
		apiHostInput = strings.TrimSpace(os.Getenv(EnvAPIHost))
	}
	if apiHostInput == "" {
		apiHostInput = strings.TrimSpace(defaultHost)
	}
	if apiHostInput == "" {
		apiHostInput = DefaultAPIHost
	}

	return NormalizeBaseURL(apiHostInput)
}

func ResolvePlatformURL() (string, error) {
	return normalizePlatformURL(DefaultAppHost)
}

func (r Runtime) RequireAPIKey() error {
	if strings.TrimSpace(r.APIKey) != "" {
		return nil
	}

	return fmt.Errorf(
		"metorial: no authentication found.\nSign in with \"metorial login\" to keep using saved profiles on this machine.\nOr set %s/%s or pass --api-key for a one-off request.",
		EnvAPIKey,
		EnvToken,
	)
}

func (r Runtime) SDK() (*metorial.MetorialSdk, error) {
	headers := map[string]string{}
	if strings.TrimSpace(r.InstanceID) != "" {
		headers["metorial-instance-id"] = strings.TrimSpace(r.InstanceID)
	}

	return r.sdk(headers)
}

func (r Runtime) BareSDK() (*metorial.MetorialSdk, error) {
	return r.sdk(nil)
}

func (r Runtime) sdk(headers map[string]string) (*metorial.MetorialSdk, error) {
	if err := r.RequireAPIKey(); err != nil {
		return nil, err
	}

	options := []metorial.Option{
		metorial.WithAPIKey(r.APIKey),
		metorial.WithAPIHost(r.APIHost),
	}
	if len(headers) > 0 {
		options = append(options, metorial.WithHeaders(headers))
	}

	return metorial.New(options...)
}

func NormalizeBaseURL(raw string) (*url.URL, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, fmt.Errorf("metorial: API host cannot be empty")
	}

	if !strings.Contains(value, "://") {
		value = DefaultScheme + "://" + value
	}

	parsed, err := url.Parse(value)
	if err != nil {
		return nil, fmt.Errorf("metorial: invalid API host %q: %w", raw, err)
	}

	if parsed.Scheme == "" {
		parsed.Scheme = DefaultScheme
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("metorial: invalid API host %q", raw)
	}

	parsed.Path = ""
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""

	return parsed, nil
}

func normalizePlatformURL(raw string) (string, error) {
	parsed, err := NormalizeBaseURL(raw)
	if err != nil {
		return "", err
	}

	return parsed.String(), nil
}

func DefaultConfigPath() (string, error) {
	cliDir, err := DefaultCLIDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(cliDir, "config.json"), nil
}

func DefaultCLIDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("metorial: failed to resolve home directory: %w", err)
	}

	return filepath.Join(homeDir, ".metorial", "cli"), nil
}

func OpenStore() (*Store, error) {
	path, err := DefaultConfigPath()
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("metorial: failed to create config directory: %w", err)
	}

	store := &Store{
		path: path,
	}

	file := defaultFile()
	if _, err := os.Stat(path); err == nil {
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("metorial: failed to read config file %q: %w", path, err)
		}
		if err := json.Unmarshal(content, &file); err != nil {
			return nil, fmt.Errorf("metorial: failed to parse config file %q: %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("metorial: failed to read config file %q: %w", path, err)
	}

	if file.Version == 0 {
		file.Version = ConfigVersion
	}
	if file.Profiles == nil {
		file.Profiles = map[string]Profile{}
	}

	store.cached = file
	return store, nil
}

func (s *Store) Path() string {
	return s.path
}

func (s *Store) Read() File {
	return cloneFile(s.cached)
}

func (s *Store) Settings() Settings {
	return s.cached.Settings
}

func (s *Store) Write(file File) error {
	if file.Version == 0 {
		file.Version = ConfigVersion
	}
	if file.Profiles == nil {
		file.Profiles = map[string]Profile{}
	}

	content, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("metorial: failed to encode config file %q: %w", s.path, err)
	}
	content = append(content, '\n')

	if err := os.WriteFile(s.path, content, 0o600); err != nil {
		return fmt.Errorf("metorial: failed to write config file %q: %w", s.path, err)
	}

	s.cached = cloneFile(file)
	return nil
}

func (s *Store) UpdateSettings(update func(*Settings)) error {
	file := s.Read()
	update(&file.Settings)
	return s.Write(file)
}

func OpenProjectStore(cwd string) (*ProjectStore, error) {
	path := filepath.Join(cwd, ".metorial", "config.json")
	store := &ProjectStore{path: path}

	file := ProjectFile{
		Version:           ConfigVersion,
		SelectedInstances: map[string]string{},
	}

	if _, err := os.Stat(path); err == nil {
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("metorial: failed to read project config file %q: %w", path, err)
		}
		if err := json.Unmarshal(content, &file); err != nil {
			return nil, fmt.Errorf("metorial: failed to parse project config file %q: %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("metorial: failed to read project config file %q: %w", path, err)
	}

	if file.Version == 0 {
		file.Version = ConfigVersion
	}
	if file.SelectedInstances == nil {
		file.SelectedInstances = map[string]string{}
	}

	store.cached = file
	return store, nil
}

func (s *ProjectStore) Path() string {
	return s.path
}

func (s *ProjectStore) SelectedInstance(key string) (string, bool) {
	value := strings.TrimSpace(s.cached.SelectedInstances[strings.TrimSpace(key)])
	if value == "" {
		return "", false
	}
	return value, true
}

func (s *ProjectStore) SetSelectedInstance(key, instanceID string) error {
	file := s.cached
	if file.Version == 0 {
		file.Version = ConfigVersion
	}
	if file.SelectedInstances == nil {
		file.SelectedInstances = map[string]string{}
	}

	file.SelectedInstances[strings.TrimSpace(key)] = strings.TrimSpace(instanceID)

	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("metorial: failed to create project config directory: %w", err)
	}

	content, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("metorial: failed to encode project config file %q: %w", s.path, err)
	}
	content = append(content, '\n')

	if err := os.WriteFile(s.path, content, 0o600); err != nil {
		return fmt.Errorf("metorial: failed to write project config file %q: %w", s.path, err)
	}

	s.cached = file
	return nil
}

func (s *Store) CurrentProfile() (*Profile, bool) {
	file := s.cached
	if file.CurrentProfileID == "" {
		return nil, false
	}

	profile, ok := file.Profiles[file.CurrentProfileID]
	if !ok {
		return nil, false
	}

	return profile.Clone(), true
}

func (s *Store) ProfileByID(id string) (*Profile, bool) {
	profile, ok := s.cached.Profiles[strings.TrimSpace(id)]
	if !ok {
		return nil, false
	}

	return profile.Clone(), true
}

func (s *Store) SortedProfiles() []Profile {
	profiles := make([]Profile, 0, len(s.cached.Profiles))
	for _, profile := range s.cached.Profiles {
		profiles = append(profiles, profile)
	}

	sort.SliceStable(profiles, func(left, right int) bool {
		if profiles[left].LastUsedAt.Equal(profiles[right].LastUsedAt) {
			return profiles[left].ID < profiles[right].ID
		}
		return profiles[left].LastUsedAt.After(profiles[right].LastUsedAt)
	})

	return profiles
}

func (s *Store) UpsertProfile(profile Profile, setCurrent bool) error {
	file := s.Read()
	now := time.Now().UTC()

	if profile.ID == "" {
		profile.ID = ProfileID(profile.OrgID, profile.UserID)
	}

	existing, ok := file.Profiles[profile.ID]
	if ok {
		if profile.Name == "" {
			profile.Name = existing.Name
		}
		if profile.CreatedAt.IsZero() {
			profile.CreatedAt = existing.CreatedAt
		}
	}

	if profile.CreatedAt.IsZero() {
		profile.CreatedAt = now
	}

	profile.UpdatedAt = now
	profile.LastUsedAt = now

	file.Profiles[profile.ID] = profile
	if setCurrent {
		file.CurrentProfileID = profile.ID
	}

	return s.Write(file)
}

func (s *Store) SetCurrentProfile(id string) error {
	file := s.Read()
	profile, ok := file.Profiles[strings.TrimSpace(id)]
	if !ok {
		return fmt.Errorf("profile not found")
	}

	now := time.Now().UTC()
	profile.LastUsedAt = now
	profile.UpdatedAt = now
	file.Profiles[profile.ID] = profile
	file.CurrentProfileID = profile.ID

	return s.Write(file)
}

func (s *Store) RemoveProfile(id string) (*Profile, error) {
	file := s.Read()
	profile, ok := file.Profiles[strings.TrimSpace(id)]
	if !ok {
		return nil, fmt.Errorf("profile not found")
	}

	delete(file.Profiles, profile.ID)
	if file.CurrentProfileID == profile.ID {
		file.CurrentProfileID = newestProfileID(file.Profiles)
	}

	if err := s.Write(file); err != nil {
		return nil, err
	}

	return profile.Clone(), nil
}

func (p Profile) Clone() *Profile {
	clone := p
	return &clone
}

func (p Profile) Expired(reference time.Time) bool {
	if p.ExpiresAt.IsZero() {
		return false
	}

	return !p.ExpiresAt.After(reference)
}

func ProfileID(orgID, userID string) string {
	return strings.TrimSpace(orgID) + ":" + strings.TrimSpace(userID)
}

func defaultFile() File {
	return File{
		Version:  ConfigVersion,
		Profiles: map[string]Profile{},
	}
}

func cloneFile(file File) File {
	clone := file
	clone.Profiles = map[string]Profile{}
	for key, profile := range file.Profiles {
		clone.Profiles[key] = profile
	}
	return clone
}

func newestProfileID(profiles map[string]Profile) string {
	if len(profiles) == 0 {
		return ""
	}

	type pair struct {
		id      string
		profile Profile
	}

	items := make([]pair, 0, len(profiles))
	for id, profile := range profiles {
		items = append(items, pair{id: id, profile: profile})
	}

	sort.SliceStable(items, func(left, right int) bool {
		if items[left].profile.LastUsedAt.Equal(items[right].profile.LastUsedAt) {
			return items[left].id < items[right].id
		}
		return items[left].profile.LastUsedAt.After(items[right].profile.LastUsedAt)
	})

	return items[0].id
}
