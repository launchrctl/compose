// Package compose with tools to download and compose packages
package compose

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/launchrctl/keyring"
)

const (
	composeFile = "compose.yaml"
	composeLock = "compose.lock"
	buildDir    = ".compose/build"
)

var (
	errComposeNotExists = errors.New("compose.yaml doesn't exist")
)

// Composer stores compose definition
type Composer struct {
	fs      fs.FS
	pwd     string
	options *ComposerOptions
	compose *YamlCompose
	k       keyring.Keyring
}

// ComposerOptions - list of possible composer options
type ComposerOptions struct {
	WorkingDir       string
	SkipNotVersioned bool
}

// CreateComposer instance
func CreateComposer(fs fs.FS, pwd string, opts ComposerOptions, k keyring.Keyring) (*Composer, error) {
	config, err := composeLookup(fs)
	if err != nil {
		return nil, errComposeNotExists
	}

	return &Composer{fs, pwd, &opts, config, k}, nil
}

// RunInstall on composr
func (c *Composer) RunInstall() error {
	lock, _ := lockLookup(c.getFS())
	dm := CreateDownloadManager()

	packagesDir := c.getPackagesDirPath()
	buildDir := c.getBuildDirPath()

	if lock != nil {
		_, err := dm.DownloadViaLock(lock, packagesDir, c.getKeyring())
		if err != nil {
			return err
		}
	} else {
		packages, err := dm.DownloadViaCompose(c.getCompose(), packagesDir, c.getKeyring())
		if err != nil {
			return err
		}
		lock = &YamlLock{Packages: packages}

		err = lock.save(filepath.Join(c.pwd, composeLock))
		if err != nil {
			return err
		}
	}

	// ensure all packages downloaded / warn user
	dm.ensurePackagesExist()

	builder := createBuilder(c.pwd, buildDir, packagesDir, c.options.SkipNotVersioned, lock.Packages)
	return builder.build()
}

func (c *Composer) getBuildDirPath() string {
	return filepath.Join(c.pwd, buildDir)
}

func (c *Composer) getPackagesDirPath() string {
	return filepath.Join(c.pwd, c.options.WorkingDir)
}

// EnsureDirExists checks if directory exists, otherwise create it
func EnsureDirExists(path string) error {
	return os.MkdirAll(path, os.ModePerm)
}

func composeLookup(fsys fs.FS) (*YamlCompose, error) {
	f, err := fs.ReadFile(fsys, composeFile)
	if err != nil {
		return &YamlCompose{}, err
	}

	cfg, err := parseComposeYaml(f)
	if err != nil {
		return &YamlCompose{}, err
	}

	return cfg, nil
}

func lockLookup(fsys fs.FS) (*YamlLock, error) {
	f, err := fs.ReadFile(fsys, composeLock)
	if err != nil {
		return nil, err
	}

	cfg, err := parseLockYaml(f)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Composer) getCompose() *YamlCompose {
	return c.compose
}

func (c *Composer) getFS() fs.FS {
	return c.fs
}

func (c *Composer) getKeyring() keyring.Keyring {
	return c.k
}
