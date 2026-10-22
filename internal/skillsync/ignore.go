package skillsync

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const (
	ignoreFile        = ".metorialignore"
	maxIgnorePatterns = 30
)

var ignoredNames = map[string]bool{
	manifestDir:    true,
	".git":         true,
	".DS_Store":    true,
	"node_modules": true,
	".idea":        true,
	".vscode":      true,
	".env":         true,
}

var ignoredNamePatterns = []string{".metorial-*", ".env.*", "*.swp", "*~"}

// Patterns with a slash are anchored to the skill root and also match folder prefixes.
type ignoreRules struct {
	names []string
	paths []string
}

// Over the limit is an error: dropping rules could push or delete ignored files.
func loadIgnoreRules(dir string) (ignoreRules, error) {
	content, err := os.ReadFile(filepath.Join(dir, ignoreFile))
	if errors.Is(err, os.ErrNotExist) {
		return ignoreRules{}, nil
	}
	if err != nil {
		return ignoreRules{}, fmt.Errorf("metorial: failed to read %s: %w", ignoreFile, err)
	}

	rules := ignoreRules{}
	scanner := bufio.NewScanner(bytes.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		pattern := strings.TrimRight(line, "/")
		switch {
		case pattern == "":
			continue
		case strings.Contains(pattern, "/"):
			rules.paths = append(rules.paths, strings.TrimPrefix(pattern, "/"))
		default:
			rules.names = append(rules.names, pattern)
		}
	}
	if count := len(rules.names) + len(rules.paths); count > maxIgnorePatterns {
		return ignoreRules{}, fmt.Errorf("metorial: %s has %d patterns; the limit is %d\nCombine some of them with wildcards, or remove the ones you no longer need", ignoreFile, count, maxIgnorePatterns)
	}

	return rules, nil
}

func ignoredSegment(name string) bool {
	if ignoredNames[name] {
		return true
	}
	for _, pattern := range ignoredNamePatterns {
		if matched, _ := path.Match(pattern, name); matched {
			return true
		}
	}
	return false
}

func (r ignoreRules) matches(remotePath string) bool {
	relative := strings.TrimPrefix(remotePath, "/")
	segments := strings.Split(relative, "/")

	for index, segment := range segments {
		if ignoredSegment(segment) {
			return true
		}
		for _, pattern := range r.names {
			if matched, _ := path.Match(pattern, segment); matched {
				return true
			}
		}

		prefix := strings.Join(segments[:index+1], "/")
		for _, pattern := range r.paths {
			if matched, _ := path.Match(pattern, prefix); matched {
				return true
			}
		}
	}

	return false
}
