// Package compose implements a launchr plugin to do platform composition
package compose

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/launchrctl/keyring"
	"github.com/launchrctl/launchr"
	"github.com/launchrctl/launchr/pkg/action"

	"github.com/launchrctl/compose/compose"
)

var (
	//go:embed action.compose.yaml
	actionComposeYaml []byte
	//go:embed action.add.yaml
	actionAddYaml []byte
	//go:embed action.update.yaml
	actionUpdateYaml []byte
	//go:embed action.delete.yaml
	actionDeleteYaml []byte
)

func init() {
	launchr.RegisterPlugin(&Plugin{})
}

// Plugin is [launchr.Plugin] plugin providing compose.
type Plugin struct {
	wd string
	k  keyring.Keyring
}

// PluginInfo implements [launchr.Plugin] interface.
func (p *Plugin) PluginInfo() launchr.PluginInfo {
	return launchr.PluginInfo{Weight: 10}
}

// OnAppInit implements [launchr.OnAppInitPlugin] interface.
func (p *Plugin) OnAppInit(app launchr.App) error {
	app.GetService(&p.k)
	p.wd = app.GetWD()
	buildDir := filepath.Join(p.wd, compose.BuildDir)
	app.RegisterFS(action.NewDiscoveryFS(os.DirFS(buildDir), p.wd))
	return nil
}

// DiscoverActions implements [launchr.ActionDiscoveryPlugin] interface.
func (p *Plugin) DiscoverActions(_ context.Context) ([]*action.Action, error) {
	// Action compose.
	composeAction := action.NewFromYAML("compose", actionComposeYaml)
	composeAction.SetRuntime(action.NewFnRuntime(func(_ context.Context, a *action.Action) error {
		input := a.Input()
		log, term := getLogger(a)
		c, err := compose.CreateComposer(
			p.wd,
			compose.ComposerOptions{
				Clean:              input.Opt("clean").(bool),
				WorkingDir:         input.Opt("working-dir").(string),
				SkipNotVersioned:   input.Opt("skip-not-versioned").(bool),
				ConflictsVerbosity: input.Opt("conflicts-verbosity").(bool),
				Interactive:        input.Opt("interactive").(bool),
			},
			p.k,
		)
		c.SetLogger(log)
		c.SetTerm(term)

		if err != nil {
			return err
		}

		return c.RunInstall()
	}))

	// Action compose:add.
	addAction := action.NewFromYAML("compose:add", actionAddYaml)
	addAction.SetRuntime(action.NewFnRuntime(func(_ context.Context, a *action.Action) error {
		input := a.Input()
		if err := packagePreRunValidate(input); err != nil {
			return err
		}
		createNew := input.Opt("allow-create").(bool)
		composeDependency := getInputDependencies(input)
		strategies := getInputStrategies(input)

		fa := prepareFormsAction(a)
		return fa.AddPackage(createNew, composeDependency, strategies, p.wd)
	}))

	// Action compose:update.
	updateAction := action.NewFromYAML("compose:update", actionUpdateYaml)
	updateAction.SetRuntime(action.NewFnRuntime(func(_ context.Context, a *action.Action) error {
		input := a.Input()
		if err := packagePreRunValidate(input); err != nil {
			return err
		}
		composeDependency := getInputDependencies(input)
		strategies := getInputStrategies(input)

		fa := prepareFormsAction(a)
		if composeDependency.Name != "" {
			return fa.UpdatePackage(composeDependency, strategies, p.wd)
		}

		return fa.UpdatePackages(p.wd)
	}))

	// Action compose:delete.
	deleteAction := action.NewFromYAML("compose:delete", actionDeleteYaml)
	deleteAction.SetRuntime(action.NewFnRuntime(func(_ context.Context, a *action.Action) error {
		input := a.Input()
		toDeletePackages := action.InputOptSlice[string](input, "packages")
		fa := prepareFormsAction(a)
		return fa.DeletePackages(toDeletePackages, p.wd)
	}))

	return []*action.Action{
		composeAction,
		addAction,
		updateAction,
		deleteAction,
	}, nil
}

func prepareFormsAction(a *action.Action) *compose.FormsAction {
	log, term := getLogger(a)
	fa := &compose.FormsAction{}
	fa.SetLogger(log)
	fa.SetTerm(term)

	return fa
}

func getLogger(a *action.Action) (*launchr.Logger, *launchr.Terminal) {
	log := launchr.Log()
	if rt, ok := a.Runtime().(action.RuntimeLoggerAware); ok {
		log = rt.LogWith()
	}

	term := launchr.Term()
	if rt, ok := a.Runtime().(action.RuntimeTermAware); ok {
		term = rt.Term()
	}

	return log, term
}

func getInputDependencies(input *action.Input) *compose.Dependency {
	return &compose.Dependency{
		Name: input.Opt("package").(string),
		Source: compose.Source{
			Type: input.Opt("type").(string),
			Ref:  input.Opt("ref").(string),
			Tag:  input.Opt("tag").(string),
			URL:  input.Opt("url").(string),
		},
	}
}

func getInputStrategies(input *action.Input) *compose.RawStrategies {
	return &compose.RawStrategies{
		Names: action.InputOptSlice[string](input, "strategy"),
		Paths: action.InputOptSlice[string](input, "strategy-path"),
	}
}

func packagePreRunValidate(input *action.Input) error {
	typeFlag := input.Opt("type").(string)

	if typeFlag == compose.HTTPType {
		refChanged := input.Opt("ref").(string) != ""
		if refChanged {
			input.SetOpt("ref", "")
		}
	}

	strategies := action.InputOptSlice[string](input, "strategy")
	paths := action.InputOptSlice[string](input, "strategy-path")
	if len(strategies) > 0 || len(paths) > 0 {
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

	return nil
}
