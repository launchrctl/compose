// Package launchr implements a launchrctl/launchr plugin to do platform composition
package launchr

import (
	"os"
	"path/filepath"

	"github.com/launchrctl/compose/compose"
	"github.com/launchrctl/launchr/core"
	"github.com/spf13/cobra"
)

var workingDir string

// ID is a plugin id.
const ID = "actions.compose"

func init() {
	core.RegisterPlugin(&Plugin{})
}

// Plugin is a plugin to discover actions defined in yaml.
type Plugin struct {
	app *core.App
}

// PluginInfo implements core.Plugin interface.
func (p *Plugin) PluginInfo() core.PluginInfo {
	return core.PluginInfo{
		ID: ID,
	}
}

// InitApp implements core.Plugin interface to provide discovered actions.
func (p *Plugin) InitApp(app *core.App) error {
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
