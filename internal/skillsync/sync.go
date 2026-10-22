package skillsync

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/metorial/cli/internal/skillsapi"
)

const (
	maxBatchChanges = 100
	skillFilePath   = "/SKILL.md"
)

var validPathSegment = regexp.MustCompile(`^[A-Za-z0-9._() $-]+$`)

type ChangeKind string

const (
	ChangeAdded    ChangeKind = "added"
	ChangeModified ChangeKind = "modified"
	ChangeDeleted  ChangeKind = "deleted"
)

type Change struct {
	Path string     `json:"path"`
	Kind ChangeKind `json:"kind"`
}

type Status struct {
	SkillID   string   `json:"skill_id"`
	SkillSlug string   `json:"skill_slug,omitempty"`
	Local     []Change `json:"local"`
	Remote    []Change `json:"remote"`
}

func (s Status) Conflicts() []string {
	remote := map[string]bool{}
	for _, change := range s.Remote {
		remote[change.Path] = true
	}

	conflicts := []string{}
	for _, change := range s.Local {
		if remote[change.Path] {
			conflicts = append(conflicts, change.Path)
		}
	}
	return conflicts
}

func (s Status) Conflicted() bool {
	return len(s.Conflicts()) > 0
}

func (s Status) Clean() bool {
	return len(s.Local) == 0 && len(s.Remote) == 0
}

type PullOptions struct {
	Force bool
}

type PullResult struct {
	Skill   skillsapi.Skill `json:"skill"`
	Dir     string          `json:"dir"`
	Written []string        `json:"written"`
	Removed []string        `json:"removed"`
	Skipped []string        `json:"skipped"`
}

type PushOptions struct {
	Force  bool
	DryRun bool
}

type PushResult struct {
	SkillID string   `json:"skill_id"`
	DryRun  bool     `json:"dry_run"`
	Changes []Change `json:"changes"`
}

var ErrRemoteChanged = errors.New("metorial: the skill was changed remotely since your last pull\nRun the skills pull command to update your copy, or push with --force to overwrite the remote changes")

// Stale files are removed first so case-only renames or file/folder swaps can't clobber new writes.
// A partial failure still updates the manifest so a retry doesn't see local changes.
func Pull(client *skillsapi.Client, skill *skillsapi.Skill, dir string, options PullOptions) (*PullResult, error) {
	root, err := resolveDir(dir)
	if err != nil {
		return nil, err
	}

	previous, err := LoadManifest(root)
	if err != nil && !errors.Is(err, ErrNoManifest) {
		return nil, err
	}

	if previous != nil {
		if previous.SkillID != skill.ID {
			return nil, fmt.Errorf("metorial: %s already contains skill %s; pull %s into a different directory", dir, previous.SkillID, skill.ID)
		}

		local, err := localChanges(root, previous)
		if err != nil {
			return nil, err
		}
		if len(local) > 0 && !options.Force {
			return nil, fmt.Errorf("metorial: %s has %d local change(s) that a pull would overwrite\nPush them first, or pull with --force to discard them", dir, len(local))
		}
	} else if !options.Force {
		if err := assertEmptyDir(root); err != nil {
			return nil, err
		}
	}

	rules, err := loadIgnoreRules(root)
	if err != nil {
		return nil, err
	}

	files, err := client.ListFiles(skill.ID)
	if err != nil {
		return nil, err
	}

	manifest := &Manifest{
		SkillID: skill.ID,
		APIHost: client.APIHost(),
		Files:   map[string]ManifestFile{},
	}
	if skill.Slug != nil {
		manifest.SkillSlug = *skill.Slug
	}

	result := &PullResult{Skill: *skill, Dir: dir, Written: []string{}, Removed: []string{}, Skipped: []string{}}

	targets := map[string]string{}
	remotePaths := []string{}
	for _, file := range files.Items {
		if rules.matches(file.Path) {
			result.Skipped = append(result.Skipped, file.Path)
			continue
		}

		target, err := localPath(root, file.Path)
		if err != nil {
			return nil, err
		}
		targets[file.Path] = target
		remotePaths = append(remotePaths, file.Path)
	}
	if err := assertNoCaseCollisions(remotePaths); err != nil {
		return nil, err
	}
	if _, remoteIgnore := targets["/"+ignoreFile]; remoteIgnore && previous == nil && !options.Force {
		if _, err := os.Stat(filepath.Join(root, ignoreFile)); err == nil {
			return nil, fmt.Errorf("metorial: the skill has its own %s, which would replace the one in %s\nMove yours aside and pull again, or pull with --force to replace it", ignoreFile, dir)
		}
	}

	removals := []string{}
	if previous != nil {
		for trackedPath, tracked := range previous.Files {
			if rules.matches(trackedPath) {
				continue
			}
			manifest.Files[trackedPath] = tracked
			if _, stillRemote := targets[trackedPath]; !stillRemote {
				removals = append(removals, trackedPath)
			}
		}
	}
	sort.Strings(removals)

	changed := false
	fail := func(cause error) (*PullResult, error) {
		if !changed {
			return nil, cause
		}
		if err := SaveManifest(root, manifest); err != nil {
			return nil, errors.Join(cause, err)
		}
		return nil, cause
	}

	for _, trackedPath := range removals {
		target, err := localPath(root, trackedPath)
		if err != nil {
			return fail(err)
		}
		if err := removeFile(root, target); err != nil {
			return fail(fmt.Errorf("metorial: failed to remove %s: %w", trackedPath, err))
		}

		delete(manifest.Files, trackedPath)
		result.Removed = append(result.Removed, trackedPath)
		changed = true
	}

	for _, file := range files.Items {
		target, ok := targets[file.Path]
		if !ok {
			continue
		}

		remote, content, err := client.ReadFile(skill.ID, file.Path)
		if err != nil {
			return fail(err)
		}

		content = skillsapi.RestoreTrailingNewline(remote.Kind, content)

		if !unchangedSymlink(target, content) {
			if err := writeFileAtomic(target, content); err != nil {
				return fail(fmt.Errorf("metorial: failed to write %s: %w", file.Path, err))
			}
		}

		manifest.Files[file.Path] = ManifestFile{Etag: remote.Etag, SHA256: hashBytes(content)}
		result.Written = append(result.Written, file.Path)
		changed = true
	}

	return result, SaveManifest(root, manifest)
}

func assertNoCaseCollisions(remotePaths []string) error {
	seen := map[string]string{}
	for _, remotePath := range remotePaths {
		segments := strings.Split(strings.TrimPrefix(remotePath, "/"), "/")
		for index := range segments {
			prefix := "/" + strings.Join(segments[:index+1], "/")
			lower := strings.ToLower(prefix)
			if other, ok := seen[lower]; ok && other != prefix {
				return fmt.Errorf("metorial: %s and %s differ only in letter case and would overwrite each other on case-insensitive file systems\nRename one of them in the skill before pulling", other, prefix)
			}
			seen[lower] = prefix
		}
	}
	return nil
}

// Prunes emptied folders so a remote folder-to-file change can be written.
func removeFile(root string, target string) error {
	if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	for dir := filepath.Dir(target); strings.HasPrefix(dir, root+string(filepath.Separator)); dir = filepath.Dir(dir) {
		if os.Remove(dir) != nil {
			break
		}
	}
	return nil
}

func unchangedSymlink(target string, content []byte) bool {
	info, err := os.Lstat(target)
	if err != nil || info.Mode()&fs.ModeSymlink == 0 {
		return false
	}

	existing, err := os.ReadFile(target)
	return err == nil && bytes.Equal(existing, content)
}

func localChanges(dir string, manifest *Manifest) ([]Change, error) {
	rules, err := loadIgnoreRules(dir)
	if err != nil {
		return nil, err
	}

	localFiles, err := scanLocalFiles(dir, rules)
	if err != nil {
		return nil, err
	}

	changes := []Change{}
	for filePath, hash := range localFiles {
		tracked, ok := manifest.Files[filePath]
		if !ok {
			changes = append(changes, Change{Path: filePath, Kind: ChangeAdded})
			continue
		}
		if tracked.SHA256 != hash {
			changes = append(changes, Change{Path: filePath, Kind: ChangeModified})
		}
	}
	for filePath := range manifest.Files {
		if _, ok := localFiles[filePath]; !ok && !rules.matches(filePath) {
			changes = append(changes, Change{Path: filePath, Kind: ChangeDeleted})
		}
	}

	sortChanges(changes)
	return changes, nil
}

func remoteChanges(client *skillsapi.Client, dir string, manifest *Manifest) ([]Change, error) {
	rules, err := loadIgnoreRules(dir)
	if err != nil {
		return nil, err
	}

	remote, err := remoteEtags(client, manifest.SkillID)
	if err != nil {
		return nil, err
	}

	for filePath := range remote {
		if rules.matches(filePath) {
			delete(remote, filePath)
		}
	}

	changes := []Change{}
	for filePath, etag := range remote {
		tracked, ok := manifest.Files[filePath]
		if !ok {
			changes = append(changes, Change{Path: filePath, Kind: ChangeAdded})
			continue
		}
		if tracked.Etag != etag {
			changes = append(changes, Change{Path: filePath, Kind: ChangeModified})
		}
	}
	for filePath := range manifest.Files {
		if _, ok := remote[filePath]; !ok && !rules.matches(filePath) {
			changes = append(changes, Change{Path: filePath, Kind: ChangeDeleted})
		}
	}

	sortChanges(changes)
	return changes, nil
}

func GetStatus(client *skillsapi.Client, dir string) (*Status, error) {
	root, err := resolveDir(dir)
	if err != nil {
		return nil, err
	}

	manifest, err := loadManifestFor(client, root)
	if err != nil {
		return nil, err
	}

	local, err := localChanges(root, manifest)
	if err != nil {
		return nil, err
	}

	remote, err := remoteChanges(client, root, manifest)
	if err != nil {
		return nil, err
	}

	return &Status{SkillID: manifest.SkillID, SkillSlug: manifest.SkillSlug, Local: local, Remote: remote}, nil
}

// The server stops a batch at the first failure but keeps earlier changes, so each batch is
// pre-checked for remote changes and etag-guarded, and partial results are recorded.
func Push(client *skillsapi.Client, dir string, options PushOptions) (*PushResult, error) {
	root, err := resolveDir(dir)
	if err != nil {
		return nil, err
	}

	manifest, err := loadManifestFor(client, root)
	if err != nil {
		return nil, err
	}

	changes, err := localChanges(root, manifest)
	if err != nil {
		return nil, err
	}

	if err := assertValidPaths(changes); err != nil {
		return nil, err
	}
	if err := assertKeepsSkillFile(changes); err != nil {
		return nil, err
	}
	if err := assertValidContent(root, changes); err != nil {
		return nil, err
	}

	result := &PushResult{SkillID: manifest.SkillID, DryRun: options.DryRun, Changes: changes}
	if options.DryRun || len(changes) == 0 {
		return result, nil
	}

	pushed := 0
	for start := 0; start < len(changes); start += maxBatchChanges {
		batch := changes[start:min(start+maxBatchChanges, len(changes))]

		before, err := remoteEtags(client, manifest.SkillID)
		if err != nil {
			return nil, partialPushError(err, pushed, len(changes))
		}
		if !options.Force {
			if err := assertRemoteUnchanged(manifest, batch, before); err != nil {
				return nil, partialPushError(err, pushed, len(changes))
			}
		}

		inputs, hashes, err := buildChangeInputs(client, root, manifest, batch, options.Force)
		if err != nil {
			return nil, partialPushError(err, pushed, len(changes))
		}

		applied, err := client.ApplyChanges(manifest.SkillID, inputs)
		if err != nil {
			return nil, recordPartialPush(client, root, manifest, batch, hashes, before, pushed, len(changes), err)
		}

		for index, change := range batch {
			if change.Kind == ChangeDeleted {
				delete(manifest.Files, change.Path)
				continue
			}

			etag := ""
			if index < len(applied.Items) && applied.Items[index].File != nil {
				etag = applied.Items[index].File.Etag
			}
			manifest.Files[change.Path] = ManifestFile{Etag: etag, SHA256: hashes[change.Path]}
		}

		pushed += len(batch)
		if err := SaveManifest(root, manifest); err != nil {
			return nil, partialPushError(err, pushed, len(changes))
		}
	}

	return result, nil
}

func remoteEtags(client *skillsapi.Client, skillID string) (map[string]string, error) {
	files, err := client.ListFiles(skillID)
	if err != nil {
		return nil, err
	}

	etags := map[string]string{}
	for _, file := range files.Items {
		etags[file.Path] = file.Etag
	}
	return etags, nil
}

func assertRemoteUnchanged(manifest *Manifest, batch []Change, remote map[string]string) error {
	for _, change := range batch {
		etag, exists := remote[change.Path]
		tracked, isTracked := manifest.Files[change.Path]
		if isTracked && (!exists || etag != tracked.Etag) || !isTracked && exists {
			return ErrRemoteChanged
		}
	}
	return nil
}

func assertKeepsSkillFile(changes []Change) error {
	for _, change := range changes {
		if change.Kind == ChangeDeleted && change.Path == skillFilePath {
			return fmt.Errorf("metorial: this push would delete %s, which every skill needs\nRestore the file, or pull again to get the remote copy back", skillFilePath)
		}
	}
	return nil
}

func partialPushError(cause error, applied int, total int) error {
	if applied == 0 {
		return cause
	}
	return fmt.Errorf("%w\nThe push stopped partway: %d of %d change(s) were applied; run the skills status command to see what is left", cause, applied, total)
}

// Records what a failed batch applied anyway; documents may have lost their trailing newline.
func recordPartialPush(client *skillsapi.Client, dir string, manifest *Manifest, batch []Change, hashes map[string]string, before map[string]string, pushed int, total int, cause error) error {
	if skillsapi.IsConflict(cause) {
		cause = ErrRemoteChanged
	}

	after, err := remoteEtags(client, manifest.SkillID)
	if err != nil {
		return fmt.Errorf("%w\nPart of the push may have been applied; run the skills status command before pushing again", cause)
	}

	applied := 0
	for _, change := range batch {
		etag, exists := after[change.Path]
		if change.Kind == ChangeDeleted {
			if !exists {
				delete(manifest.Files, change.Path)
				applied++
			}
			continue
		}

		if previous, existed := before[change.Path]; !exists || existed && previous == etag {
			continue
		}
		if remoteMatches(client, manifest.SkillID, change.Path, hashes[change.Path]) {
			manifest.Files[change.Path] = ManifestFile{Etag: etag, SHA256: hashes[change.Path]}
			applied++
		}
	}

	if err := SaveManifest(dir, manifest); err != nil {
		return err
	}

	return partialPushError(cause, pushed+applied, total)
}

func remoteMatches(client *skillsapi.Client, skillID string, filePath string, hash string) bool {
	remote, content, err := client.ReadFile(skillID, filePath)
	if err != nil {
		return false
	}
	return hashBytes(content) == hash || hashBytes(skillsapi.RestoreTrailingNewline(remote.Kind, content)) == hash
}

func buildChangeInputs(client *skillsapi.Client, dir string, manifest *Manifest, changes []Change, force bool) ([]skillsapi.ChangeInput, map[string]string, error) {
	inputs := make([]skillsapi.ChangeInput, 0, len(changes))
	hashes := map[string]string{}

	for _, change := range changes {
		input := skillsapi.ChangeInput{Path: change.Path}

		if !force {
			if tracked, ok := manifest.Files[change.Path]; ok {
				input.IfMatch = skillsapi.MatchEtag(tracked.Etag)
			} else {
				input.IfMatch = skillsapi.MatchAbsent()
			}
		}

		if change.Kind == ChangeDeleted {
			input.Type = skillsapi.ChangeTypeDelete
			inputs = append(inputs, input)
			continue
		}

		target, err := localPath(dir, change.Path)
		if err != nil {
			return nil, nil, err
		}
		content, err := os.ReadFile(target)
		if err != nil {
			return nil, nil, fmt.Errorf("metorial: failed to read %s: %w", change.Path, err)
		}

		input.Type = skillsapi.ChangeTypePut
		hashes[change.Path] = hashBytes(content)

		isText, err := skillsapi.IsText(change.Path, content)
		if err != nil {
			return nil, nil, err
		}
		if isText {
			text := string(content)
			input.Content = &text
		} else {
			uploaded, err := client.UploadFile(path.Base(change.Path), content)
			if err != nil {
				return nil, nil, fmt.Errorf("metorial: failed to upload %s: %w", change.Path, err)
			}
			input.FileID = uploaded.ID
		}

		inputs = append(inputs, input)
	}

	return inputs, hashes, nil
}

func resolveDir(dir string) (string, error) {
	absolute, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("metorial: failed to resolve %s: %w", dir, err)
	}

	resolved, err := filepath.EvalSymlinks(absolute)
	if errors.Is(err, os.ErrNotExist) {
		return absolute, nil
	}
	if err != nil {
		return "", fmt.Errorf("metorial: failed to resolve %s: %w", dir, err)
	}
	return resolved, nil
}

// Non-file symlinks are an error: skipping them would make them look deleted.
func scanLocalFiles(dir string, rules ignoreRules) (map[string]string, error) {
	files := map[string]string{}

	err := filepath.WalkDir(dir, func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if current == dir {
			return nil
		}

		remote, err := remotePath(dir, current)
		if err != nil {
			return err
		}

		if rules.matches(remote) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			if info, err := os.Stat(current); err != nil || !info.Mode().IsRegular() {
				return fmt.Errorf("%s is a symbolic link that does not point to a regular file; replace it with a file or remove it", remote)
			}
		} else if !entry.Type().IsRegular() {
			return nil
		}

		content, err := os.ReadFile(current)
		if err != nil {
			return err
		}

		files[remote] = hashBytes(content)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("metorial: failed to scan %s: %w", dir, err)
	}

	return files, nil
}

func assertValidPaths(changes []Change) error {
	invalid := []string{}
	for _, change := range changes {
		for _, segment := range strings.Split(strings.TrimPrefix(change.Path, "/"), "/") {
			if !validPathSegment.MatchString(segment) || strings.Trim(segment, ".") == "" {
				invalid = append(invalid, change.Path)
				break
			}
		}
	}

	if len(invalid) == 0 {
		return nil
	}

	return fmt.Errorf("metorial: these paths contain characters skills do not support (allowed: letters, numbers, spaces and . _ - ( ) $):\n  %s", strings.Join(invalid, "\n  "))
}

// Runs before the first batch so a rejected file can't stop a half-applied push.
func assertValidContent(dir string, changes []Change) error {
	problems := []error{}
	for _, change := range changes {
		if change.Kind == ChangeDeleted {
			continue
		}

		target, err := localPath(dir, change.Path)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(target)
		if err != nil {
			return fmt.Errorf("metorial: failed to read %s: %w", change.Path, err)
		}
		if _, err := skillsapi.IsText(change.Path, content); err != nil {
			problems = append(problems, err)
		}
	}
	return errors.Join(problems...)
}

func assertEmptyDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.Name() != ignoreFile && !ignoredSegment(entry.Name()) {
			return fmt.Errorf("metorial: %s is not empty; pull into an empty directory or pass --force", dir)
		}
	}

	return nil
}

func sortChanges(changes []Change) {
	sort.Slice(changes, func(i, j int) bool {
		return changes[i].Path < changes[j].Path
	})
}
