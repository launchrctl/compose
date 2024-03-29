package compose

import (
	"fmt"
	"os"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/launchrctl/keyring"
)

type gitDownloader struct{}

func newGit() Downloader {
	return &gitDownloader{}
}

// Download implements Downloader.Download interface
func (g *gitDownloader) Download(pkg *Package, targetDir string, ci keyring.CredentialsItem) error {
	fmt.Println(fmt.Sprintf("git fetch: " + pkg.GetURL()))

	url := pkg.GetURL()
	if url == "" {
		return errNoURL
	}

	options := &git.CloneOptions{
		URL:      url,
		Progress: os.Stdout,
		Depth:    1,
	}
	if pkg.GetRef() != "" {
		options.ReferenceName = plumbing.NewBranchReferenceName(pkg.GetRef())
	} else if pkg.GetTag() != "" {
		options.ReferenceName = plumbing.NewTagReferenceName(pkg.GetTag())
	}

	if ci != (keyring.CredentialsItem{}) {
		options.Auth = &http.BasicAuth{
			Username: ci.Username,
			Password: ci.Password,
		}
	}

	_, err := git.PlainClone(targetDir, false, options)
	return err
}
