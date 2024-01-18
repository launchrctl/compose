// Package compose with tools to download and compose packages
package compose

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/launchrctl/keyring"
)

const (
	MainDir        = ".compose"         // MainDir is a compose directory.
	BuildDir       = MainDir + "/build" // BuildDir is a result directory of compose action.
	composeFile    = "compose.yaml"
	dirPermissions = 0755
)

var (
	errComposeNotExists    = errors.New("compose.yaml doesn't exist")
	errComposeBadStructure = errors.New("incorrect mapping for compose.yaml, ensure structure is correct")
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
}

// CreateComposer instance
func CreateComposer(pwd string, opts ComposerOptions, k keyring.Keyring) (*Composer, error) {
	config, err := composeLookup(os.DirFS(pwd))
	if err != nil {
		return nil, err
	}

	return &Composer{pwd, &opts, config, k}, nil
}

// RunInstall on composr
func (c *Composer) RunInstall() error {
	buildDir, packagesDir, err := c.prepareInstall()
	if err != nil {
		return err
	}

	dm := CreateDownloadManager(c.getKeyring())
	packages, err := dm.Download(c.getCompose(), packagesDir)
	if err != nil {
		return err
	}

	// ensure all packages downloaded / warn user
	dm.ensurePackagesExist()

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
	composePath := c.getPath(MainDir)
	packagesPath := c.getPath(c.options.WorkingDir)

	if c.options.Clean {
		fmt.Printf("Cleaning compose dir: %s\n", MainDir)
		err := os.RemoveAll(composePath)
		if err != nil {
			return "", "", err
		}

		fmt.Printf("Cleaning packages dir: %s\n", c.options.WorkingDir)
		err = os.RemoveAll(packagesPath)
		if err != nil {
			return "", "", err
		}
	} else {
		fmt.Printf("Cleaning build dir: %s\n", BuildDir)
		err := os.RemoveAll(buildPath)
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
