// Package compose implements a launchr plugin to do platform composition
package compose

import (
	"os"

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

// PluginInfo implements launchr.Plugin interface.
func (p *Plugin) PluginInfo() launchr.PluginInfo {
	return launchr.PluginInfo{
		ID: ID,
	}
}

// InitApp implements launchr.Plugin interface to provide discovered actions.
func (p *Plugin) InitApp(app *launchr.App) error {
	p.app = app
	return nil
}

// CobraAddCommands implements launchr.CobraPlugin interface to provide discovered actions.
func (p *Plugin) CobraAddCommands(rootCmd *cobra.Command) error {
	// CLI command to discover actions in file structure and provide
	var composeCmd = &cobra.Command{
		Use:   "compose",
		Short: "Composes platform",
		RunE: func(cmd *cobra.Command, args []string) error {
			dp := p.app.GetWD()

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
