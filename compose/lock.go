package compose

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	// LockFile is the name of the lock file written to the build directory.
	LockFile = "plasma-compose.lock"
)

var errLockNotExists = errors.New("plasma-compose.lock not found, run compose first")

// LockEntry represents a single resolved package in the lock file.
type LockEntry struct {
	Name       string   `yaml:"name"`
	Type       string   `yaml:"type"`
	URL        string   `yaml:"url"`
	Ref        string   `yaml:"ref"`
	Path       string   `yaml:"path"`
	RequiredBy []string `yaml:"required_by"`
}

// Lock stores the full resolved dependency list produced by compose.
type Lock struct {
	Packages []LockEntry `yaml:"packages"`
}

// LookupLock reads and parses the lock file from the given build directory fs.
// Returns errLockNotExists if the file is absent.
func LookupLock(fsys fs.FS) (*Lock, error) {
	data, err := fs.ReadFile(fsys, LockFile)
	if err != nil {
		return nil, errLockNotExists
	}

	var lock Lock
	if err = yaml.Unmarshal(data, &lock); err != nil {
		return nil, fmt.Errorf("plasma-compose.lock parsing failed: %w", err)
	}

	return &lock, nil
}

// writeLock serialises lock and writes it to targetDir/plasma-compose.lock.
func writeLock(lock *Lock, targetDir string) error {
	data, err := yaml.Marshal(lock)
	if err != nil {
		return fmt.Errorf("could not marshal lock file: %w", err)
	}

	// Ensure trailing newline so the file is consistent with text editors and
	// can be compared with txtar fixture files (which always end with \n).
	if len(data) > 0 && data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}

	path := filepath.Join(targetDir, LockFile)
	return os.WriteFile(path, data, 0600)
}

// buildLock constructs a Lock from the resolved package list.
// packages must already be in topological order (as returned by build).
// platformDir is the project root — paths in the lock are relative to it.
// packagesDir is the absolute path where packages are stored.
func buildLock(packages []*Package, platformDir, packagesDir string, requiredBy map[string][]string) (*Lock, error) {
	lock := &Lock{Packages: make([]LockEntry, 0, len(packages))}

	for _, pkg := range packages {
		ref := pkg.GetTarget()

		var pkgPath string
		if pkg.GetType() == HTTPType {
			// http packages have no ref subdirectory (current behaviour).
			pkgPath = filepath.Join(packagesDir, pkg.GetName())
		} else {
			pkgPath = filepath.Join(packagesDir, pkg.GetName(), ref)
		}

		relPath, err := filepath.Rel(platformDir, pkgPath)
		if err != nil {
			return nil, fmt.Errorf("computing relative path for %q: %w", pkg.GetName(), err)
		}

		entry := LockEntry{
			Name:       pkg.GetName(),
			Type:       pkg.GetType(),
			URL:        pkg.GetURL(),
			Ref:        pkg.GetRef(),
			Path:       filepath.ToSlash(relPath),
			RequiredBy: requiredBy[pkg.GetName()],
		}
		lock.Packages = append(lock.Packages, entry)
	}

	return lock, nil
}
