package compose

import (
	"os"
	"path/filepath"

	"github.com/launchrctl/keyring"
)

const (
	gitType  = "git"
	httpType = "http"
)

// Downloader interface
type Downloader interface {
	Download(pkg *Package, targetDir string, k keyring.Keyring) error
}

// DownloadManager struct, provides methods to fetch packages
type DownloadManager struct {
}

// CreateDownloadManager instance
func CreateDownloadManager() DownloadManager {
	return DownloadManager{}
}

func getDownloaderForPackage(downloadType string) Downloader {
	switch {
	case downloadType == gitType:
		return newGit()
	case downloadType == httpType:
		return newHTTP()
	default:
		return newGit()
	}
}

func (m DownloadManager) ensurePackagesExist() {

}

func getPassword(k keyring.Keyring, url string) (keyring.CredentialsItem, error) {
	creds, err := k.GetForURL(url)
	if err != nil {
		return keyring.CredentialsItem{}, err
	}
	return creds, nil
}

// Download packages using compose file
func (m DownloadManager) Download(c *YamlCompose, targetDir string, k keyring.Keyring) ([]*Package, error) {
	packages := []*Package{}
	err := EnsureDirExists(targetDir)
	if err != nil {
		return packages, err
	}

	return m.recursiveDownload(c, packages, nil, targetDir, k)
}

func (m DownloadManager) recursiveDownload(c *YamlCompose, packages []*Package, parent *Package, targetDir string, k keyring.Keyring) ([]*Package, error) {
	for _, d := range c.Dependencies {
		// build package from dependency struct
		// add depedency if parent exists
		pkg := d.ToPackage(d.Name)
		if parent != nil {
			parent.AddDependency(d.Name)
		}

		err := downloadPackage(pkg, targetDir, k)
		if err != nil {
			return packages, err
		}

		var packagePath = filepath.Join(targetDir, d.Name)

		// If package has compose.yaml, proceed with it
		if _, err := os.Stat(filepath.Join(packagePath, composeFile)); !os.IsNotExist(err) {
			cfg, err := composeLookup(os.DirFS(packagePath))
			if err == nil {
				packages, err = m.recursiveDownload(cfg, packages, pkg, targetDir, k)
				if err != nil {
					return packages, err
				}
			}
		}

		packages = append(packages, pkg)
	}

	return packages, nil
}

func downloadPackage(pkg *Package, targetDir string, k keyring.Keyring) error {
	downloader := getDownloaderForPackage(pkg.GetType())
	packagePath := filepath.Join(targetDir, pkg.GetName())
	downloadPath := filepath.Join(packagePath, pkg.GetTarget())

	if _, err := os.Stat(downloadPath); !os.IsNotExist(err) {
		// Skip package download if folder exists in packages dir.
		return nil
	}

	// temporary
	if dtype := pkg.GetType(); dtype == httpType {
		downloadPath = packagePath
	}

	err := downloader.Download(pkg, downloadPath, k)
	return err
}
