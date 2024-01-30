// Package compose implements a launchr plugin to do platform composition
package compose

import (
	"os"
	"path/filepath"

	"github.com/launchrctl/keyring"
	"github.com/launchrctl/launchr"
	"github.com/launchrctl/launchr/pkg/action"
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
	buildDir := filepath.Join(p.wd, compose.BuildDir)
	app.RegisterFS(action.NewDiscoveryFS(os.DirFS(buildDir), p.wd))
	return nil
}

// CobraAddCommands implements launchr.CobraPlugin interface to provide compose functionality.
func (p *Plugin) CobraAddCommands(rootCmd *cobra.Command) error {
	var workingDir string
	var skipNotVersioned bool
	var conflictsVerbosity bool
	var clean bool
	var composeCmd = &cobra.Command{
		Use:   "compose",
		Short: "Composes filesystem (files & dirs)",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Don't show usage help on a runtime error.
			cmd.SilenceUsage = true
			c, err := compose.CreateComposer(
				p.wd,
				compose.ComposerOptions{
					Clean:              clean,
					WorkingDir:         workingDir,
					SkipNotVersioned:   skipNotVersioned,
					ConflictsVerbosity: conflictsVerbosity,
				},
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
	composeCmd.Flags().BoolVar(&conflictsVerbosity, "conflicts-verbosity", false, "Log files conflicts")
	composeCmd.Flags().BoolVar(&clean, "clean", false, "Remove .compose dir on start")
	rootCmd.AddCommand(composeCmd)
	return nil
}
