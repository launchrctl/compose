package compose

import (
	"fmt"
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
	Download(pkg *Package, targetDir string, ci keyring.CredentialsItem) error
}

// DownloadManager struct, provides methods to fetch packages
type DownloadManager struct {
	k keyring.Keyring
}

func (m DownloadManager) getKeyring() keyring.Keyring {
	return m.k
}

// CreateDownloadManager instance
func CreateDownloadManager(keyring keyring.Keyring) DownloadManager {
	return DownloadManager{k: keyring}
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
func (m DownloadManager) Download(c *YamlCompose, targetDir string) ([]*Package, error) {
	packages := []*Package{}
	err := EnsureDirExists(targetDir)
	if err != nil {
		return packages, err
	}

	packages, err = m.recursiveDownload(c, packages, nil, targetDir)
	if err != nil {
		return packages, err
	}

	return packages, m.getKeyring().Save()
}

func (m DownloadManager) recursiveDownload(c *YamlCompose, packages []*Package, parent *Package, targetDir string) ([]*Package, error) {
	for _, d := range c.Dependencies {
		// build package from dependency struct
		// add depedency if parent exists
		pkg := d.ToPackage(d.Name)
		if parent != nil {
			parent.AddDependency(d.Name)
		}

		url := pkg.GetURL()
		if url == "" {
			return packages, errNoURL
		}

		// @TODO check if package require auth for download
		ci, err := getPassword(m.getKeyring(), url)
		if err != nil {
			ci.URL = url
			ci, err = m.fillCreds(ci)
			if err != nil {
				return packages, err
			}
		}

		err = downloadPackage(pkg, targetDir, ci)
		if err != nil {
			return packages, err
		}

		var packagePath = filepath.Join(targetDir, d.Name)

		// If package has compose.yaml, proceed with it
		if _, err := os.Stat(filepath.Join(packagePath, composeFile)); !os.IsNotExist(err) {
			cfg, err := composeLookup(os.DirFS(packagePath))
			if err == nil {
				packages, err = m.recursiveDownload(cfg, packages, pkg, targetDir)
				if err != nil {
					return packages, err
				}
			}
		}

		packages = append(packages, pkg)
	}

	return packages, nil
}

func (m DownloadManager) fillCreds(ci keyring.CredentialsItem) (keyring.CredentialsItem, error) {
	k := m.getKeyring()

	if ci.URL != "" {
		fmt.Printf("Please add login and passwod for URL - %s\n", ci.URL)
	}
	err := keyring.RequestCredentialsFromTty(&ci)
	if err != nil {
		return ci, err
	}

	err = k.AddItem(ci)
	if err != nil {
		return ci, err
	}

	return ci, nil
}

func downloadPackage(pkg *Package, targetDir string, ci keyring.CredentialsItem) error {
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

	err := downloader.Download(pkg, downloadPath, ci)
	return err
}
