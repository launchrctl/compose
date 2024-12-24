package compose

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/launchrctl/keyring"
	"github.com/launchrctl/launchr"
)

type gitDownloader struct {
	k *keyringWrapper
}

func newGit(kw *keyringWrapper) Downloader {
	return &gitDownloader{k: kw}
}

func (g *gitDownloader) fetchRemotes(r *git.Repository, url string, refSpec []config.RefSpec) error {
	remotes, errR := r.Remotes()
	if errR != nil {
		return errR
	}

	launchr.Term().Printfln("Fetching remote %s", url)
	for _, rem := range remotes {
		options := git.FetchOptions{
			//RefSpecs: []config.RefSpec{"refs/*:refs/*", "HEAD:refs/heads/HEAD"},
			RefSpecs: refSpec,
			Force:    true,
		}

		auths := []authorizationMode{authorisationNone, authorisationKeyring, authorisationManual}
		for _, authType := range auths {
			if authType == authorisationNone {
				err := rem.Fetch(&options)
				if err != nil {
					if errors.Is(err, transport.ErrAuthenticationRequired) {
						continue
					}

					if !errors.Is(err, git.NoErrAlreadyUpToDate) {
						return err
					}

					return nil
				}
			}

			if authType == authorisationKeyring {
				ci, err := g.k.getForURL(url)
				if err != nil {
					return err
				}

				options.Auth = &http.BasicAuth{
					Username: ci.Username,
					Password: ci.Password,
				}

				err = rem.Fetch(&options)
				if err != nil {
					if errors.Is(err, transport.ErrAuthorizationFailed) || errors.Is(err, transport.ErrAuthenticationRequired) {
						if g.k.interactive {
							launchr.Term().Println("invalid auth, trying manual authorisation")
							continue
						}
					}

					if !errors.Is(err, git.NoErrAlreadyUpToDate) {
						return err
					}

					return nil
				}
			}

			if authType == authorisationManual {
				ci := keyring.CredentialsItem{}
				ci.URL = url
				ci, err := g.k.fillCredentials(ci)
				if err != nil {
					return err
				}

				options.Auth = &http.BasicAuth{
					Username: ci.Username,
					Password: ci.Password,
				}

				err = rem.Fetch(&options)
				if err != nil {
					if !errors.Is(err, git.NoErrAlreadyUpToDate) {
						return err
					}

					return nil
				}
			}

			break
		}
	}

	return nil
}

func (g *gitDownloader) EnsureLatest(pkg *Package, downloadPath string) (bool, error) {
	if _, err := os.Stat(downloadPath); os.IsNotExist(err) {
		// Return False in case package doesn't exist.
		return false, nil
	}

	emptyDir, err := IsEmptyDir(downloadPath)
	if err != nil {
		return false, err
	}

	if emptyDir {
		return false, nil
	}

	r, err := git.PlainOpen(downloadPath)
	if err != nil {
		launchr.Log().Debug("git init error", "err", err)
		return false, nil
	}

	head, err := r.Head()
	if err != nil {
		launchr.Log().Debug("get head error", "err", err)
		return false, fmt.Errorf("can't get HEAD of '%s', ensure package is valid", pkg.GetName())
	}

	headName := head.Name().Short()
	pkgRefName := pkg.GetRef()
	remoteRefName := pkgRefName

	if pkg.GetTarget() == TargetLatest && headName != "" {
		pkgRefName = headName
		remoteRefName = plumbing.HEAD.String()
	}

	pullTarget := ""
	isLatest := false
	if headName == pkgRefName {
		pullTarget = "branch"
		isLatest, err = g.ensureLatestBranch(r, pkg.GetURL(), pkgRefName, remoteRefName)
		if err != nil {
			launchr.Term().Warning().Printfln("Couldn't check local branch, marking package %s(%s) as outdated, see debug for detailed error.", pkg.GetName(), pkgRefName)
			launchr.Log().Debug("ensure branch error", "err", err)
			return isLatest, nil
		}
	} else {
		pullTarget = "tag"
		isLatest, err = g.ensureLatestTag(r, pkg.GetURL(), pkgRefName)
		if err != nil {
			launchr.Term().Warning().Printfln("Couldn't check local tag, marking package %s(%s) as outdated, see debug for detailed error.", pkg.GetName(), pkgRefName)
			launchr.Log().Debug("ensure tag error", "err", err)
			return isLatest, nil
		}
	}

	if !isLatest {
		launchr.Term().Info().Printfln("Pulling new changes from %s '%s' of %s package", pullTarget, pkgRefName, pkg.GetName())
	}

	return isLatest, nil
}

func (g *gitDownloader) ensureLatestBranch(r *git.Repository, fetchURL, refName, remoteRefName string) (bool, error) {
	refSpec := []config.RefSpec{config.RefSpec(fmt.Sprintf("refs/heads/%s:refs/heads/%s", refName, refName))}
	err := g.fetchRemotes(r, fetchURL, refSpec)
	if err != nil {
		return false, err
	}

	br, err := r.Branch(refName)
	if err != nil {
		return false, err
	}

	localRef, err := r.Reference(plumbing.ReferenceName(br.Merge.String()), true)
	if err != nil {
		return false, err
	}

	remote := filepath.Join("refs", "remotes", br.Remote, remoteRefName)
	remoteRef, err := r.Reference(plumbing.ReferenceName(remote), false)
	if err != nil {
		return false, err
	}

	return localRef.Hash() == remoteRef.Hash(), nil
}

func (g *gitDownloader) ensureLatestTag(r *git.Repository, fetchURL, refName string) (bool, error) {
	oldTag, err := r.Tag(refName)
	if err != nil {
		return false, err
	}

	head, err := r.Head()
	if err != nil {
		return false, err
	}

	refSpec := []config.RefSpec{config.RefSpec(fmt.Sprintf("refs/tags/%s:refs/tags/%s", refName, refName))}
	err = g.fetchRemotes(r, fetchURL, refSpec)
	if err != nil {
		return false, err
	}

	newTag, err := r.Tag(refName)
	if err != nil {
		return false, err
	}

	if oldTag.Hash().String() != newTag.Hash().String() {
		return false, err
	}
	revision := plumbing.Revision(newTag.Name().String())
	tagCommitHash, err := r.ResolveRevision(revision)
	if err != nil {
		return false, err
	}

	commit, err := r.CommitObject(*tagCommitHash)
	if err != nil {
		return false, err
	}

	return commit.ID() == head.Hash(), nil
}

// Download implements Downloader.Download interface
func (g *gitDownloader) Download(pkg *Package, targetDir string) error {
	launchr.Term().Printfln("git fetch: %s", pkg.GetURL())

	url := pkg.GetURL()
	if url == "" {
		return errNoURL
	}

	ref := pkg.GetRef()
	if ref == "" {
		// Try to clone latest master branch.
		err := g.tryDownload(targetDir, g.buildOptions(url))
		if err != nil {
			return err
		}

		return nil
	}

	loaded := false

	// As we don't know if ref exists, iterate and try to clone both: tag and branch references.
	refs := []plumbing.ReferenceName{plumbing.NewTagReferenceName(ref), plumbing.NewBranchReferenceName(ref)}
	for _, r := range refs {
		options := g.buildOptions(url)
		options.ReferenceName = r

		err := g.tryDownload(targetDir, options)
		if err != nil {
			noMatchError := git.NoMatchingRefSpecError{}
			if errors.Is(err, noMatchError) {
				continue
			}

			return err
		}

		loaded = true
		break
	}

	if !loaded {
		return fmt.Errorf("couldn't find remote ref %s", ref)
	}

	return nil
}

func (g *gitDownloader) buildOptions(url string) *git.CloneOptions {
	return &git.CloneOptions{
		URL:          url,
		Progress:     os.Stdout,
		SingleBranch: true,
	}
}

func (g *gitDownloader) tryDownload(targetDir string, options *git.CloneOptions) error {
	url := options.URL
	auths := []authorizationMode{authorisationNone, authorisationKeyring, authorisationManual}
	for _, authType := range auths {
		if authType == authorisationNone {
			_, err := git.PlainClone(targetDir, false, options)
			if err != nil {
				if errors.Is(err, transport.ErrAuthenticationRequired) {
					launchr.Term().Println("auth required, trying keyring authorisation")
					continue
				}

				return err
			}
		}

		if authType == authorisationKeyring {
			ci, err := g.k.getForURL(url)
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
					if g.k.interactive {
						launchr.Term().Println("invalid auth, trying manual authorisation")
						continue
					}
				}

				return err
			}
		}

		if authType == authorisationManual {
			ci := keyring.CredentialsItem{}
			ci.URL = url
			ci, err := g.k.fillCredentials(ci)
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
