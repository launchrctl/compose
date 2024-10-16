package compose

import (
	"fmt"
	"io/fs"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	composePermissions uint32 = 0644
)

// YamlCompose stores compose definition
type YamlCompose struct {
	Name         string       `yaml:"name"`
	Dependencies []Dependency `yaml:"dependencies,omitempty"`
}

// Package stores package definition
type Package struct {
	Name         string   `yaml:"name"`
	Source       Source   `yaml:"source,omitempty"`
	Dependencies []string `yaml:"dependencies,omitempty"`
}

// Dependency stores Dependency definition
type Dependency struct {
	Name   string `yaml:"name"`
	Source Source `yaml:"source,omitempty"`
}

// Strategy stores packages merge strategy name and Paths
type Strategy struct {
	Name  string   `yaml:"name"`
	Paths []string `yaml:"path"`
}

// Source stores package source definition
type Source struct {
	Type       string     `yaml:"type"`
	URL        string     `yaml:"url"`
	Ref        string     `yaml:"ref,omitempty"`
	Tag        string     `yaml:"tag,omitempty"`
	Strategies []Strategy `yaml:"strategy,omitempty"`
}

// ToPackage converts dependency to package
func (d *Dependency) ToPackage(name string) *Package {
	return &Package{
		Name:   name,
		Source: d.Source,
	}
}

// AddDependency appends new package dependency
func (p *Package) AddDependency(dep string) {
	p.Dependencies = append(p.Dependencies, dep)
}

// GetStrategies from package
func (p *Package) GetStrategies() []Strategy {
	return p.Source.Strategies
}

// GetName from package
func (p *Package) GetName() string {
	return p.Name
}

// GetType from package source
func (p *Package) GetType() string {
	t := p.Source.Type
	if t == "" {
		return GitType
	}

	return strings.ToLower(t)
}

// GetURL from package source
func (p *Package) GetURL() string {
	return p.Source.URL
}

// GetRef from package source
func (p *Package) GetRef() string {
	return p.Source.Ref
}

// GetTag from package source
func (p *Package) GetTag() string {
	return p.Source.Tag
}

// GetTarget returns a target version of package
func (p *Package) GetTarget() string {
	target := "latest"

	if p.GetRef() != "" {
		target = p.GetRef()
	} else if p.GetTag() != "" {
		target = p.GetTag()
	}

	return target
}

// Lookup allows to search compose file, read and parse it.
func Lookup(fsys fs.FS) (*YamlCompose, error) {
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

func parseComposeYaml(input []byte) (*YamlCompose, error) {
	cfg := YamlCompose{}
	err := yaml.Unmarshal(input, &cfg)
	return &cfg, err
}

func writeComposeYaml(compose *YamlCompose) error {
	yamlContent, err := yaml.Marshal(compose)
	if err != nil {
		return fmt.Errorf("could not marshal struct into YAML: %v", err)
	}

	err = os.WriteFile(composeFile, yamlContent, os.FileMode(composePermissions))
	if err != nil {
		return err
	}

	return nil
}
