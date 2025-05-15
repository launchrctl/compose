package compose

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"dario.cat/mergo"
	"github.com/charmbracelet/huh"
	"github.com/launchrctl/launchr/pkg/action"
)

// RawStrategies represents collection of submitted flags for strategies.
type RawStrategies struct {
	Names []string
	Paths []string
}

// FormsAction provides form operations with compose file.
type FormsAction struct {
	action.WithLogger
	action.WithTerm
}

// AddPackage adds a new package to plasma-compose.
func (f *FormsAction) AddPackage(doCreate bool, newDependency *Dependency, rawStrategies *RawStrategies, dir string) error {
	config, err := Lookup(os.DirFS(dir))
	if err != nil {
		if !errors.Is(err, errComposeNotExists) {
			return err
		}

		if !doCreate {
			createNew := false
			err = huh.NewConfirm().
				Title("Plasma-compose doesn't exist, would you like to create default one ?").
				Value(&createNew).
				Run()
			if err != nil || !createNew {
				return err
			}
		}

		config = &YamlCompose{
			Name:         "plasma",
			Dependencies: []Dependency{},
		}
	}

	strategies := convertRawStrategies(rawStrategies)
	if len(strategies) > 0 {
		newDependency.Source.Strategies = strategies
	}

	if newDependency.Name == "" || newDependency.Source.URL == "" {
		form := preparePackageForm(newDependency, config, true)
		err = form.Run()
		if err != nil {
			return err
		}

		err = f.processStrategiesForm(newDependency)
		if err != nil {
			return err
		}
	} else {
		for _, originalDep := range config.Dependencies {
			if originalDep.Name == newDependency.Name {
				return fmt.Errorf("package with the same name %s already exists", newDependency.Name)
			}

			if originalDep.Source.URL == newDependency.Source.URL {
				return fmt.Errorf("package with the same URL as %s already exists", newDependency.Name)
			}
		}
	}

	sanitizeDependency(newDependency)
	config.Dependencies = append(config.Dependencies, *newDependency)
	f.Term().Println("Saving plasma-compose...")
	sortPackages(config)
	err = writeComposeYaml(config)

	return err
}

// UpdatePackage updates a single package in plasma-compose.
func (f *FormsAction) UpdatePackage(dependency *Dependency, rawStrategies *RawStrategies, dir string) error {
	config, err := Lookup(os.DirFS(dir))
	if err != nil {
		return err
	}

	var toUpdate *Dependency
	for i := range config.Dependencies {
		if config.Dependencies[i].Name == dependency.Name {
			toUpdate = &config.Dependencies[i]
			continue
		}

		if config.Dependencies[i].Source.URL == dependency.Source.URL {
			return errors.New("URL you trying to set is present in other package")
		}

	}

	if toUpdate == nil {
		return errors.New("no package to update")
	}

	strategies := convertRawStrategies(rawStrategies)
	if len(strategies) > 0 {
		dependency.Source.Strategies = strategies
	}

	if err = mergo.Merge(toUpdate, dependency, mergo.WithOverride); err != nil {
		return err
	}

	sanitizeDependency(toUpdate)
	f.Term().Println("Saving plasma-compose...")
	sortPackages(config)
	err = writeComposeYaml(config)

	return err
}

// UpdatePackages updates packages in plasma-compose in interactive way.
func (f *FormsAction) UpdatePackages(dir string) error {
	config, err := Lookup(os.DirFS(dir))
	if err != nil {
		return err
	}

	packagesMap := make(map[string]*Dependency)
	var options []huh.Option[string]

	for i := range config.Dependencies {
		packagesMap[config.Dependencies[i].Name] = &config.Dependencies[i]
		options = append(options, huh.NewOption(config.Dependencies[i].Name, config.Dependencies[i].Name))
	}

	continueUpdating := true
	for continueUpdating {
		var selectedPackage string

		form := huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("Packages").
					Options(options...).
					Value(&selectedPackage),
			),
		)

		err = form.Run()
		if err != nil {
			return err
		}

		selectedDep := packagesMap[selectedPackage]

		formEdit := preparePackageForm(selectedDep, config, false)
		err = formEdit.Run()
		if err != nil {
			return err
		}

		err = f.processStrategiesForm(selectedDep)
		if err != nil {
			return err
		}

		sanitizeDependency(selectedDep)
		err = huh.NewConfirm().
			Title("Do you want to edit other packages?").
			Value(&continueUpdating).
			Run()

		if err != nil {
			return err
		}
	}

	f.Term().Println("Saving plasma-compose...")
	var newDeps []Dependency
	for _, dep := range packagesMap {
		newDeps = append(newDeps, *dep)
	}

	config.Dependencies = newDeps
	sortPackages(config)
	err = writeComposeYaml(config)

	return err
}

// DeletePackages removes packages plasma-compose.
func (f *FormsAction) DeletePackages(packages []string, dir string) error {
	config, err := Lookup(os.DirFS(dir))
	if err != nil {
		return err
	}

	// Ask user to select packages to remove.
	if len(packages) == 0 {
		var toDelete string
		var deleteOptions []huh.Option[string]
		for _, dep := range config.Dependencies {
			deleteOptions = append(deleteOptions, huh.NewOption(dep.Name, dep.Name))
		}

		form := huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("Packages").
					Options(deleteOptions...).
					Value(&toDelete),
			))

		err = form.Run()
		if err != nil {
			return err
		}

		packages = append(packages, toDelete)
	}

	var dependencies []Dependency
	saveRequired := false

OUTER:
	for _, dep := range config.Dependencies {
		for _, pkg := range packages {
			if dep.Name == pkg {
				saveRequired = true
				continue OUTER
			}
		}

		dependencies = append(dependencies, dep)
	}

	if saveRequired {
		f.Term().Println("Updating plasma-compose...")
		config.Dependencies = dependencies
		sortPackages(config)
		err = writeComposeYaml(config)
	} else {
		f.Term().Println("Nothing to update, quiting")
	}

	return err
}

func (f *FormsAction) processStrategiesForm(dependency *Dependency) error {
	var addStrategies bool
	err := huh.NewConfirm().
		Title("Would you like to add strategies?").
		Value(&addStrategies).
		Run()

	if err != nil {
		return err
	}

	if addStrategies {
		var strategies []Strategy

		strategiesQueue := true
		for strategiesQueue {
			var selectedStrategy string
			var strategyPaths string
			formStrategy := huh.NewForm(
				huh.NewGroup(
					huh.NewSelect[string]().
						Title("Strategies").
						Options(
							huh.NewOption("Overwrite Local File", StrategyOverwriteLocal),
							huh.NewOption("Remove Extra Local Files", StrategyRemoveExtraLocal),
							huh.NewOption("Ignore Extra Package", StrategyIgnoreExtraPackage),
							huh.NewOption("Filter Package Files", StrategyFilterPackage),
						).
						Value(&selectedStrategy),

					huh.NewText().
						Title("Paths").
						Value(&strategyPaths),
				))

			err = formStrategy.Run()
			if err != nil {
				return err
			}

			lines := strings.Split(strategyPaths, "\n")
			var paths []string
			for _, line := range lines {
				path := strings.TrimSpace(line)
				paths = append(paths, path)
			}

			strategies = append(strategies, Strategy{Name: selectedStrategy, Paths: paths})

			err = huh.NewConfirm().
				Title("Add other strategy").
				Value(&strategiesQueue).
				Run()

			if err != nil {
				return err
			}
		}

		dependency.Source.Strategies = strategies
	}

	return nil
}

func preparePackageForm(dependency *Dependency, config *YamlCompose, isAdd bool) *huh.Form {
	uniqueLimit := 1
	if isAdd {
		uniqueLimit = 0
	}

	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("- Enter package name").
				Value(&dependency.Name).
				Validate(func(str string) error {
					if str == "" {
						return errors.New("package name can't be empty")
					}

					unique := 0
					for _, originalDep := range config.Dependencies {
						if originalDep.Name == str {
							unique++
						}
					}

					if unique > uniqueLimit {
						return errors.New("package with the same name already exists, please choose other name")
					}

					return nil
				}),

			huh.NewSelect[string]().
				Title("- Select source type").
				Options(
					huh.NewOption("Git", GitType).Selected(true),
					huh.NewOption("Http", HTTPType),
				).
				Value(&dependency.Source.Type),

			huh.NewInput().
				Title("- Enter package URL").
				Value(&dependency.Source.URL).
				Validate(func(str string) error {
					if str == "" {
						return errors.New("URL can't be empty")
					}

					unique := 0
					for _, originalDep := range config.Dependencies {
						if originalDep.Source.URL == str {
							unique++
						}
					}

					if unique > uniqueLimit {
						return errors.New("package with the same URL already exists")
					}

					return nil
				}),
		),

		huh.NewGroup(
			huh.NewInput().
				Title("- Enter Ref").
				Value(&dependency.Source.Ref),
		).WithHideFunc(func() bool { return dependency.Source.Type != GitType }),
	)
}

func convertRawStrategies(input *RawStrategies) []Strategy {
	var strategies []Strategy

	for i := range input.Names {
		paths := strings.Split(input.Paths[i], "|")

		for y, path := range paths {
			paths[y] = strings.TrimSpace(path)
		}

		strategy := Strategy{
			Name:  input.Names[i],
			Paths: paths,
		}

		strategies = append(strategies, strategy)
	}

	return strategies
}

func sortPackages(config *YamlCompose) {
	sort.Slice(config.Dependencies, func(i, j int) bool {
		return config.Dependencies[i].Name < config.Dependencies[j].Name
	})
}

func sanitizeDependency(dependency *Dependency) {
	dependency.Name = strings.TrimSpace(dependency.Name)
	dependency.Source.URL = strings.TrimSpace(dependency.Source.URL)
	dependency.Source.Ref = strings.TrimSpace(dependency.Source.Ref)
}
