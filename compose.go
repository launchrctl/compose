// Package compose implements a launchr plugin to do platform composition
package compose

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/launchrctl/launchr/pkg/cli"

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
	var interactive bool

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
					Interactive:        interactive,
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
	composeCmd.Flags().BoolVar(&interactive, "interactive", true, "Interactive mode allows to submit user credentials during action")

	composeDependency := &compose.Dependency{}
	strategies := &compose.RawStrategies{}
	var createNew bool
	var addCmd = &cobra.Command{
		Use:   "compose:add",
		Short: "Add a new package to plasma-compose",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return packagePreRunValidate(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			// Don't show usage help on a runtime error.
			cmd.SilenceUsage = true

			return compose.AddPackage(createNew, composeDependency, strategies, p.wd)
		},
	}

	var updateCmd = &cobra.Command{
		Use:   "compose:update",
		Short: "Update a plasma-compose package",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return packagePreRunValidate(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			// Don't show usage help on a runtime error.
			cmd.SilenceUsage = true

			if composeDependency.Name != "" {
				return compose.UpdatePackage(composeDependency, strategies, p.wd)
			}

			return compose.UpdatePackages(p.wd)
		},
	}

	var toDeletePackages []string
	var deleteCmd = &cobra.Command{
		Use:   "compose:delete",
		Short: "Remove a package from plasma-compose",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Don't show usage help on a runtime error.
			cmd.SilenceUsage = true

			return compose.DeletePackages(toDeletePackages, p.wd)
		},
	}

	addCmd.Flags().BoolVarP(&createNew, "allow-create", "", false, "Create plasma-compose if not exist")
	addPackageFlags(addCmd, composeDependency, strategies)
	addPackageFlags(updateCmd, composeDependency, strategies)

	deleteCmd.Flags().StringSliceVarP(&toDeletePackages, "packages", "", []string{}, "List of packages to remove. Comma separated.")

	rootCmd.AddCommand(composeCmd)
	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(deleteCmd)

	return nil
}

func addPackageFlags(cmd *cobra.Command, dependency *compose.Dependency, strategies *compose.RawStrategies) {
	cmd.Flags().StringVarP(&dependency.Name, "package", "", "", "Name of the package")
	compose.EnumVarP(cmd, &dependency.Source.Type, "type", "", compose.GitType, []string{compose.GitType, compose.HTTPType}, "Type of the package source: git, http")
	cmd.Flags().StringVarP(&dependency.Source.Ref, "ref", "", "", "Reference of the package source")
	cmd.Flags().StringVarP(&dependency.Source.Tag, "tag", "", "", "Tag of the package source")
	cmd.Flags().StringVarP(&dependency.Source.URL, "url", "", "", "URL of the package source")

	cmd.Flags().StringSliceVarP(&strategies.Names, "strategy", "", []string{}, "Strategy name")
	cmd.Flags().StringSliceVarP(&strategies.Paths, "strategy-path", "", []string{}, "Strategy paths. paths separated by |, strategies are comma separated (path/1|path/2,path/1|path/2)")
}

func packagePreRunValidate(cmd *cobra.Command, _ []string) error {
	tagChanged := cmd.Flag("tag").Changed
	refChanged := cmd.Flag("ref").Changed
	if tagChanged && refChanged {
		return errors.New("tag and ref cannot be used at the same time")
	}

	typeFlag, err := cmd.Flags().GetString("type")
	if err != nil {
		return err
	}

	if typeFlag == compose.HTTPType {
		if tagChanged {
			cli.Println("Tag can't be used with HTTP source")
			err = cmd.Flags().Set("tag", "")
		}
		if refChanged {
			cli.Println("Ref can't be used with HTTP source")
			err = cmd.Flags().Set("ref", "")
		}
	}

	strategyChanged := cmd.Flag("strategy").Changed
	pathsChanged := cmd.Flag("strategy-path").Changed
	if strategyChanged || pathsChanged {
		var strategies []string
		var paths []string

		strategies, err = cmd.Flags().GetStringSlice("strategy")
		if err != nil {
			return err
		}

		paths, err = cmd.Flags().GetStringSlice("strategy-path")
		if err != nil {
			return err
		}

		if len(strategies) != len(paths) {
			return errors.New("number of strategies and paths must be equal")
		}

		list := map[string]bool{
			compose.StrategyOverwriteLocal:     true,
			compose.StrategyRemoveExtraLocal:   true,
			compose.StrategyIgnoreExtraPackage: true,
			compose.StrategyFilterPackage:      true,
		}

		for _, strategy := range strategies {
			if _, ok := list[strategy]; !ok {
				return fmt.Errorf("submitted strategy %s doesn't exist", strategy)
			}
		}
	}

	return err
}
