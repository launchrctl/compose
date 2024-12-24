package compose

import (
	"io"
	"os"
	"path/filepath"

	"github.com/launchrctl/launchr"
)

const (
	// GitType is const for GIT source type download.
	GitType = "git"
	// HTTPType is const for http source type download.
	HTTPType = "http"
)

// Downloader interface
type Downloader interface {
	Download(pkg *Package, targetDir string) error
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

func getDownloaderForPackage(downloadType string, kw *keyringWrapper) Downloader {
	switch {
	case downloadType == GitType:
		return newGit(kw)
	case downloadType == HTTPType:
		return newHTTP(kw)
	default:
		return newGit(kw)
	}
}

// Download packages using compose file
func (m DownloadManager) Download(c *YamlCompose, targetDir string) ([]*Package, error) {
	var packages []*Package
	//credentials := []keyring.CredentialsItem{}
	err := EnsureDirExists(targetDir)
	if err != nil {
		return packages, err
	}

	kw := m.getKeyring()
	packages, err = m.recursiveDownload(c, kw, packages, nil, targetDir)
	if err != nil {
		return packages, err
	}

	// store keyring credentials
	if kw.shouldUpdate {
		err = kw.keyringService.Save()
	}

	return packages, err
}

func (m DownloadManager) recursiveDownload(yc *YamlCompose, kw *keyringWrapper, packages []*Package, parent *Package, targetDir string) ([]*Package, error) {
	for _, d := range yc.Dependencies {
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

		err := downloadPackage(pkg, targetDir, kw)
		if err != nil {
			return packages, err
		}

		// If package has plasma-compose.yaml, proceed with it
		if _, err = os.Stat(filepath.Join(packagePath, composeFile)); !os.IsNotExist(err) {
			cfg, err := Lookup(os.DirFS(packagePath))
			if err == nil {
				packages, err = m.recursiveDownload(cfg, kw, packages, pkg, targetDir)
				if err != nil {
					return packages, err
				}
			}
		}

		packages = append(packages, pkg)
	}

	return packages, nil
}

func downloadPackage(pkg *Package, targetDir string, kw *keyringWrapper) error {
	downloader := getDownloaderForPackage(pkg.GetType(), kw)
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

	err = downloader.Download(pkg, downloadPath)
	if err != nil {
		errRemove := os.RemoveAll(downloadPath)
		if errRemove != nil {
			launchr.Log().Debug("error cleaning package folder", "path", downloadPath, "err", err)
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

	return false, err
}
