package compose

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/launchrctl/launchr/pkg/action"
	"github.com/stevenle/topsort"
)

const (
	// DependencyPlatform is the platform node in the dependencies graph, always processed last
	DependencyPlatform = "platform"
	gitPrefix          = ".git"
)

var excludedFolders = map[string]struct{}{".compose": {}}
var excludedFiles = map[string]struct{}{composeFile: {}}

type mergeConflictResolve uint8
type mergeStrategyType uint8
type mergeStrategyTarget uint8
type mergeStrategy struct {
	s     mergeStrategyType
	t     mergeStrategyTarget
	paths []string
}

const (
	undefinedStrategy       mergeStrategyType    = iota
	removeExtraLocalFiles   mergeStrategyType    = 1
	ignoreExtraPackageFiles mergeStrategyType    = 2
	filterPackageFiles      mergeStrategyType    = 3
	noConflict              mergeConflictResolve = iota
	resolveToPackage        mergeConflictResolve = 1
	localStrategy           mergeStrategyTarget  = 1
	packageStrategy         mergeStrategyTarget  = 2
)

var (

	// StrategyOverwriteLocal string const
	StrategyOverwriteLocal = "overwrite-local-file"
	// StrategyRemoveExtraLocal string const
	StrategyRemoveExtraLocal = "remove-extra-local-files"
	// StrategyIgnoreExtraPackage string const
	StrategyIgnoreExtraPackage = "ignore-extra-package-files"
	// StrategyFilterPackage string const
	StrategyFilterPackage = "filter-package-files"
)

// return conflict const (0 - no warning, 1 - conflict with local, 2 conflict with package)

func cleanStrategyPaths(paths []string) []string {
	// remove trailing separators and add only one separator at the end.
	// so prefix won't be greedy during comparison.
	var r []string

	for _, p := range paths {
		path := filepath.Clean(p)
		if !strings.HasSuffix(path, string(os.PathSeparator)) {
			path += string(os.PathSeparator)
		}

		r = append(r, path)
	}

	return r
}

func retrieveStrategies(packages []*Package) ([]*mergeStrategy, map[string][]*mergeStrategy) {
	var ls []*mergeStrategy
	ps := make(map[string][]*mergeStrategy)
	for _, pkg := range packages {
		var strategies []*mergeStrategy
		for _, item := range pkg.GetStrategies() {
			s, t := identifyStrategy(item.Name)
			if s == undefinedStrategy {
				continue
			}
			strategy := &mergeStrategy{s, t, cleanStrategyPaths(item.Paths)}

			if t == localStrategy {
				ls = append(ls, strategy)
			} else {
				strategies = append(strategies, strategy)
			}
		}
		ps[pkg.GetName()] = strategies
	}

	return ls, ps
}

func identifyStrategy(name string) (mergeStrategyType, mergeStrategyTarget) {
	s := undefinedStrategy
	t := packageStrategy

	switch name {
	case StrategyOverwriteLocal:
		// deprecated: no-op, default conflict resolution is last-writer-wins
	case StrategyRemoveExtraLocal:
		s = removeExtraLocalFiles
		t = localStrategy
	case StrategyIgnoreExtraPackage:
		s = ignoreExtraPackageFiles
	case StrategyFilterPackage:
		s = filterPackageFiles
	}

	return s, t
}

// Builder struct, provides methods to merge packages into build
type Builder struct {
	action.WithLogger
	action.WithTerm

	platformDir      string
	targetDir        string
	sourceDir        string
	skipNotVersioned bool
	logConflicts     bool
	packages         []*Package
	requiredBy       map[string][]string
}

type fsEntry struct {
	Prefix   string
	Path     string
	Entry    fs.FileInfo
	Excluded bool
	From     string
}

func createBuilder(c *Composer, targetDir, sourceDir string, packages []*Package, requiredBy map[string][]string) *Builder {
	return &Builder{
		c.WithLogger,
		c.WithTerm,
		c.pwd,
		targetDir,
		sourceDir,
		c.options.SkipNotVersioned,
		c.options.ConflictsVerbosity,
		packages,
		requiredBy,
	}
}

func getVersionedMap(gitDir string) (map[string]bool, error) {
	versionedFiles := make(map[string]bool)
	repo, err := git.PlainOpen(gitDir)
	if err != nil {
		return versionedFiles, err
	}
	head, err := repo.Head()
	if err != nil {
		return versionedFiles, err
	}

	commit, _ := repo.CommitObject(head.Hash())
	tree, _ := commit.Tree()
	err = tree.Files().ForEach(func(f *object.File) error {
		dir := filepath.Dir(f.Name)
		if _, ok := versionedFiles[dir]; !ok {
			versionedFiles[dir] = true
		}

		versionedFiles[f.Name] = true
		return nil
	})

	return versionedFiles, err
}

func (b *Builder) build(ctx context.Context) error {
	b.Term().Println("Creating composition...")
	err := EnsureDirExists(b.targetDir)
	if err != nil {
		return err
	}

	versionedMap := make(map[string]bool)
	checkVersioned := b.skipNotVersioned
	if checkVersioned {
		versionedMap, err = getVersionedMap(b.platformDir)
		if err != nil {
			checkVersioned = false
		}
	}

	ls, ps := retrieveStrategies(b.packages)

	for _, pkg := range b.packages {
		for _, s := range pkg.GetStrategies() {
			if s.Name == StrategyOverwriteLocal {
				b.Term().Warning().Printfln("strategy %q in package %q is deprecated and has no effect, default conflict resolution is now last-writer-wins", StrategyOverwriteLocal, pkg.GetName())
			}
		}
	}

	entriesMap := make(map[string]*fsEntry)
	var entriesTree []*fsEntry

	graph := buildDependenciesGraph(b.packages)
	items, _ := graph.TopSort(DependencyPlatform)
	targetsMap := getTargetsMap(b.packages)

	if b.logConflicts {
		b.Term().Info().Printf("Conflicting files:\n")
	}

	for i := 0; i < len(items); i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			pkgName := items[i]
			switch pkgName {
			case DependencyPlatform:
				baseFs := os.DirFS(b.platformDir)
				err = fs.WalkDir(baseFs, ".", func(path string, d fs.DirEntry, err error) error {
					if err != nil {
						return err
					}

					root := rgxPathRoot.FindString(path)
					if _, ok := excludedFolders[root]; ok {
						return nil
					}

					if !d.IsDir() {
						filename := filepath.Base(path)
						if _, ok := excludedFiles[filename]; ok {
							return nil
						}
					}

					for _, localStrategy := range ls {
						if localStrategy.s == removeExtraLocalFiles {
							if ensureStrategyPrefixPath(path, localStrategy.paths) {
								return nil
							}
						}
					}

					if checkVersioned && !strings.HasPrefix(path, gitPrefix) {
						if _, ok := versionedMap[path]; !ok {
							return nil
						}
					}

					finfo, _ := d.Info()
					entry := &fsEntry{Prefix: b.platformDir, Path: path, Entry: finfo, Excluded: false, From: DependencyPlatform}
					var conflictReslv mergeConflictResolve
					entriesTree, conflictReslv = addEntries(entriesTree, entriesMap, entry, path)

					if b.logConflicts && !d.IsDir() {
						b.logConflictResolve(conflictReslv, path, DependencyPlatform, entriesMap[path])
					}

					return nil
				})

				if err != nil {
					return err
				}
			default:
				pkgPath := filepath.Join(b.sourceDir, pkgName, targetsMap[pkgName])
				packageFs := os.DirFS(pkgPath)
				strategies, ok := ps[pkgName]
				err = fs.WalkDir(packageFs, ".", func(path string, d fs.DirEntry, err error) error {
					if err != nil {
						return err
					}

					// Skip .git folder from packages
					if strings.HasPrefix(path, gitPrefix) {
						return nil
					}

					if !d.IsDir() {
						if _, excluded := excludedFiles[filepath.Base(path)]; excluded {
							return nil
						}
					}

					var conflictReslv mergeConflictResolve
					finfo, _ := d.Info()
					entry := &fsEntry{Prefix: pkgPath, Path: path, Entry: finfo, Excluded: false, From: pkgName}

					if !ok {
						// No strategies for package. Proceed with default merge.
						entriesTree, conflictReslv = addEntries(entriesTree, entriesMap, entry, path)
					} else {
						entriesTree, conflictReslv = addStrategyEntries(strategies, entriesTree, entriesMap, entry, path)
					}

					if b.logConflicts && !finfo.IsDir() {
						b.logConflictResolve(conflictReslv, path, pkgName, entriesMap[path])
					}

					return nil
				})

				if err != nil {
					return err
				}
			}
		}
	}

	// @todo check rsync
	for _, treeItem := range entriesTree {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			sourcePath := filepath.Join(treeItem.Prefix, treeItem.Path)
			destPath := filepath.Join(b.targetDir, treeItem.Path)
			isSymlink := false
			permissions := os.FileMode(dirPermissions)

			switch treeItem.Entry.Mode() & os.ModeType {
			case os.ModeDir:
				if err := createDir(destPath, treeItem.Entry.Mode()); err != nil {
					return err
				}
			case os.ModeSymlink:
				if err := lcopy(sourcePath, destPath); err != nil {
					return err
				}
				isSymlink = true
			default:
				permissions = treeItem.Entry.Mode()
				if err := fcopy(sourcePath, destPath); err != nil {
					return err
				}
			}

			if !isSymlink {
				if err := os.Chmod(destPath, permissions); err != nil {
					return err
				}
			}
		}
	}

	// Write lock file in topological order (same order used for file merging).
	// items contains DependencyPlatform as last entry — exclude it.
	pkgByName := make(map[string]*Package, len(b.packages))
	for _, pkg := range b.packages {
		pkgByName[pkg.GetName()] = pkg
	}
	sortedPackages := make([]*Package, 0, len(b.packages))
	for _, name := range items {
		if name == DependencyPlatform {
			continue
		}
		if pkg, ok := pkgByName[name]; ok {
			sortedPackages = append(sortedPackages, pkg)
		}
	}

	var lock *Lock
	lock, err = buildLock(sortedPackages, b.platformDir, b.sourceDir, b.requiredBy)
	if err != nil {
		return err
	}

	return writeLock(lock, b.targetDir)
}

func (b *Builder) logConflictResolve(resolveto mergeConflictResolve, path, pkgName string, entry *fsEntry) {
	if resolveto == noConflict {
		return
	}

	b.Term().Info().Printfln("[%s] - %s > Selected from %s", pkgName, path, entry.From)
}

func getTargetsMap(packages []*Package) map[string]string {
	targets := make(map[string]string)
	for _, p := range packages {
		targets[p.GetName()] = p.GetTarget()
	}

	return targets
}

func addEntries(entriesTree []*fsEntry, entriesMap map[string]*fsEntry, entry *fsEntry, path string) ([]*fsEntry, mergeConflictResolve) {
	existing, ok := entriesMap[path]
	if !ok {
		entriesTree = append(entriesTree, entry)
		entriesMap[path] = entry
		return entriesTree, noConflict
	}

	// Last writer wins: overwrite existing entry in-place so entriesTree pointer stays valid.
	existing.Prefix = entry.Prefix
	existing.Entry = entry.Entry
	existing.From = entry.From
	return entriesTree, resolveToPackage
}

func addStrategyEntries(strategies []*mergeStrategy, entriesTree []*fsEntry, entriesMap map[string]*fsEntry, entry *fsEntry, path string) ([]*fsEntry, mergeConflictResolve) {
	conflictResolve := noConflict

	// Apply strategies package strategies
	for _, ms := range strategies {
		switch ms.s {
		case filterPackageFiles:
			if ensureStrategyPrefixPath(path, ms.paths) || (entry.Entry.IsDir() && ensureStrategyContainsPath(path, ms.paths)) {
				entriesTree, conflictResolve = addEntries(entriesTree, entriesMap, entry, path)
			}

		case ignoreExtraPackageFiles:
			// Skip strategy if filepath does not match strategy Paths
			if !ensureStrategyPrefixPath(path, ms.paths) {
				continue
			}
			// just do nothing and skip
		}

		return entriesTree, conflictResolve
	}

	return addEntries(entriesTree, entriesMap, entry, path)
}

func ensureStrategyPrefixPath(path string, strategyPaths []string) bool {
	for _, sp := range strategyPaths {
		if strings.HasPrefix(path, sp) {
			return true
		}
	}

	return false
}

func ensureStrategyContainsPath(path string, strategyPaths []string) bool {
	for _, sp := range strategyPaths {
		if strings.Contains(sp, path) {
			return true
		}
	}

	return false
}

func buildDependenciesGraph(packages []*Package) *topsort.Graph {
	graph := topsort.NewGraph()
	packageNames := make(map[string]bool)

	for _, a := range packages {
		if _, k := packageNames[a.GetName()]; !k {
			packageNames[a.GetName()] = true
		}

		graph.AddNode(a.GetName())
		if a.Dependencies != nil {
			for _, d := range a.Dependencies {
				_ = graph.AddEdge(a.GetName(), d)
				packageNames[d] = false
			}
		}
	}

	// Platform is the topsort root — all top-level packages are its dependencies,
	// so they are processed first and platform always comes last.
	graph.AddNode(DependencyPlatform)

	// Collect top-level packages in YAML declaration order (preserved by the packages slice).
	// Later in YAML = processed later = wins with last-writer-wins.
	var topLevel []string
	seen := make(map[string]bool)
	for _, a := range packages {
		n := a.GetName()
		if packageNames[n] && !seen[n] {
			topLevel = append(topLevel, n)
			seen[n] = true
		}
	}

	for _, n := range topLevel {
		_ = graph.AddEdge(DependencyPlatform, n)
	}
	// Enforce YAML order between siblings: topLevel[i] processed before topLevel[i+1].
	for i := 1; i < len(topLevel); i++ {
		_ = graph.AddEdge(topLevel[i], topLevel[i-1])
	}

	return graph
}

func lcopy(src, dest string) error {
	src, err := os.Readlink(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return os.Symlink(src, dest)
}

func fcopy(src, dst string) error {
	sourceFileStat, err := os.Stat(src)
	if err != nil {
		return err
	}

	if !sourceFileStat.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", src)
	}

	source, err := os.Open(filepath.Clean(src))
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}

	if _, err := io.Copy(destination, source); err != nil {
		return err
	}

	return destination.Close()
}

func exists(path string) bool {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false
	}

	return true
}

func createDir(dir string, perm os.FileMode) error {
	if exists(dir) {
		return nil
	}

	if err := os.MkdirAll(dir, perm); err != nil {
		return fmt.Errorf("failed to create directory: '%s', error: '%s'", dir, err.Error())
	}

	return nil
}
