package compose

import (
	"fmt"
	"os"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
)

const (
	fallbackPath = "https://github.com/plasma/"
)

type gitDownloader struct{}

func newGit() Downloader {
	return &gitDownloader{}
}

// Download implements Downloader.Download interface
func (g *gitDownloader) Download(pkg *Package, targetDir string) error {
	fmt.Println(fmt.Sprintf("git fetch: " + pkg.GetURL()))

	url := pkg.GetURL()
	if url == "" {
		url = fallbackPath + pkg.GetName()
	}

	options := &git.CloneOptions{
		URL:      url,
		Progress: os.Stdout,
	}
	if pkg.GetRef() != "" {
		options.ReferenceName = plumbing.NewTagReferenceName(pkg.GetRef())
	}

	auth := pkg.GetAuth()
	if auth != nil {
		options.Auth = &http.BasicAuth{
			Username: auth.Name,
			Password: auth.Password,
		}
	}

	_, err := git.PlainClone(targetDir, false, options)
	return err
}
