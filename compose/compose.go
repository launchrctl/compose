// Package compose with tools to download and compose packages
package compose

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/launchrctl/launchr/pkg/log"

	"github.com/launchrctl/keyring"
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
	config, err := composeLookup(os.DirFS(pwd))
	if err != nil {
		return nil, err
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
			log.Debug("%s", errGet)
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
		fmt.Printf("Please add login and password for URL - %s\n", ci.URL)
	}
	err := keyring.RequestCredentialsFromTty(&ci)
	if err != nil {
		return ci, err
	}

	return ci, nil
}

// RunInstall on Composer
func (c *Composer) RunInstall() error {
	buildDir, packagesDir, err := c.prepareInstall()
	if err != nil {
		return err
	}

	kw := &keyringWrapper{keyringService: c.getKeyring(), shouldUpdate: false, interactive: c.options.Interactive}
	dm := CreateDownloadManager(kw)
	packages, err := dm.Download(c.getCompose(), packagesDir)
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
	return builder.build()
}

func (c *Composer) prepareInstall() (string, string, error) {
	buildPath := c.getPath(BuildDir)
	packagesPath := c.getPath(c.options.WorkingDir)

	fmt.Printf("Cleaning build dir: %s\n", BuildDir)
	err := os.RemoveAll(buildPath)
	if err != nil {
		return "", "", err
	}

	if c.options.Clean {
		fmt.Printf("Cleaning packages dir: %s\n", packagesPath)
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

func composeLookup(fsys fs.FS) (*YamlCompose, error) {
	f, err := fs.ReadFile(fsys, composeFile)
	if err != nil {
		return &YamlCompose{}, errComposeNotExists
	}

	cfg, err := parseComposeYaml(f)
	if err != nil {
		return &YamlCompose{}, errComposeBadStructure
	}

	return cfg, nil
}

func (c *Composer) getCompose() *YamlCompose {
	return c.compose
}

func (c *Composer) getKeyring() keyring.Keyring {
	return c.k
}
