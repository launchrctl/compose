// Package compose implements a launchr plugin to do platform composition
package compose

import (
	"os"
	"path/filepath"

	"github.com/launchrctl/launchr"
	"github.com/spf13/cobra"

	"github.com/launchrctl/compose/compose"
)

var workingDir string

// ID is a plugin id.
const ID = "compose"

func init() {
	launchr.RegisterPlugin(&Plugin{})
}

// Plugin is a plugin to discover actions defined in yaml.
type Plugin struct {
	app *launchr.App
}

// PluginInfo implements core.Plugin interface.
func (p *Plugin) PluginInfo() launchr.PluginInfo {
	return launchr.PluginInfo{
		ID: ID,
	}
}

// InitApp implements core.Plugin interface to provide discovered actions.
func (p *Plugin) InitApp(app *launchr.App) error {
	p.app = app
	return nil
}

// CobraAddCommands implements core.CobraPlugin interface to provide discovered actions.
func (p *Plugin) CobraAddCommands(rootCmd *cobra.Command) error {
	// CLI command to discover actions in file structure and provide
	var composeCmd = &cobra.Command{
		Use:   "compose",
		Short: "Composes platform",
		RunE: func(cmd *cobra.Command, args []string) error {
			dp, _ := GetDiscoveryPath()

			action, err := compose.CreateComposer(
				os.DirFS(dp),
				dp,
				compose.ComposerOptions{WorkingDir: workingDir},
			)
			if err != nil {
				return err
			}

			return action.RunInstall()
		},
	}

	composeCmd.Flags().StringVarP(&workingDir, "working-dir", "w", ".compose/packages", "Working directory for temp files")
	rootCmd.AddCommand(composeCmd)
	return nil
}

// GetDiscoveryPath provides actions absolute path.
func GetDiscoveryPath() (string, error) {
	sp := os.Getenv("LAUNCHR_DISCOVERY_PATH")
	if sp == "" {
		sp = "./"
	}
	return filepath.Abs(sp)
}
