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
	composeFile    = "compose.yaml"
	buildDir       = ".compose/build"
	dirPermissions = 0755
)

var (
	errComposeNotExists = errors.New("compose.yaml doesn't exist")
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
	WorkingDir         string
	SkipNotVersioned   bool
	ConflictsVerbosity bool
}

// CreateComposer instance
func CreateComposer(pwd string, opts ComposerOptions, k keyring.Keyring) (*Composer, error) {
	config, err := composeLookup(os.DirFS(pwd))
	if err != nil {
		return nil, errComposeNotExists
	}

	return &Composer{pwd, &opts, config, k}, nil
}

// RunInstall on composr
func (c *Composer) RunInstall() error {
	dm := CreateDownloadManager()

	packagesDir := c.getPackagesDirPath()
	packages, err := dm.Download(c.getCompose(), packagesDir, c.getKeyring())
	if err != nil {
		return err
	}

	// ensure all packages downloaded / warn user
	dm.ensurePackagesExist()

	builder := createBuilder(
		c.pwd,
		c.getBuildDirPath(),
		packagesDir,
		c.options.SkipNotVersioned,
		c.options.ConflictsVerbosity,
		packages)
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
	return os.MkdirAll(path, dirPermissions)
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

func (c *Composer) getCompose() *YamlCompose {
	return c.compose
}

func (c *Composer) getKeyring() keyring.Keyring {
	return c.k
}
