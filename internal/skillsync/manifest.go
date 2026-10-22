package skillsync

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/metorial/cli/internal/skillsapi"
)

const (
	manifestDir  = ".metorial"
	manifestFile = "skill.json"
)

type Manifest struct {
	SkillID   string                  `json:"skill_id"`
	SkillSlug string                  `json:"skill_slug,omitempty"`
	APIHost   string                  `json:"api_host,omitempty"`
	Files     map[string]ManifestFile `json:"files"`
}

type ManifestFile struct {
	Etag   string `json:"etag"`
	SHA256 string `json:"sha256"`
}

var ErrNoManifest = errors.New("metorial: this directory is not a pulled skill (no .metorial/skill.json)\nRun the skills pull command first")

func manifestPath(dir string) string {
	return filepath.Join(dir, manifestDir, manifestFile)
}

func LoadManifest(dir string) (*Manifest, error) {
	content, err := os.ReadFile(manifestPath(dir))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNoManifest
	}
	if err != nil {
		return nil, fmt.Errorf("metorial: failed to read skill manifest: %w", err)
	}

	manifest := &Manifest{}
	if err := json.Unmarshal(content, manifest); err != nil {
		return nil, fmt.Errorf("metorial: the skill manifest at %s is invalid: %w", manifestPath(dir), err)
	}
	if manifest.Files == nil {
		manifest.Files = map[string]ManifestFile{}
	}

	return manifest, nil
}

func loadManifestFor(client *skillsapi.Client, dir string) (*Manifest, error) {
	manifest, err := LoadManifest(dir)
	if err == nil && manifest.APIHost != "" && manifest.APIHost != client.APIHost() {
		return nil, fmt.Errorf("metorial: %s was pulled from %s, but the current profile uses %s", dir, manifest.APIHost, client.APIHost())
	}
	return manifest, err
}

func SaveManifest(dir string, manifest *Manifest) error {
	content, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("metorial: failed to write skill manifest: %w", err)
	}

	if err := os.MkdirAll(filepath.Join(dir, manifestDir), 0o755); err != nil {
		return fmt.Errorf("metorial: failed to create %s: %w", manifestDir, err)
	}

	if err := writeFileAtomic(manifestPath(dir), append(content, '\n')); err != nil {
		return fmt.Errorf("metorial: failed to write skill manifest: %w", err)
	}
	return nil
}

func hashBytes(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func localPath(dir string, remotePath string) (string, error) {
	if strings.ContainsAny(remotePath, `\:`) {
		return "", fmt.Errorf("metorial: refusing to sync %q: skill paths cannot contain \\ or :", remotePath)
	}

	target := filepath.Join(dir, filepath.FromSlash(strings.TrimPrefix(remotePath, "/")))
	relative, err := filepath.Rel(dir, target)
	if err != nil || relative == "." || relative == ".." || filepath.IsAbs(relative) || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("metorial: refusing to sync %q: it points outside of %s", remotePath, dir)
	}

	return target, nil
}

func remotePath(dir string, local string) (string, error) {
	relative, err := filepath.Rel(dir, local)
	if err != nil {
		return "", err
	}

	return "/" + filepath.ToSlash(relative), nil
}

// Refuses symlinks: status and push follow links, so replacing one detaches the local copy.
func writeFileAtomic(target string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}

	mode := fs.FileMode(0o644)
	if info, err := os.Lstat(target); err == nil {
		if info.Mode()&fs.ModeSymlink != 0 {
			return errors.New("it is a symbolic link, and replacing it would break the link; replace the link with a regular file or remove it, then try again")
		}
		mode = info.Mode().Perm()
	}

	temp, err := os.CreateTemp(filepath.Dir(target), ".metorial-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()

	if err := temp.Chmod(mode); err != nil {
		_ = temp.Close()
		_ = os.Remove(tempName)
		return err
	}
	if _, err := temp.Write(content); err != nil {
		_ = temp.Close()
		_ = os.Remove(tempName)
		return err
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempName)
		return err
	}

	return os.Rename(tempName, target)
}
