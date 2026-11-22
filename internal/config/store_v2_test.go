package config

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"
)

type memorySecrets struct {
	mu    sync.Mutex
	items map[string]Secret
}

func useMemorySecrets(t *testing.T) *memorySecrets {
	t.Helper()
	secrets := &memorySecrets{items: map[string]Secret{}}
	previous := newSecrets
	newSecrets = func() Secrets { return secrets }
	t.Cleanup(func() { newSecrets = previous })
	t.Setenv(EnvCredentialStorage, TokenStorageKeychain)
	return secrets
}

func (m *memorySecrets) Get(profileID string) (Secret, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	secret, ok := m.items[profileID]
	return secret, ok, nil
}

func (m *memorySecrets) Set(profileID string, secret Secret) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items[profileID] = secret
	return nil
}

func (m *memorySecrets) Delete(profileID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.items, profileID)
	return nil
}

func readRawConfig(t *testing.T) map[string]any {
	t.Helper()
	path, err := DefaultConfigPath()
	if err != nil {
		t.Fatalf("DefaultConfigPath() error = %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	raw := map[string]any{}
	if err := json.Unmarshal(content, &raw); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	return raw
}

func writeRawConfig(t *testing.T, content string) {
	t.Helper()
	path, err := DefaultConfigPath()
	if err != nil {
		t.Fatalf("DefaultConfigPath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func rawProfile(t *testing.T, raw map[string]any, id string) map[string]any {
	t.Helper()
	profile, ok := raw["profiles"].(map[string]any)[id].(map[string]any)
	if !ok {
		t.Fatalf("profile %q missing from %v", id, raw["profiles"])
	}
	return profile
}

func consumerProfile(access string) Profile {
	return Profile{
		Name:              "Ada",
		APIHost:           "https://api.metorial.com",
		ClientID:          "client_1",
		AccessToken:       access,
		RefreshToken:      "refresh-" + access,
		ExpiresAt:         time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC),
		SubjectType:       SubjectTypeConsumer,
		InstanceID:        "ins_1",
		ConsumerProfileID: "cpr_1",
	}
}

func TestKeychainProfilesKeepTokensOutOfTheFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	secrets := useMemorySecrets(t)

	store, err := OpenStore()
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	if err := store.UpsertProfile(consumerProfile("access_1"), true); err != nil {
		t.Fatalf("UpsertProfile() error = %v", err)
	}

	raw := readRawConfig(t)
	if raw["version"] != float64(ConfigVersion) {
		t.Fatalf("version = %v, want %d", raw["version"], ConfigVersion)
	}
	stored := rawProfile(t, raw, "ins_1:cpr_1")
	if stored["access_token"] != "" || stored["refresh_token"] != "" || stored["token_storage"] != TokenStorageKeychain {
		t.Fatalf("stored profile = %v, want empty tokens in keychain storage", stored)
	}
	if secret := secrets.items["ins_1:cpr_1"]; secret.AccessToken != "access_1" || secret.RefreshToken != "refresh-access_1" {
		t.Fatalf("keychain secret = %+v", secret)
	}

	reloaded, err := OpenStore()
	if err != nil {
		t.Fatalf("OpenStore() reload error = %v", err)
	}
	current, ok := reloaded.CurrentProfileFor(SubjectTypeConsumer)
	if !ok || current.AccessToken != "access_1" || current.RefreshToken != "refresh-access_1" {
		t.Fatalf("CurrentProfileFor() = %+v, %v", current, ok)
	}

	if _, err := reloaded.RemoveProfile(current.ID); err != nil {
		t.Fatalf("RemoveProfile() error = %v", err)
	}
	if _, ok := secrets.items["ins_1:cpr_1"]; ok {
		t.Fatalf("RemoveProfile() left the keychain item behind")
	}
}

func TestLegacyTokensMoveToTheKeychainOnOpen(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	secrets := useMemorySecrets(t)
	writeRawConfig(t, `{"version":1,"current_profile_id":"org_1:usr_1","profiles":{"org_1:usr_1":{"id":"org_1:usr_1","name":"legacy","org_id":"org_1","user_id":"usr_1","client_id":"c","access_token":"a","refresh_token":"r","expires_at":"2030-01-01T00:00:00Z"}}}`)

	store, err := OpenStore()
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}

	raw := readRawConfig(t)
	stored := rawProfile(t, raw, "org_1:usr_1")
	if raw["version"] != float64(ConfigVersion) || stored["access_token"] != "" || stored["token_storage"] != TokenStorageKeychain {
		t.Fatalf("migrated file = %v", raw)
	}
	if secrets.items["org_1:usr_1"].AccessToken != "a" {
		t.Fatalf("keychain secret = %+v", secrets.items["org_1:usr_1"])
	}

	current, ok := store.CurrentProfile()
	if !ok || current.AccessToken != "a" || current.RefreshToken != "r" {
		t.Fatalf("CurrentProfile() = %+v, %v", current, ok)
	}
}

func TestFileTokenStorageWhenRequested(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	secrets := useMemorySecrets(t)
	t.Setenv(EnvCredentialStorage, TokenStorageFile)

	store, err := OpenStore()
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	if err := store.UpsertProfile(consumerProfile("access_1"), true); err != nil {
		t.Fatalf("UpsertProfile() error = %v", err)
	}

	stored := rawProfile(t, readRawConfig(t), "ins_1:cpr_1")
	if stored["access_token"] != "access_1" || stored["token_storage"] != TokenStorageFile {
		t.Fatalf("stored profile = %v, want plaintext file storage", stored)
	}
	if len(secrets.items) != 0 {
		t.Fatalf("keychain items = %v, want none", secrets.items)
	}
}

func TestUnknownFieldsSurviveWrites(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	useMemorySecrets(t)
	writeRawConfig(t, `{"version":2,"desktop_note":{"a":1},"settings":{"default_api_host":"","default_format":"","theme":"dark"},"profiles":{"ins_1:cpr_1":{"id":"ins_1:cpr_1","subject_type":"consumer","token_storage":"keychain","desktop_only":true}}}`)

	store, err := OpenStore()
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	member := Profile{Name: "member", OrgID: "org_1", UserID: "usr_1"}
	if err := store.UpsertProfile(member, true); err != nil {
		t.Fatalf("UpsertProfile() error = %v", err)
	}
	if err := store.SetCurrentProfile("ins_1:cpr_1"); err != nil {
		t.Fatalf("SetCurrentProfile() error = %v", err)
	}

	raw := readRawConfig(t)
	if note, ok := raw["desktop_note"].(map[string]any); !ok || note["a"] != float64(1) {
		t.Fatalf("top-level extra lost: %v", raw)
	}
	if raw["settings"].(map[string]any)["theme"] != "dark" {
		t.Fatalf("settings extra lost: %v", raw["settings"])
	}
	if rawProfile(t, raw, "ins_1:cpr_1")["desktop_only"] != true {
		t.Fatalf("profile extra lost: %v", raw["profiles"])
	}
}

func TestStaleCopiesDoNotOverwriteRotatedTokens(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	secrets := useMemorySecrets(t)

	first, err := OpenStore()
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	if err := first.UpsertProfile(consumerProfile("access_1"), true); err != nil {
		t.Fatalf("UpsertProfile() error = %v", err)
	}
	stale, _ := first.CurrentProfileFor(SubjectTypeConsumer)

	second, err := OpenStore()
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	rotated, _ := second.CurrentProfileFor(SubjectTypeConsumer)
	rotated.AccessToken = "access_2"
	rotated.RefreshToken = "refresh-access_2"
	if err := second.UpsertProfile(*rotated, true); err != nil {
		t.Fatalf("UpsertProfile(rotated) error = %v", err)
	}

	stale.PortalName = "Renamed"
	if err := first.UpsertProfile(*stale, false); err != nil {
		t.Fatalf("UpsertProfile(stale) error = %v", err)
	}

	if secret := secrets.items["ins_1:cpr_1"]; secret.AccessToken != "access_2" || secret.RefreshToken != "refresh-access_2" {
		t.Fatalf("keychain secret = %+v, want the rotated tokens", secret)
	}
	reloaded, _ := OpenStore()
	current, _ := reloaded.CurrentProfileFor(SubjectTypeConsumer)
	if current.PortalName != "Renamed" || current.AccessToken != "access_2" {
		t.Fatalf("CurrentProfileFor() = %+v", current)
	}
}

func TestRefreshLockIsExclusive(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := os.MkdirAll(filepath.Join(os.Getenv("HOME"), ".metorial", "cli"), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	unlock, err := LockRefresh("ins_1:cpr_1")
	if err != nil {
		t.Fatalf("LockRefresh() error = %v", err)
	}
	path, _ := refreshLockPath("ins_1:cpr_1")
	if filepath.Base(path) != "refresh-ins_1_cpr_1.lock" {
		t.Fatalf("refreshLockPath() = %q", path)
	}
	if _, err := lockPath(path, 100*time.Millisecond); err == nil {
		t.Fatalf("second lock succeeded while the first was held")
	}
	unlock()

	again, err := lockPath(path, time.Second)
	if err != nil {
		t.Fatalf("lock after unlock error = %v", err)
	}
	again()
}

const writerProcessEnv = "METORIAL_CONFIG_TEST_WRITER"

func TestConcurrentWriterProcessesKeepEveryProfile(t *testing.T) {
	home := t.TempDir()
	const writers, perWriter = 4, 15

	var wait sync.WaitGroup
	failures := make(chan error, writers)
	for writer := 0; writer < writers; writer++ {
		wait.Add(1)
		go func(writer int) {
			defer wait.Done()
			command := exec.Command(os.Args[0], "-test.run=^TestConfigWriterProcess$")
			command.Env = append(os.Environ(),
				"HOME="+home,
				writerProcessEnv+"="+strconv.Itoa(writer),
				EnvCredentialStorage+"="+TokenStorageFile,
			)
			if output, err := command.CombinedOutput(); err != nil {
				failures <- fmt.Errorf("writer %d: %v\n%s", writer, err, output)
			}
		}(writer)
	}
	wait.Wait()
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}

	t.Setenv("HOME", home)
	store, err := OpenStore()
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	if got := len(store.SortedProfiles()); got != writers*perWriter {
		t.Fatalf("profiles = %d, want %d (lost updates)", got, writers*perWriter)
	}
}

func TestConfigWriterProcess(t *testing.T) {
	writer := os.Getenv(writerProcessEnv)
	if writer == "" {
		t.Skip("helper for TestConcurrentWriterProcessesKeepEveryProfile")
	}

	for index := 0; index < 15; index++ {
		store, err := OpenStore()
		if err != nil {
			t.Fatalf("OpenStore() error = %v", err)
		}
		profile := Profile{
			Name:        "writer",
			OrgID:       "org_" + writer,
			UserID:      "usr_" + strconv.Itoa(index),
			AccessToken: "access",
		}
		if err := store.UpsertProfile(profile, true); err != nil {
			t.Fatalf("UpsertProfile() error = %v", err)
		}
	}
}
