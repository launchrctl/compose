package compose

import (
	"os"
	"path/filepath"
)

const (
	// GitType is const for GIT source type download.
	GitType = "git"
	// SourceReferenceTag represents git tag source.
	SourceReferenceTag = "tag"
	// SourceReferenceBranch represents git branch source.
	SourceReferenceBranch = "ref"
	// HTTPType is const for http source type download.
	HTTPType = "http"
)

// Downloader interface
type Downloader interface {
	Download(pkg *Package, targetDir string, kw *keyringWrapper) error
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

func getDownloaderForPackage(downloadType string) Downloader {
	switch {
	case downloadType == GitType:
		return newGit()
	case downloadType == HTTPType:
		return newHTTP()
	default:
		return newGit()
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

		// Skip package download if it exists in packages dir.
		if _, err := os.Stat(packagePath); os.IsNotExist(err) {
			err = downloadPackage(pkg, targetDir, kw)
			if err != nil {
				return packages, err
			}
		}

		// If package has plasma-compose.yaml, proceed with it
		if _, err := os.Stat(filepath.Join(packagePath, composeFile)); !os.IsNotExist(err) {
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
	downloader := getDownloaderForPackage(pkg.GetType())
	packagePath := filepath.Join(targetDir, pkg.GetName())
	downloadPath := filepath.Join(packagePath, pkg.GetTarget())

	if _, err := os.Stat(downloadPath); !os.IsNotExist(err) {
		// Skip package download if folder exists in packages dir.
		return nil
	}

	// temporary
	if dtype := pkg.GetType(); dtype == HTTPType {
		downloadPath = packagePath
	}

	err := downloader.Download(pkg, downloadPath, kw)
	return err
}
