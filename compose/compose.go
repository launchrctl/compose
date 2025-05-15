// Package compose with tools to download and compose packages
package compose

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/launchrctl/keyring"
	"github.com/launchrctl/launchr/pkg/action"
)

const (
	// MainDir is a compose directory.
	MainDir = ".compose"
	// BuildDir is a result directory of compose action.
	BuildDir       = MainDir + "/build"
	composeFile    = "plasma-compose.yaml"
	dirPermissions = 0755
)

var (
	errComposeNotExists    = errors.New("plasma-compose.yaml doesn't exist")
	errComposeBadStructure = errors.New("incorrect mapping for plasma-compose.yaml, ensure structure is correct")
)

type keyringWrapper struct {
	action.WithLogger
	action.WithTerm

	keyringService keyring.Keyring
	interactive    bool
	shouldUpdate   bool
}

func (kw *keyringWrapper) getForURL(url string) (keyring.CredentialsItem, error) {
	ci, errGet := kw.keyringService.GetForURL(url)
	if errGet != nil {
		if errors.Is(errGet, keyring.ErrEmptyPass) {
			return ci, errGet
		} else if !errors.Is(errGet, keyring.ErrNotFound) {
			kw.Log().Debug(errGet.Error())
			return ci, errors.New("the keyring is malformed or wrong passphrase provided")
		}

		if !kw.interactive {
			return ci, errGet
		}

		ci.URL = url
		newCI, err := kw.fillCredentials(ci)
		if err != nil {
			return ci, err
		}

		err = kw.keyringService.AddItem(newCI)
		if err != nil {
			return ci, err
		}

		ci = newCI
		kw.shouldUpdate = true
	}

	return ci, nil
}

func (kw *keyringWrapper) fillCredentials(ci keyring.CredentialsItem) (keyring.CredentialsItem, error) {
	if ci.URL != "" {
		kw.Term().Printfln("Please add login and password for URL - %s", ci.URL)
	}
	err := keyring.RequestCredentialsFromTty(&ci)
	if err != nil {
		return ci, err
	}

	return ci, nil
}

// Composer stores compose definition
type Composer struct {
	action.WithLogger
	action.WithTerm

	pwd     string
	options *ComposerOptions
	compose *YamlCompose
	k       keyring.Keyring
}

// ComposerOptions - list of possible composer options
type ComposerOptions struct {
	Clean              bool
	WorkingDir         string
	SkipNotVersioned   bool
	ConflictsVerbosity bool
	Interactive        bool
}

// CreateComposer instance
func CreateComposer(pwd string, opts ComposerOptions, k keyring.Keyring) (*Composer, error) {
	config, err := Lookup(os.DirFS(pwd))
	if err != nil {
		return nil, err
	}

	return &Composer{pwd: pwd, options: &opts, compose: config, k: k}, nil
}

// RunInstall on Composer
func (c *Composer) RunInstall() error {
	ctx, cancel := context.WithCancel(context.Background())

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-signalChan
		c.Term().Printfln("\nTermination signal received. Cleaning up...")
		// cleanup dir
		_, _, _ = c.prepareInstall(false)

		cancel()
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		buildDir, packagesDir, err := c.prepareInstall(c.options.Clean)
		if err != nil {
			return err
		}

		kw := &keyringWrapper{
			keyringService: c.getKeyring(),
			shouldUpdate:   false,
			interactive:    c.options.Interactive,
		}
		kw.SetLogger(c.Log())
		kw.SetTerm(c.Term())
		dm := CreateDownloadManager(kw)
		packages, err := dm.Download(ctx, c.getCompose(), packagesDir)
		if err != nil {
			return err
		}

		builder := createBuilder(
			c,
			buildDir,
			packagesDir,
			packages,
		)
		return builder.build(ctx)
	}
}

func (c *Composer) prepareInstall(clean bool) (string, string, error) {
	for _, dep := range c.compose.Dependencies {
		if dep.Source.Tag != "" {
			c.Term().Warning().Printfln("found deprecated field `tag` in `%s` dependency. Use `ref` field for tags or branches.", dep.Name)
		}
	}

	buildPath := c.getPath(BuildDir)
	packagesPath := c.getPath(c.options.WorkingDir)

	c.Term().Printfln("Cleaning build dir: %s", BuildDir)
	err := os.RemoveAll(buildPath)
	if err != nil {
		return "", "", err
	}

	if clean {
		c.Term().Printfln("Cleaning packages dir: %s", packagesPath)
		err = os.RemoveAll(packagesPath)
		if err != nil {
			return "", "", err
		}
	}

	return buildPath, packagesPath, nil
}

func (c *Composer) getPath(value string) string {
	return filepath.Join(c.pwd, value)
}

// EnsureDirExists checks if directory exists, otherwise create it
func EnsureDirExists(path string) error {
	return os.MkdirAll(path, dirPermissions)
}

func (c *Composer) getCompose() *YamlCompose {
	return c.compose
}

func (c *Composer) getKeyring() keyring.Keyring {
	return c.k
}
