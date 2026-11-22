package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/metorial/cli/internal/claimlink"
	metorial "github.com/metorial/metorial-go/v1"
)

const (
	DefaultAPIHost  = "api.metorial.com"
	DefaultAppHost  = "platform.metorial.com"
	DefaultScheme   = "https"
	EnvAPIKey       = "METORIAL_API_KEY"
	EnvToken        = "METORIAL_TOKEN"
	EnvAPIHost      = "METORIAL_API_HOST"
	EnvInstanceID   = "METORIAL_INSTANCE_ID"
	DefaultFeedback = "https://github.com/metorial/cli"
	ConfigVersion   = 2

	projectConfigVersion = 1
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

const (
	SubjectTypeOrganizationMember = "organization_member"
	SubjectTypeConsumer           = "consumer"
)

type File struct {
	Version                  int                `mapstructure:"version" json:"version"`
	CurrentProfileID         string             `mapstructure:"current_profile_id" json:"current_profile_id"`
	CurrentConsumerProfileID string             `mapstructure:"current_consumer_profile_id" json:"current_consumer_profile_id,omitempty"`
	Settings                 Settings           `mapstructure:"settings" json:"settings"`
	Profiles                 map[string]Profile `mapstructure:"profiles" json:"profiles"`

	extra map[string]json.RawMessage
}

type Settings struct {
	DefaultAPIHost string `mapstructure:"default_api_host" json:"default_api_host"`
	DefaultFormat  string `mapstructure:"default_format" json:"default_format"`

	extra map[string]json.RawMessage
}

type Profile struct {
	ID           string    `mapstructure:"id" json:"id"`
	Name         string    `mapstructure:"name" json:"name"`
	APIHost      string    `mapstructure:"api_host" json:"api_host"`
	ClientID     string    `mapstructure:"client_id" json:"client_id"`
	AccessToken  string    `mapstructure:"access_token" json:"access_token"`
	RefreshToken string    `mapstructure:"refresh_token" json:"refresh_token"`
	TokenStorage string    `mapstructure:"token_storage" json:"token_storage,omitempty"`
	TokenType    string    `mapstructure:"token_type" json:"token_type"`
	ExpiresAt    time.Time `mapstructure:"expires_at" json:"expires_at"`
	OrgID        string    `mapstructure:"org_id" json:"org_id"`
	OrgName      string    `mapstructure:"org_name" json:"org_name"`
	UserID       string    `mapstructure:"user_id" json:"user_id"`
	UserName     string    `mapstructure:"user_name" json:"user_name"`
	UserEmail    string    `mapstructure:"user_email" json:"user_email"`

	SubjectType       string `mapstructure:"subject_type" json:"subject_type,omitempty"`
	ConsumerProfileID string `mapstructure:"consumer_profile_id" json:"consumer_profile_id,omitempty"`
	ConsumerName      string `mapstructure:"consumer_name" json:"consumer_name,omitempty"`
	ConsumerEmail     string `mapstructure:"consumer_email" json:"consumer_email,omitempty"`
	PortalID          string `mapstructure:"portal_id" json:"portal_id,omitempty"`
	PortalName        string `mapstructure:"portal_name" json:"portal_name,omitempty"`
	InstanceID        string `mapstructure:"instance_id" json:"instance_id,omitempty"`

	Unclaimed      bool       `mapstructure:"unclaimed" json:"unclaimed,omitempty"`
	ClaimToken     string     `mapstructure:"claim_token" json:"claim_token,omitempty"`
	ClaimURL       string     `mapstructure:"claim_url" json:"claim_url,omitempty"`
	OrgSlug        string     `mapstructure:"org_slug" json:"org_slug,omitempty"`
	ClaimCheckedAt *time.Time `mapstructure:"claim_checked_at" json:"claim_checked_at,omitempty"`

	ClaimEmail string `mapstructure:"claim_email" json:"claim_email,omitempty"`

	CreatedAt  time.Time `mapstructure:"created_at" json:"created_at"`
	UpdatedAt  time.Time `mapstructure:"updated_at" json:"updated_at"`
	LastUsedAt time.Time `mapstructure:"last_used_at" json:"last_used_at"`

	extra map[string]json.RawMessage
}

type Store struct {
	path    string
	cached  File
	secrets Secrets
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
	if browserSessionAuthAvailable() {
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
		metorial.WithAPIHost(r.APIHost),
	}
	if strings.TrimSpace(r.APIKey) != "" {
		options = append(options, metorial.WithAPIKey(r.APIKey))
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
		path:    path,
		secrets: newSecrets(),
	}

	file, err := store.load()
	if err != nil {
		return nil, err
	}
	store.cached = file

	if store.secrets != nil && !preferFileTokenStorage() && hasLegacyTokens(file) {
		_ = store.update(func(*File) error { return nil })
	}

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
	return s.update(func(current *File) error {
		if file.extra == nil {
			file.extra = current.extra
		}
		*current = cloneFile(file)
		return nil
	})
}

func (s *Store) Reload() error {
	file, err := s.load()
	if err != nil {
		return err
	}
	s.cached = file
	return nil
}

func (s *Store) UpdateSettings(update func(*Settings)) error {
	return s.update(func(file *File) error {
		update(&file.Settings)
		return nil
	})
}

func (s *Store) load() (File, error) {
	file := defaultFile()
	content, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return file, nil
		}
		return File{}, fmt.Errorf("metorial: failed to read config file %q: %w", s.path, err)
	}

	file.Version = 0
	if err := json.Unmarshal(content, &file); err != nil {
		return File{}, fmt.Errorf("metorial: failed to parse config file %q: %w", s.path, err)
	}

	if file.Version == 0 {
		file.Version = 1
	}
	if file.Profiles == nil {
		file.Profiles = map[string]Profile{}
	}

	return file, nil
}

func (s *Store) update(mutate func(*File) error) error {
	unlock, err := lockPath(s.path+".lock", configLockTimeout)
	if err != nil {
		return fmt.Errorf("metorial: failed to lock config file %q: %w", s.path, err)
	}
	defer unlock()

	file, err := s.load()
	if err != nil {
		return err
	}
	if err := mutate(&file); err != nil {
		return err
	}

	file.Version = ConfigVersion
	if file.Profiles == nil {
		file.Profiles = map[string]Profile{}
	}

	stored := cloneFile(file)
	for id, profile := range stored.Profiles {
		profile, err := s.storeTokens(profile)
		if err != nil {
			return err
		}
		stored.Profiles[id] = profile
	}

	content, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return fmt.Errorf("metorial: failed to encode config file %q: %w", s.path, err)
	}
	content = append(content, '\n')

	if err := writeFileAtomic(s.path, content, 0o600); err != nil {
		return fmt.Errorf("metorial: failed to write config file %q: %w", s.path, err)
	}

	s.cached = stored
	return nil
}

func (s *Store) storeTokens(profile Profile) (Profile, error) {
	secret := profile.secret()
	if secret.empty() {
		return profile, nil
	}
	if err := ValidateProfileID(profile.ID); err != nil {
		return Profile{}, err
	}

	if s.secrets == nil || (profile.TokenStorage != TokenStorageKeychain && preferFileTokenStorage()) {
		profile.TokenStorage = TokenStorageFile
		return profile, nil
	}

	if err := s.secrets.Set(profile.ID, secret); err != nil {
		fmt.Fprintf(os.Stderr, "metorial: could not save the sign-in for %s in the system keychain (%v).\nIt is kept in %s, readable only by you, until the keychain works again.\n", profile.ID, err, s.path)
		profile.TokenStorage = TokenStorageFile
		return profile, nil
	}

	profile.TokenStorage = TokenStorageKeychain
	profile.AccessToken = ""
	profile.RefreshToken = ""
	return profile, nil
}

func (s *Store) withTokens(profile Profile) Profile {
	legacyRewrite := profile.TokenStorage == "" && profile.AccessToken == ""
	if s.secrets == nil || (profile.TokenStorage != TokenStorageKeychain && !legacyRewrite) || ValidateProfileID(profile.ID) != nil {
		return profile
	}

	secret, ok, err := s.secrets.Get(profile.ID)
	if err != nil || !ok {
		return profile
	}

	profile.TokenStorage = TokenStorageKeychain
	profile.AccessToken = secret.AccessToken
	profile.RefreshToken = secret.RefreshToken
	if !secret.ExpiresAt.IsZero() {
		profile.ExpiresAt = secret.ExpiresAt
	}
	return profile
}

func writeFileAtomic(path string, content []byte, mode os.FileMode) error {
	temp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)

	if err := temp.Chmod(mode); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(content); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}

	if err := renameReplacing(tempPath, path); err != nil {
		return err
	}
	syncDir(filepath.Dir(path))
	return nil
}

func renameReplacing(from, to string) error {
	err := os.Rename(from, to)
	for attempt := 1; err != nil && runtime.GOOS == "windows" && attempt <= 5; attempt++ {
		time.Sleep(time.Duration(attempt*20) * time.Millisecond)
		err = os.Rename(from, to)
	}
	return err
}

func syncDir(dir string) {
	if runtime.GOOS == "windows" {
		return
	}
	handle, err := os.Open(dir)
	if err != nil {
		return
	}
	_ = handle.Sync()
	_ = handle.Close()
}

func hasLegacyTokens(file File) bool {
	for _, profile := range file.Profiles {
		if profile.TokenStorage == "" && !profile.secret().empty() {
			return true
		}
	}
	return false
}

func OpenProjectStore(cwd string) (*ProjectStore, error) {
	path := filepath.Join(cwd, ".metorial", "config.json")
	store := &ProjectStore{path: path}

	file := ProjectFile{
		Version:           projectConfigVersion,
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
		file.Version = projectConfigVersion
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
		file.Version = projectConfigVersion
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
	return s.CurrentProfileFor(SubjectTypeOrganizationMember)
}

func (s *Store) CurrentProfileFor(subjectType string) (*Profile, bool) {
	file := s.cached
	currentID := file.currentProfileID(subjectType)
	if currentID == "" {
		return nil, false
	}

	profile, ok := file.Profiles[currentID]
	if !ok || profile.Subject() != NormalizeSubjectType(subjectType) {
		return nil, false
	}

	return s.withTokens(profile).Clone(), true
}

func (s *Store) ProfileByID(id string) (*Profile, bool) {
	profile, ok := s.cached.Profiles[strings.TrimSpace(id)]
	if !ok {
		return nil, false
	}

	return s.withTokens(profile).Clone(), true
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

func (s *Store) SortedProfilesFor(subjectType string) []Profile {
	subjectType = NormalizeSubjectType(subjectType)

	profiles := []Profile{}
	for _, profile := range s.SortedProfiles() {
		if profile.Subject() == subjectType {
			profiles = append(profiles, profile)
		}
	}

	return profiles
}

func (s *Store) UpsertProfile(profile Profile, setCurrent bool) error {
	now := time.Now().UTC()

	if profile.ID == "" {
		if profile.Subject() == SubjectTypeConsumer {
			profile.ID = ProfileID(profile.InstanceID, profile.ConsumerProfileID)
		} else {
			profile.ID = ProfileID(profile.OrgID, profile.UserID)
		}
	}
	if err := ValidateProfileID(profile.ID); err != nil {
		return err
	}

	return s.update(func(file *File) error {
		existing, ok := file.Profiles[profile.ID]
		if ok {
			if profile.Name == "" {
				profile.Name = existing.Name
			}
			if profile.CreatedAt.IsZero() {
				profile.CreatedAt = existing.CreatedAt
			}
			if profile.extra == nil {
				profile.extra = existing.extra
			}
		}

		if profile.CreatedAt.IsZero() {
			profile.CreatedAt = now
		}

		profile.UpdatedAt = now
		profile.LastUsedAt = now

		file.Profiles[profile.ID] = profile
		if setCurrent {
			file.setCurrentProfileID(profile.Subject(), profile.ID)
		}

		return nil
	})
}

var ErrProfileNotFound = errors.New("profile not found")

// EditProfile edits the profile as it is stored right now: it never writes back stale tokens or
// brings back a profile signed out meanwhile. The edit does not see tokens kept in the keychain.
func (s *Store) EditProfile(id string, edit func(*Profile)) error {
	return s.editProfile(id, false, edit)
}

func (s *Store) SetCurrentProfile(id string) error {
	return s.editProfile(id, true, func(*Profile) {})
}

func (s *Store) editProfile(id string, setCurrent bool, edit func(*Profile)) error {
	id = strings.TrimSpace(id)
	return s.update(func(file *File) error {
		profile, ok := file.Profiles[id]
		if !ok {
			return ErrProfileNotFound
		}

		edit(&profile)
		profile.ID = id
		now := time.Now().UTC()
		profile.UpdatedAt = now
		profile.LastUsedAt = now
		file.Profiles[id] = profile
		if setCurrent {
			file.setCurrentProfileID(profile.Subject(), id)
		}
		return nil
	})
}

func (s *Store) RemoveProfile(id string) (*Profile, error) {
	id = strings.TrimSpace(id)
	unlock, err := LockRefresh(id)
	if err != nil {
		return nil, fmt.Errorf("metorial: failed to remove profile %s: %w", id, err)
	}
	defer unlock()

	var removed Profile
	err = s.update(func(file *File) error {
		profile, ok := file.Profiles[id]
		if !ok {
			return ErrProfileNotFound
		}

		delete(file.Profiles, id)
		subjectType := profile.Subject()
		if file.currentProfileID(subjectType) == id {
			file.setCurrentProfileID(subjectType, newestProfileID(file.Profiles, subjectType))
		}

		removed = profile
		return nil
	})
	if err != nil {
		return nil, err
	}

	if s.secrets != nil {
		_ = s.secrets.Delete(id)
	}

	return removed.Clone(), nil
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

const claimLinkLifetime = 24 * time.Hour

// ClaimLink signs a fresh claim link with the claim token, so the link printed to terminals and
// agent transcripts stops working on its own. The region comes from the stored link. It falls back
// to the stored link when the profile has no claim token or the stored link has no base to sign onto.
func (p Profile) ClaimLink(now time.Time) string {
	base, err := url.Parse(strings.TrimSpace(p.ClaimURL))
	if err != nil || base.Host == "" || strings.TrimSpace(p.ClaimToken) == "" {
		return p.ClaimURL
	}

	query := base.Query()
	region := claimlink.Region(query.Get("token"))
	query.Set("token", claimlink.Sign(p.ClaimToken, claimlink.PurposeClaim, region, now.Add(claimLinkLifetime)))
	base.RawQuery = query.Encode()

	return base.String()
}

func (p Profile) Subject() string {
	return NormalizeSubjectType(p.SubjectType)
}

func NormalizeSubjectType(subjectType string) string {
	if strings.TrimSpace(subjectType) == SubjectTypeConsumer {
		return SubjectTypeConsumer
	}
	return SubjectTypeOrganizationMember
}

func ProfileID(scopeID, subjectID string) string {
	return strings.TrimSpace(scopeID) + ":" + strings.TrimSpace(subjectID)
}

func (f File) currentProfileID(subjectType string) string {
	if NormalizeSubjectType(subjectType) == SubjectTypeConsumer {
		return f.CurrentConsumerProfileID
	}
	return f.CurrentProfileID
}

func (f *File) setCurrentProfileID(subjectType, id string) {
	if NormalizeSubjectType(subjectType) == SubjectTypeConsumer {
		f.CurrentConsumerProfileID = id
		return
	}
	f.CurrentProfileID = id
}

func defaultFile() File {
	return File{
		Version:  ConfigVersion,
		Profiles: map[string]Profile{},
	}
}

func (p Profile) secret() Secret {
	return Secret{
		AccessToken:  p.AccessToken,
		RefreshToken: p.RefreshToken,
		ExpiresAt:    p.ExpiresAt,
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

func newestProfileID(profiles map[string]Profile, subjectType string) string {
	type pair struct {
		id      string
		profile Profile
	}

	subjectType = NormalizeSubjectType(subjectType)
	items := make([]pair, 0, len(profiles))
	for id, profile := range profiles {
		if profile.Subject() == subjectType {
			items = append(items, pair{id: id, profile: profile})
		}
	}
	if len(items) == 0 {
		return ""
	}

	sort.SliceStable(items, func(left, right int) bool {
		if items[left].profile.LastUsedAt.Equal(items[right].profile.LastUsedAt) {
			return items[left].id < items[right].id
		}
		return items[left].profile.LastUsedAt.After(items[right].profile.LastUsedAt)
	})

	return items[0].id
}
