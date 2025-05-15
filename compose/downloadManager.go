package compose

import (
	"context"
	"io"
	"os"
	"path/filepath"
)

const (
	// GitType is const for GIT source type download.
	GitType = "git"
	// HTTPType is const for http source type download.
	HTTPType = "http"
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
	switch {
	case downloadType == HTTPType:
		return newHTTP(m.kw)
	case downloadType == GitType:
		fallthrough
	default:
		return newGit(m.kw)
	}
}

// Download packages using compose file
func (m DownloadManager) Download(ctx context.Context, c *YamlCompose, targetDir string) ([]*Package, error) {
	var packages []*Package
	//credentials := []keyring.CredentialsItem{}
	err := EnsureDirExists(targetDir)
	if err != nil {
		return packages, err
	}

	kw := m.getKeyring()
	packages, err = m.recursiveDownload(ctx, c, packages, nil, targetDir)
	if err != nil {
		return packages, err
	}

	// store keyring credentials
	if kw.shouldUpdate {
		err = kw.keyringService.Save()
	}

	return packages, err
}

func (m DownloadManager) recursiveDownload(ctx context.Context, yc *YamlCompose, packages []*Package, parent *Package, targetDir string) ([]*Package, error) {
	for _, d := range yc.Dependencies {
		select {
		case <-ctx.Done():
			return packages, ctx.Err()
		default:
			// build package from dependency struct
			// add dependency if parent exists
			pkg := d.ToPackage(d.Name)
			if parent != nil {
				parent.AddDependency(d.Name)
			}

			url := pkg.GetURL()
			if url == "" {
				return packages, errNoURL
			}

			packagePath := filepath.Join(targetDir, pkg.GetName(), pkg.GetTarget())

			err := m.downloadPackage(ctx, pkg, targetDir)
			if err != nil {
				return packages, err
			}

			// If package has plasma-compose.yaml, proceed with it
			if _, err = os.Stat(filepath.Join(packagePath, composeFile)); !os.IsNotExist(err) {
				cfg, err := Lookup(os.DirFS(packagePath))
				if err == nil {
					packages, err = m.recursiveDownload(ctx, cfg, packages, pkg, targetDir)
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
