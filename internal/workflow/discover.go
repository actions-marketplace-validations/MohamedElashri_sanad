package workflow

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const nestedWorkflowPath = "**/.github/workflows"

func DiscoverWorkflowFiles(paths []string) ([]string, error) {
	seen := make(map[string]struct{})
	var files []string

	for _, path := range paths {
		path = filepath.Clean(path)
		matches, err := discoverWorkflowFiles(path)
		if err != nil {
			return nil, err
		}
		for _, match := range matches {
			if _, ok := seen[match]; ok {
				continue
			}
			seen[match] = struct{}{}
			files = append(files, match)
		}
	}

	sort.Strings(files)
	return files, nil
}

func discoverWorkflowFiles(path string) ([]string, error) {
	// Config paths use slash-separated repository-relative paths. Normalize
	// separators before cleaning so the conventional recursive path remains
	// recognizable on Windows, where filepath.Clean turns it into
	// "**\\.github\\workflows".
	if strings.ReplaceAll(path, `\`, "/") == nestedWorkflowPath {
		return discoverNestedWorkflowFiles(".")
	}
	var files []string

	err := filepath.WalkDir(path, func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			if current == path && isNotExist(err) {
				return nil
			}
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if isWorkflowYAML(current) {
				return fmt.Errorf("workflow path %q must not be a symlink", current)
			}
			return nil
		}
		if isWorkflowYAML(current) {
			files = append(files, normalizeWorkflowPath(current))
		}
		return nil
	})
	if err != nil && isNotExist(err) {
		return nil, nil
	}

	return files, err
}

// discoverNestedWorkflowFiles finds conventional GitHub workflow directories
// below a repository root. It deliberately matches only directories named
// .github/workflows instead of treating every YAML file in the repository as a
// workflow.
func discoverNestedWorkflowFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if !entry.IsDir() {
			return nil
		}
		name := entry.Name()
		if name == "node_modules" || name == "vendor" {
			return filepath.SkipDir
		}
		// Skip .git and all hidden directories except .github, which is where
		// workflow files live. This avoids walking into dev-environment caches
		// like .gomodcache, .gocache, .gopath, .cache, etc.
		// The root of the walk (current == root) is exempt so that we do not
		// skip the entire tree when WalkDir starts from ".".
		if current != root && strings.HasPrefix(name, ".") && name != ".github" {
			return filepath.SkipDir
		}
		if entry.Name() != "workflows" || filepath.Base(filepath.Dir(current)) != ".github" {
			return nil
		}

		matches, err := discoverWorkflowFiles(current)
		if err != nil {
			return err
		}
		files = append(files, matches...)
		return filepath.SkipDir
	})
	if err != nil && isNotExist(err) {
		return nil, nil
	}
	return files, err
}

func isWorkflowYAML(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".yml" || ext == ".yaml"
}

func normalizeWorkflowPath(path string) string {
	return strings.ReplaceAll(filepath.ToSlash(path), "\\", "/")
}

func isNotExist(err error) bool {
	return errors.Is(err, os.ErrNotExist)
}
