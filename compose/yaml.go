package compose

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// YamlCompose stores compose definition
type YamlCompose struct {
	Name         string       `yaml:"name"`
	Dependencies []Dependency `yaml:"dependencies,omitempty"`
}

// YamlLock stores lock definition
type YamlLock struct {
	Hash     string     `yaml:"hash"`
	Packages []*Package `yaml:"packages,omitempty"`
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

// Source stores package source definition
type Source struct {
	Type string `yaml:"type"`
	URL  string `yaml:"url"`
	Ref  string `yaml:"ref,omitempty"`
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

// GetName from package
func (p *Package) GetName() string {
	return p.Name
}

// GetType from package source
func (p *Package) GetType() string {
	t := p.Source.Type
	if t == "" {
		return gitType
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

func parseComposeYaml(input []byte) (*YamlCompose, error) {
	cfg := YamlCompose{}
	err := yaml.Unmarshal(input, &cfg)
	return &cfg, err
}

func parseLockYaml(input []byte) (*YamlLock, error) {
	cfg := YamlLock{}
	err := yaml.Unmarshal(input, &cfg)
	return &cfg, err
}

func (l *YamlLock) save(path string) error {
	data, err := yaml.Marshal(l.Packages)
	if err != nil {
		return err
	}

	hash := getHashSum(data)
	if hash != l.Hash {
		l.Hash = hash
		data, err := yaml.Marshal(l)
		if err != nil {
			return err
		}
		err = os.WriteFile(path, data, 0600)
		return err
	}

	return nil
}

func getHashSum(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}
