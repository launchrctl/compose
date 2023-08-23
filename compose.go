// Package compose implements a launchr plugin to do platform composition
package compose

import (
	"os"

	"github.com/launchrctl/keyring"
	"github.com/launchrctl/launchr"
	"github.com/spf13/cobra"

	"github.com/launchrctl/compose/compose"
)

func init() {
	launchr.RegisterPlugin(&Plugin{})
}

// Plugin implements launchr.Plugin to provide compose functionality.
type Plugin struct {
	wd string
	k  keyring.Keyring
}

// PluginInfo implements launchr.Plugin interface.
func (p *Plugin) PluginInfo() launchr.PluginInfo {
	return launchr.PluginInfo{Weight: 10}
}

// OnAppInit implements launchr.Plugin interface.
func (p *Plugin) OnAppInit(app launchr.App) error {
	app.GetService(&p.k)
	p.wd = app.GetWD()
	return nil
}

// CobraAddCommands implements launchr.CobraPlugin interface to provide compose functionality.
func (p *Plugin) CobraAddCommands(rootCmd *cobra.Command) error {
	var workingDir string
	var skipNotVersioned bool
	var composeCmd = &cobra.Command{
		Use:   "compose",
		Short: "Composes filesystem (files & dirs)",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Don't show usage help on a runtime error.
			cmd.SilenceUsage = true
			c, err := compose.CreateComposer(
				os.DirFS(p.wd),
				p.wd,
				compose.ComposerOptions{WorkingDir: workingDir, SkipNotVersioned: skipNotVersioned},
				p.k,
			)
			if err != nil {
				return err
			}

			return c.RunInstall()
		},
	}

	composeCmd.Flags().StringVarP(&workingDir, "working-dir", "w", ".compose/packages", "Working directory for temp files")
	composeCmd.Flags().BoolVarP(&skipNotVersioned, "skip-not-versioned", "s", false, "Skip not versioned files from source directory (git only)")
	rootCmd.AddCommand(composeCmd)
	return nil
}
