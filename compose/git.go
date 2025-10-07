package compose

import (
	"context"
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

	g.k.Term().Printfln("Fetching remote %s", url)
	for _, rem := range remotes {
		options := git.FetchOptions{
			//RefSpecs: []config.RefSpec{"refs/*:refs/*", "HEAD:refs/heads/HEAD"},
			RefSpecs: refSpec,
			Force:    true,
		}

		auths := []authenticationMode{authenticationModeNone, authenticationModeKeyringGlobal, authenticationModeKeyring, authenticationModeManual}
		for _, authMode := range auths {
			if authMode == authenticationModeNone {
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

			if authMode == authenticationModeKeyringGlobal {
				ci, err := g.k.getForBaseURL(url)
				if err != nil {
					if errors.Is(err, keyring.ErrNotFound) {
						continue
					}

					return err
				}

				options.Auth = &http.BasicAuth{
					Username: ci.Username,
					Password: ci.Password,
				}

				err = rem.Fetch(&options)
				if err != nil {
					if errors.Is(err, git.NoErrAlreadyUpToDate) {
						return nil
					}

					if !errors.Is(err, transport.ErrAuthorizationFailed) || !errors.Is(err, transport.ErrAuthenticationRequired) {
						return err
					}

					continue
				}
			}

			if authMode == authenticationModeKeyring {
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
							g.k.Term().Println("invalid auth, trying manual authentication")
							continue
						}
					}

					if !errors.Is(err, git.NoErrAlreadyUpToDate) {
						return err
					}

					return nil
				}
			}

			if authMode == authenticationModeManual {
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
		g.k.Log().Debug("git init error", "err", err)
		return false, nil
	}

	head, err := r.Head()
	if err != nil {
		g.k.Log().Debug("get head error", "err", err)
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
			g.k.Term().Warning().Printfln("Couldn't check local branch, marking package %s(%s) as outdated, see debug for detailed error.", pkg.GetName(), pkgRefName)
			g.k.Log().Debug("ensure branch error", "err", err)
			return isLatest, nil
		}
	} else {
		pullTarget = "tag"
		isLatest, err = g.ensureLatestTag(r, pkg.GetURL(), pkgRefName)
		if err != nil {
			g.k.Term().Warning().Printfln("Couldn't check local tag, marking package %s(%s) as outdated, see debug for detailed error.", pkg.GetName(), pkgRefName)
			g.k.Log().Debug("ensure tag error", "err", err)
			return isLatest, nil
		}
	}

	if !isLatest {
		g.k.Term().Info().Printfln("Pulling new changes from %s '%s' of %s package", pullTarget, pkgRefName, pkg.GetName())
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
func (g *gitDownloader) Download(ctx context.Context, pkg *Package, targetDir string) error {
	g.k.Term().Printfln("git fetch: %s", pkg.GetURL())

	url := pkg.GetURL()
	if url == "" {
		return errNoURL
	}

	ref := pkg.GetRef()
	if ref == "" {
		// Try to clone latest master branch.
		err := g.tryDownload(ctx, targetDir, g.buildOptions(url))
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

		err := g.tryDownload(ctx, targetDir, options)
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

func (g *gitDownloader) tryDownload(ctx context.Context, targetDir string, options *git.CloneOptions) error {
	url := options.URL
	auths := []authenticationMode{authenticationModeNone, authenticationModeKeyringGlobal, authenticationModeKeyring, authenticationModeManual}
	for _, authMode := range auths {
		if authMode == authenticationModeNone {
			_, err := git.PlainCloneContext(ctx, targetDir, false, options)
			g.k.Term().Println("")
			if err != nil {
				if errors.Is(err, transport.ErrAuthenticationRequired) {
					g.k.Term().Println("auth required, trying keyring authentication")
					continue
				}

				return err
			}
		}

		if authMode == authenticationModeKeyringGlobal {
			ci, err := g.k.getForBaseURL(url)
			if err != nil {
				if errors.Is(err, keyring.ErrNotFound) {
					continue
				}

				return err
			}

			options.Auth = &http.BasicAuth{
				Username: ci.Username,
				Password: ci.Password,
			}

			_, err = git.PlainCloneContext(ctx, targetDir, false, options)
			if err != nil {
				if !errors.Is(err, transport.ErrAuthorizationFailed) || !errors.Is(err, transport.ErrAuthenticationRequired) {
					return err
				}

				continue
			}
		}

		if authMode == authenticationModeKeyring {
			ci, err := g.k.getForURL(url)
			if err != nil {
				return err
			}

			options.Auth = &http.BasicAuth{
				Username: ci.Username,
				Password: ci.Password,
			}

			_, err = git.PlainCloneContext(ctx, targetDir, false, options)
			if err != nil {
				if errors.Is(err, transport.ErrAuthorizationFailed) || errors.Is(err, transport.ErrAuthenticationRequired) {
					if g.k.interactive {
						g.k.Term().Println("invalid auth, trying manual authentication")
						continue
					}
				}

				return err
			}
		}

		if authMode == authenticationModeManual {
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

			_, err = git.PlainCloneContext(ctx, targetDir, false, options)
			if err != nil {
				return err
			}
		}

		break
	}

	return nil
}

type authenticationMode int

const (
	authenticationModeNone authenticationMode = iota
	authenticationModeKeyringGlobal
	authenticationModeKeyring
	authenticationModeManual
)
