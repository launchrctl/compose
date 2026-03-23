package compose

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

type pathDownloader struct{}

func newPath() Downloader {
	return &pathDownloader{}
}

// EnsureLatest returns true if the destination already exists.
// Local paths have no remote version to check against.
func (p *pathDownloader) EnsureLatest(_ *Package, downloadPath string) (bool, error) {
	if _, err := os.Stat(downloadPath); !os.IsNotExist(err) {
		return true, nil
	}
	return false, nil
}

// Download copies the local source directory to targetDir.
func (p *pathDownloader) Download(_ context.Context, pkg *Package, targetDir string) error {
	srcPath := pkg.GetURL()
	if srcPath == "" {
		return errNoURL
	}

	if _, err := os.Stat(srcPath); os.IsNotExist(err) {
		return fmt.Errorf("local path %q does not exist", srcPath)
	}

	return copyDir(srcPath, targetDir)
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		destPath := filepath.Join(dst, rel)

		if d.IsDir() {
			return os.MkdirAll(destPath, dirPermissions)
		}

		return fcopy(path, destPath)
	})
}
