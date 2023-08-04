package compose

import (
	"os"
	"path/filepath"

	"github.com/launchrctl/keyring"
	"golang.org/x/sync/errgroup"
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

// DownloadViaLock packages using compose lock file
func (m DownloadManager) DownloadViaLock(l *YamlLock, targetDir string, k keyring.Keyring) ([]*Package, error) {
	err := EnsureDirExists(targetDir)
	if err != nil {
		return l.Packages, err
	}

	g := new(errgroup.Group)
	for _, p := range l.Packages {
		p := p
		g.Go(func() error {
			return downloadPackage(p, targetDir, k)
		})
	}

	if err := g.Wait(); err != nil {
		return l.Packages, err
	}

	return l.Packages, nil
}

func downloadPackage(pkg *Package, targetDir string, k keyring.Keyring) error {
	downloader := getDownloaderForPackage(pkg.GetType())
	var packagePath = filepath.Join(targetDir, pkg.GetName())
	var downloadPath = packagePath
	// temporary
	if dtype := pkg.GetType(); dtype == httpType {
		downloadPath = targetDir
	}

	err := downloader.Download(pkg, downloadPath, k)
	return err
}

// DownloadViaCompose packages using compose file
func (m DownloadManager) DownloadViaCompose(c *YamlCompose, targetDir string, k keyring.Keyring) ([]*Package, error) {
	packages := []*Package{}
	err := EnsureDirExists(targetDir)
	if err != nil {
		return packages, err
	}

	return m.composeDownload(c, packages, nil, targetDir, k)
}

func (m DownloadManager) composeDownload(c *YamlCompose, packages []*Package, parent *Package, targetDir string, k keyring.Keyring) ([]*Package, error) {
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
				packages, err = m.composeDownload(cfg, packages, pkg, targetDir, k)
				if err != nil {
					return packages, err
				}
			}
		}

		packages = append(packages, pkg)
	}

	return packages, nil
}
