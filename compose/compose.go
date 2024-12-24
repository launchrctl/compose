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
	"github.com/launchrctl/launchr"
)

const (
	MainDir        = ".compose"         // MainDir is a compose directory.
	BuildDir       = MainDir + "/build" // BuildDir is a result directory of compose action.
	composeFile    = "plasma-compose.yaml"
	dirPermissions = 0755
)

var (
	errComposeNotExists    = errors.New("plasma-compose.yaml doesn't exist")
	errComposeBadStructure = errors.New("incorrect mapping for plasma-compose.yaml, ensure structure is correct")
)

// Composer stores compose definition
type Composer struct {
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

	for _, dep := range config.Dependencies {
		if dep.Source.Tag != "" {
			launchr.Term().Warning().Printfln("found deprecated field `tag` in `%s` dependency. Use `ref` field for tags or branches.", dep.Name)
		}
	}

	return &Composer{pwd, &opts, config, k}, nil
}

type keyringWrapper struct {
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
			launchr.Log().Debug(errGet.Error())
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
		launchr.Term().Printfln("Please add login and password for URL - %s", ci.URL)
	}
	err := keyring.RequestCredentialsFromTty(&ci)
	if err != nil {
		return ci, err
	}

	return ci, nil
}

// RunInstall on Composer
func (c *Composer) RunInstall() error {
	ctx, cancel := context.WithCancel(context.Background())

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-signalChan
		launchr.Term().Printfln("\nTermination signal received. Cleaning up...")
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

		kw := &keyringWrapper{keyringService: c.getKeyring(), shouldUpdate: false, interactive: c.options.Interactive}
		dm := CreateDownloadManager(kw)
		packages, err := dm.Download(ctx, c.getCompose(), packagesDir)
		if err != nil {
			return err
		}

		builder := createBuilder(
			c.pwd,
			buildDir,
			packagesDir,
			c.options.SkipNotVersioned,
			c.options.ConflictsVerbosity,
			packages,
		)
		return builder.build(ctx)
	}
}

func (c *Composer) prepareInstall(clean bool) (string, string, error) {
	buildPath := c.getPath(BuildDir)
	packagesPath := c.getPath(c.options.WorkingDir)

	launchr.Term().Printfln("Cleaning build dir: %s", BuildDir)
	err := os.RemoveAll(buildPath)
	if err != nil {
		return "", "", err
	}

	if clean {
		launchr.Term().Printfln("Cleaning packages dir: %s", packagesPath)
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
