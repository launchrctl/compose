package compose

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	// GitType is const for GIT source type download.
	GitType = "git"
	// HTTPType is const for http source type download.
	HTTPType = "http"
	// PathType is const for local filesystem source type.
	PathType = "path"
)

// Downloader interface
type Downloader interface {
	Download(ctx context.Context, pkg *Package, targetDir string) error
	EnsureLatest(pkg *Package, downloadPath string) (bool, error)
}

// DownloadManager struct, provides methods to fetch packages
type DownloadManager struct {
	kw *keyringWrapper
}

func (m DownloadManager) getKeyring() *keyringWrapper {
	return m.kw
}

// CreateDownloadManager instance
func CreateDownloadManager(keyring *keyringWrapper) DownloadManager {
	return DownloadManager{kw: keyring}
}

func (m DownloadManager) getDownloaderForPackage(downloadType string) Downloader {
	switch downloadType {
	case HTTPType:
		return newHTTP(m.kw)
	case PathType:
		return newPath()
	case GitType:
		fallthrough
	default:
		return newGit(m.kw)
	}
}

// resolvedPkg tracks a resolved package for conflict and cycle detection.
type resolvedPkg struct {
	pkgType    string
	url        string
	ref        string
	requiredBy []string
}

// depStack tracks the current recursion path for cycle detection.
// It maintains insertion order so error messages show the exact dependency chain.
type depStack struct {
	items []string
	set   map[string]bool
}

func newDepStack() *depStack {
	return &depStack{set: make(map[string]bool)}
}

func (s *depStack) push(name string) {
	s.items = append(s.items, name)
	s.set[name] = true
}

func (s *depStack) pop(name string) {
	s.items = s.items[:len(s.items)-1]
	delete(s.set, name)
}

func (s *depStack) has(name string) bool {
	return s.set[name]
}

func (s *depStack) path() string {
	return strings.Join(s.items, " → ")
}

// Download packages using compose file.
// Returns packages in topological order and a map of package name → required_by list.
func (m DownloadManager) Download(ctx context.Context, c *YamlCompose, targetDir string) ([]*Package, map[string][]string, error) {
	var packages []*Package
	err := EnsureDirExists(targetDir)
	if err != nil {
		return packages, nil, err
	}

	kw := m.getKeyring()
	seen := make(map[string]resolvedPkg)
	packages, err = m.recursiveDownload(ctx, c, packages, nil, targetDir, seen, newDepStack())
	if err != nil {
		return packages, nil, err
	}

	// store keyring credentials
	if kw.shouldUpdate {
		err = kw.keyringService.Save()
	}

	requiredBy := make(map[string][]string, len(seen))
	for name, rp := range seen {
		requiredBy[name] = rp.requiredBy
	}

	return packages, requiredBy, err
}

func (m DownloadManager) recursiveDownload(ctx context.Context, yc *YamlCompose, packages []*Package, parent *Package, targetDir string, seen map[string]resolvedPkg, stack *depStack) ([]*Package, error) {
	for _, d := range yc.Dependencies {
		select {
		case <-ctx.Done():
			return packages, ctx.Err()
		default:
			pkg := d.ToPackage(d.Name)
			name := pkg.GetName()
			ref := pkg.GetTarget()

			if parent != nil {
				parent.AddDependency(name)
			}

			if pkg.GetURL() == "" {
				return packages, errNoURL
			}

			// Cycle detection: package is already being processed up the call stack.
			// Must run before the seen dedup check, otherwise cycles with the same ref
			// would be silently skipped instead of raising an error.
			if stack.has(name) {
				return packages, fmt.Errorf("circular dependency detected: %s → %s", stack.path(), name)
			}

			// Conflict detection: same package name required with different type, url, or ref.
			if existing, ok := seen[name]; ok {
				if existing.pkgType != pkg.GetType() || existing.url != pkg.GetURL() || existing.ref != ref {
					return packages, fmt.Errorf(
						"version conflict: package %q required as %s %s@%s by %q and as %s %s@%s by %q",
						name,
						existing.pkgType, existing.url, existing.ref, existing.requiredBy,
						pkg.GetType(), pkg.GetURL(), ref, requiredBy(parent),
					)
				}
				// Same source already resolved — accumulate the additional parent and skip.
				existing.requiredBy = append(existing.requiredBy, requiredBy(parent))
				seen[name] = existing
				continue
			}

			seen[name] = resolvedPkg{pkgType: pkg.GetType(), url: pkg.GetURL(), ref: ref, requiredBy: []string{requiredBy(parent)}}

			packagePath := filepath.Join(targetDir, name, ref)

			err := m.downloadPackage(ctx, pkg, targetDir)
			if err != nil {
				return packages, err
			}

			// If package has plasma-compose.yaml, recurse into it.
			if _, statErr := os.Stat(filepath.Join(packagePath, composeFile)); !os.IsNotExist(statErr) {
				cfg, err := Lookup(os.DirFS(packagePath))
				if err != nil && !errors.Is(err, errComposeNotExists) {
					return packages, fmt.Errorf("package %q: %w", name, err)
				}
				if err == nil {
					stack.push(name)
					packages, err = m.recursiveDownload(ctx, cfg, packages, pkg, targetDir, seen, stack)
					stack.pop(name)
					if err != nil {
						return packages, err
					}
				}
			}

			packages = append(packages, pkg)
		}
	}

	return packages, nil
}

// requiredBy returns the name of the parent package, or "root" if declared in the root compose.
func requiredBy(parent *Package) string {
	if parent == nil {
		return "root"
	}
	return parent.GetName()
}

func (m DownloadManager) downloadPackage(ctx context.Context, pkg *Package, targetDir string) error {
	downloader := m.getDownloaderForPackage(pkg.GetType())
	packagePath := filepath.Join(targetDir, pkg.GetName())
	downloadPath := filepath.Join(packagePath, pkg.GetTarget())

	isLatest, err := downloader.EnsureLatest(pkg, downloadPath)
	if err != nil {
		return err
	}

	if isLatest {
		return nil
	}

	// Ensure old package doesn't exist in case of update.
	err = os.RemoveAll(downloadPath)
	if err != nil {
		return err
	}

	// temporary
	if dtype := pkg.GetType(); dtype == HTTPType {
		downloadPath = packagePath
	}

	err = downloader.Download(ctx, pkg, downloadPath)
	if err != nil {
		errRemove := os.RemoveAll(downloadPath)
		if errRemove != nil {
			m.kw.Log().Debug("error cleaning package folder", "path", downloadPath, "err", err)
		}
	}

	return err
}

// IsEmptyDir check if directory has at least 1 file.
func IsEmptyDir(name string) (bool, error) {
	f, err := os.Open(filepath.Clean(name))
	if err != nil {
		return false, err
	}
	defer f.Close()

	_, err = f.Readdirnames(1)
	if err == io.EOF {
		return true, nil
	}

	// Check if .git exists and nothing else
	gitPath := filepath.Join(name, ".git")
	if _, err = os.Stat(gitPath); err == nil {
		// .git exists, now check if it's the only entry
		entries, err := f.Readdirnames(2) // Read at most 2 entries
		if err != nil {
			return false, err
		}
		if len(entries) == 1 && entries[0] == ".git" {
			return true, nil
		}
	}

	// Directory is not empty
	return false, err
}
