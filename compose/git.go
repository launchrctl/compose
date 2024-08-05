package compose

import (
	"errors"
	"fmt"
	"os"

	"github.com/launchrctl/launchr/pkg/cli"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/launchrctl/keyring"
	"github.com/launchrctl/launchr/pkg/log"
)

type gitDownloader struct{}

func newGit() Downloader {
	return &gitDownloader{}
}

// Download implements Downloader.Download interface
func (g *gitDownloader) Download(pkg *Package, targetDir string, kw *keyringWrapper) error {
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

	auths := []authorizationMode{authorisationNone, authorisationKeyring, authorisationManual}
	for _, authType := range auths {
		options.Auth = nil
		if authType == authorisationNone {
			log.Debug("Auth None")
			options.Auth = &http.BasicAuth{}
			_, err := git.PlainClone(targetDir, false, options)
			if err != nil {
				if errors.Is(err, transport.ErrAuthenticationRequired) {
					cli.Println("auth required, trying keyring authorisation")
					continue
				}

				return err
			}
		}

		if authType == authorisationKeyring {
			log.Debug("Auth Keyring")
			ci, err := kw.getForURL(url)
			if err != nil {
				return err
			}

			options.Auth = &http.BasicAuth{
				Username: ci.Username,
				Password: ci.Password,
			}

			_, err = git.PlainClone(targetDir, false, options)
			if err != nil {
				if errors.Is(err, transport.ErrAuthorizationFailed) || errors.Is(err, transport.ErrAuthenticationRequired) {
					if kw.interactive {
						cli.Println("invalid auth, trying manual authorisation")
						continue
					}
				}

				return err
			}
		}

		if authType == authorisationManual {
			log.Debug("Auth Manual")
			ci := keyring.CredentialsItem{}
			ci.URL = url
			ci, err := kw.fillCredentials(ci)
			if err != nil {
				return err
			}

			options.Auth = &http.BasicAuth{
				Username: ci.Username,
				Password: ci.Password,
			}

			_, err = git.PlainClone(targetDir, false, options)
			if err != nil {
				return err
			}
		}

		break
	}

	return nil
}

type authorizationMode int

const (
	authorisationNone authorizationMode = iota
	authorisationKeyring
	authorisationManual
)
