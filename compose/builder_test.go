package compose

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// newTestBuilder creates a Builder pointing at platformDir and targetDir with given packages.
// It creates an empty source directory for each package so build() can walk them.
func newTestBuilder(t *testing.T, platformDir, targetDir string, packages []*Package) *Builder {
	t.Helper()
	sourceDir := t.TempDir()
	for _, pkg := range packages {
		dir := filepath.Join(sourceDir, pkg.GetName(), pkg.GetTarget())
		if err := os.MkdirAll(dir, 0750); err != nil {
			t.Fatal(err)
		}
	}
	return &Builder{
		platformDir: platformDir,
		targetDir:   targetDir,
		sourceDir:   sourceDir,
		packages:    packages,
		requiredBy:  map[string][]string{},
	}
}

// writePlatformFile creates a file inside platformDir at the given relative path.
func writePlatformFile(t *testing.T, platformDir, relPath, content string) {
	t.Helper()
	full := filepath.Join(platformDir, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

// fakeFileInfo implements fs.FileInfo for testing.
type fakeFileInfo struct {
	name  string
	isDir bool
}

func (f fakeFileInfo) Name() string       { return f.name }
func (f fakeFileInfo) Size() int64        { return 0 }
func (f fakeFileInfo) Mode() fs.FileMode  { return 0644 }
func (f fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeFileInfo) IsDir() bool        { return f.isDir }
func (f fakeFileInfo) Sys() any           { return nil }

func makeEntry(prefix, path, from string) *fsEntry {
	return &fsEntry{Prefix: prefix, Path: path, Entry: fakeFileInfo{name: path}, From: from}
}

func indexOf(items []string, item string) int {
	for i, v := range items {
		if v == item {
			return i
		}
	}
	return -1
}

func makeStrategy(s mergeStrategyType, paths []string) []*mergeStrategy {
	return []*mergeStrategy{{s: s, t: packageStrategy, paths: cleanStrategyPaths(paths)}}
}

// buildItems simulates what build() does: topsort from a platform.
func buildItems(packages []*Package) []string {
	graph := buildDependenciesGraph(packages)
	items, _ := graph.TopSort(DependencyPlatform)
	return items
}

// --- buildDependenciesGraph ---

func TestDependencyGraph(t *testing.T) {
	tests := []struct {
		name     string
		packages []*Package
		check    func(t *testing.T, items []string)
	}{
		{
			name:     "no packages — platform is last",
			packages: nil,
			check: func(t *testing.T, items []string) {
				if items[len(items)-1] != DependencyPlatform {
					t.Errorf("platform should be last, got %v", items)
				}
			},
		},
		{
			name:     "flat packages — platform is last",
			packages: []*Package{{Name: "pkg1"}, {Name: "pkg2"}},
			check: func(t *testing.T, items []string) {
				if items[len(items)-1] != DependencyPlatform {
					t.Errorf("platform should be last, got %v", items)
				}
			},
		},
		{
			name: "dependency processed before dependent",
			// pkgA depends on pkgB — pkgB first, pkgA after, platform last.
			// With last-wins, this means pkgA's files override pkgB's files.
			packages: []*Package{
				{Name: "pkgA", Dependencies: []string{"pkgB"}},
				{Name: "pkgB"},
			},
			check: func(t *testing.T, items []string) {
				idxA, idxB, idxP := indexOf(items, "pkgA"), indexOf(items, "pkgB"), indexOf(items, DependencyPlatform)
				if idxA == -1 || idxB == -1 {
					t.Fatalf("packages missing in sort result: %v", items)
				}
				if idxB > idxA {
					t.Errorf("pkgB (dependency, %d) should come before pkgA (%d) in %v", idxB, idxA, items)
				}
				if idxA > idxP {
					t.Errorf("pkgA (%d) should come before platform (%d) in %v", idxA, idxP, items)
				}
			},
		},
		{
			name: "top-level packages follow YAML order — last declared wins",
			packages: []*Package{
				{Name: "first"},
				{Name: "second"},
				{Name: "third"},
			},
			check: func(t *testing.T, items []string) {
				idxFirst, idxSecond, idxThird, idxP :=
					indexOf(items, "first"), indexOf(items, "second"), indexOf(items, "third"), indexOf(items, DependencyPlatform)
				if idxFirst >= idxSecond || idxSecond >= idxThird || idxThird >= idxP {
					t.Errorf("expected first < second < third < platform, got %v", items)
				}
			},
		},
		{
			// Simulates a realistic plasma-compose.yaml:
			//
			//   dependencies:
			//     - name: pkg-a        # depends on lib-common
			//     - name: pkg-b        # depends on lib-common (shared), listed after pkg-a
			//     - name: pkg-c        # independent, last in YAML → wins all conflicts
			//
			// lib-common itself depends on lib-base, which depends on lib-core (depth=3).
			// packages slice reflects downloadManager output: deps appended before dependents.
			//
			// Expected processing order:
			//   lib-core → lib-base → lib-common → pkg-a → pkg-b → pkg-c → platform
			//
			// pkg-c wins over pkg-b wins over pkg-a (YAML order, last-wins).
			// pkg-a and pkg-b both win over their shared lib-common.
			name: "deep deps + shared lib + YAML order",
			packages: []*Package{
				{Name: "lib-core"},
				{Name: "lib-base", Dependencies: []string{"lib-core"}},
				{Name: "lib-common", Dependencies: []string{"lib-base"}},
				{Name: "pkg-a", Dependencies: []string{"lib-common"}},
				{Name: "pkg-b", Dependencies: []string{"lib-common"}},
				{Name: "pkg-c"},
			},
			check: func(t *testing.T, items []string) {
				idx := func(n string) int { return indexOf(items, n) }

				order := []struct{ before, after string }{
					// dependency chain
					{"lib-core", "lib-base"},
					{"lib-base", "lib-common"},
					{"lib-common", "pkg-a"},
					{"lib-common", "pkg-b"},
					// YAML order between siblings
					{"pkg-a", "pkg-b"},
					{"pkg-b", "pkg-c"},
					// platform always last
					{"pkg-c", DependencyPlatform},
				}

				for _, o := range order {
					a, b := idx(o.before), idx(o.after)
					if a == -1 || b == -1 {
						t.Errorf("%q or %q missing in %v", o.before, o.after, items)
					} else if a > b {
						t.Errorf("expected %q (%d) before %q (%d) in %v", o.before, a, o.after, b, items)
					}
				}
			},
		},
		{
			name: "all packages before platform",
			packages: []*Package{
				{Name: "base"},
				{Name: "infra", Dependencies: []string{"base"}},
				{Name: "app", Dependencies: []string{"infra"}},
			},
			check: func(t *testing.T, items []string) {
				idxPlatform := indexOf(items, DependencyPlatform)
				for _, name := range []string{"base", "infra", "app"} {
					idx := indexOf(items, name)
					if idx == -1 {
						t.Errorf("package %q missing in sort result", name)
					} else if idx > idxPlatform {
						t.Errorf("package %q (%d) should come before platform (%d)", name, idx, idxPlatform)
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.check(t, buildItems(tt.packages))
		})
	}
}

// --- addEntries (last writer wins) ---

func TestAddEntries(t *testing.T) {
	tests := []struct {
		name          string
		existing      *fsEntry // nil = empty state
		incoming      *fsEntry
		wantResolve   mergeConflictResolve
		wantTreeLen   int
		wantFrom      string
		wantPrefix    string
		wantTree0From string // if set, checks tree[0].From (pointer update)
	}{
		{
			name:        "new path — added to tree",
			existing:    nil,
			incoming:    makeEntry("prefix", "file.txt", "pkg1"),
			wantResolve: noConflict,
			wantTreeLen: 1,
			wantFrom:    "pkg1",
			wantPrefix:  "prefix",
		},
		{
			name:          "existing path — last wins",
			existing:      makeEntry("prefix1", "file.txt", "pkg1"),
			incoming:      makeEntry("prefix2", "file.txt", "pkg2"),
			wantResolve:   resolveToPackage,
			wantTreeLen:   1,
			wantFrom:      "pkg2",
			wantPrefix:    "prefix2",
			wantTree0From: "pkg2",
		},
		{
			name:          "platform overrides package",
			existing:      makeEntry("pkg-prefix", "config.yaml", "pkg1"),
			incoming:      makeEntry("platform-prefix", "config.yaml", DependencyPlatform),
			wantResolve:   resolveToPackage,
			wantTreeLen:   1,
			wantFrom:      DependencyPlatform,
			wantPrefix:    "platform-prefix",
			wantTree0From: DependencyPlatform,
		},
	}

	// Extra: three packages on the same file — each overwrites the previous.
	t.Run("three packages — last always wins", func(t *testing.T) {
		const lastPkg = "pkg3"
		var tree []*fsEntry
		m := map[string]*fsEntry{}

		for _, from := range []string{"pkg1", "pkg2", lastPkg} {
			tree, _ = addEntries(tree, m, makeEntry("prefix-"+from, "file.txt", from), "file.txt")
		}

		if len(tree) != 1 {
			t.Errorf("tree len: got %d, want 1", len(tree))
		}
		if m["file.txt"].From != lastPkg {
			t.Errorf("From: got %q, want %s", m["file.txt"].From, lastPkg)
		}
		if tree[0].From != lastPkg {
			t.Errorf("tree[0].From: got %q, want %s", tree[0].From, lastPkg)
		}
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var tree []*fsEntry
			m := map[string]*fsEntry{}
			path := tt.incoming.Path

			if tt.existing != nil {
				tree = append(tree, tt.existing)
				m[path] = tt.existing
			}

			tree, resolve := addEntries(tree, m, tt.incoming, path)

			if resolve != tt.wantResolve {
				t.Errorf("resolve: got %d, want %d", resolve, tt.wantResolve)
			}
			if len(tree) != tt.wantTreeLen {
				t.Errorf("tree len: got %d, want %d", len(tree), tt.wantTreeLen)
			}
			if m[path].From != tt.wantFrom {
				t.Errorf("From: got %q, want %q", m[path].From, tt.wantFrom)
			}
			if m[path].Prefix != tt.wantPrefix {
				t.Errorf("Prefix: got %q, want %q", m[path].Prefix, tt.wantPrefix)
			}
			if tt.wantTree0From != "" && tree[0].From != tt.wantTree0From {
				t.Errorf("tree[0].From (pointer): got %q, want %q", tree[0].From, tt.wantTree0From)
			}
		})
	}
}

// --- addStrategyEntries ---

func TestAddStrategyEntries(t *testing.T) {
	tests := []struct {
		name         string
		strategy     mergeStrategyType
		stratPaths   []string
		existing     *fsEntry // nil = empty state
		incoming     *fsEntry
		wantTreeLen  int
		wantFrom     string // expected m[path].From; empty = entry not in map
		wantNotInMap bool
	}{
		{
			name:        "filter — matching path added if absent",
			strategy:    filterPackageFiles,
			stratPaths:  []string{"include/"},
			existing:    nil,
			incoming:    makeEntry("prefix", "include/file.txt", "pkg1"),
			wantTreeLen: 1,
			wantFrom:    "pkg1",
		},
		{
			name:        "filter — matching path overwrites existing (last-wins within whitelist)",
			strategy:    filterPackageFiles,
			stratPaths:  []string{"include/"},
			existing:    makeEntry("prefix1", "include/file.txt", "pkg1"),
			incoming:    makeEntry("prefix2", "include/file.txt", "pkg2"),
			wantTreeLen: 1,
			wantFrom:    "pkg2", // last wins
		},
		{
			name:         "filter — non-matching path skipped",
			strategy:     filterPackageFiles,
			stratPaths:   []string{"include/"},
			existing:     nil,
			incoming:     makeEntry("prefix", "other/file.txt", "pkg1"),
			wantTreeLen:  0,
			wantNotInMap: true,
		},
		{
			name:         "ignore — matching path skipped",
			strategy:     ignoreExtraPackageFiles,
			stratPaths:   []string{"vendor/"},
			existing:     nil,
			incoming:     makeEntry("prefix", "vendor/lib.go", "pkg1"),
			wantTreeLen:  0,
			wantNotInMap: true,
		},
		{
			name:        "ignore — matching path already in map — skips incoming, keeps existing",
			strategy:    ignoreExtraPackageFiles,
			stratPaths:  []string{"vendor/"},
			existing:    makeEntry("prefix1", "vendor/lib.go", "pkg1"),
			incoming:    makeEntry("prefix2", "vendor/lib.go", "pkg2"),
			wantTreeLen: 1,
			wantFrom:    "pkg1", // existing stays, incoming ignored
		},
		{
			name:        "ignore — non-matching path falls through (added)",
			strategy:    ignoreExtraPackageFiles,
			stratPaths:  []string{"vendor/"},
			existing:    nil,
			incoming:    makeEntry("prefix", "src/file.go", "pkg1"),
			wantTreeLen: 1,
			wantFrom:    "pkg1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var tree []*fsEntry
			m := map[string]*fsEntry{}
			path := tt.incoming.Path

			if tt.existing != nil {
				tree = append(tree, tt.existing)
				m[path] = tt.existing
			}

			strategies := makeStrategy(tt.strategy, tt.stratPaths)
			tree, _ = addStrategyEntries(strategies, tree, m, tt.incoming, path)

			if len(tree) != tt.wantTreeLen {
				t.Errorf("tree len: got %d, want %d", len(tree), tt.wantTreeLen)
			}
			if tt.wantNotInMap {
				if _, ok := m[path]; ok {
					t.Errorf("entry should not be in map for path %q", path)
				}
			} else if m[path].From != tt.wantFrom {
				t.Errorf("From: got %q, want %q", m[path].From, tt.wantFrom)
			}
		})
	}
}

// --- remove-extra-local-files (platform walk filter) ---

func TestRemoveExtraLocalFiles(t *testing.T) {
	tests := []struct {
		name          string
		stratPaths    []string
		platformFiles []string
		wantInBuild   []string
		wantExcluded  []string
	}{
		{
			name:       "matching directory excluded from build",
			stratPaths: []string{"scripts/"},
			platformFiles: []string{
				"scripts/run.sh",
				"scripts/deploy.sh",
				"config/base.yml",
				"README.md",
			},
			wantInBuild:  []string{"config/base.yml", "README.md"},
			wantExcluded: []string{"scripts/run.sh", "scripts/deploy.sh"},
		},
		{
			name:       "non-matching files not affected",
			stratPaths: []string{"tmp/"},
			platformFiles: []string{
				"tmp/cache.bin",
				"src/main.go",
			},
			wantInBuild:  []string{"src/main.go"},
			wantExcluded: []string{"tmp/cache.bin"},
		},
		{
			name:       "multiple excluded paths",
			stratPaths: []string{"scripts/", "tmp/"},
			platformFiles: []string{
				"scripts/run.sh",
				"tmp/cache.bin",
				"config/app.yml",
			},
			wantInBuild:  []string{"config/app.yml"},
			wantExcluded: []string{"scripts/run.sh", "tmp/cache.bin"},
		},
		{
			name:       "empty strategy paths — nothing excluded",
			stratPaths: []string{},
			platformFiles: []string{
				"scripts/run.sh",
				"config/app.yml",
			},
			wantInBuild:  []string{"scripts/run.sh", "config/app.yml"},
			wantExcluded: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			platformDir := t.TempDir()
			targetDir := t.TempDir()

			for _, f := range tt.platformFiles {
				writePlatformFile(t, platformDir, f, "content")
			}

			pkg := &Package{
				Name: "test-pkg",
				Source: Source{
					Type: PathType,
					Strategies: []Strategy{
						{Name: StrategyRemoveExtraLocal, Paths: tt.stratPaths},
					},
				},
			}

			b := newTestBuilder(t, platformDir, targetDir, []*Package{pkg})
			if err := b.build(context.Background()); err != nil {
				t.Fatalf("build() error: %v", err)
			}

			for _, f := range tt.wantInBuild {
				if _, err := os.Stat(filepath.Join(targetDir, f)); err != nil {
					t.Errorf("expected %q in build, but not found", f)
				}
			}
			for _, f := range tt.wantExcluded {
				if _, err := os.Stat(filepath.Join(targetDir, f)); !os.IsNotExist(err) {
					t.Errorf("expected %q to be excluded from build, but it exists", f)
				}
			}
		})
	}
}

// --- diamond dependency in build ---

func TestBuildDiamond(t *testing.T) {
	// Graph:       platform
	//             /        \
	//         pkg-a        pkg-b
	//             \        /
	//            lib-common
	//
	// Topsort: lib-common → pkg-a → pkg-b → platform
	// shared.txt conflict resolution: pkg-b wins (last in YAML).

	platformDir := t.TempDir()
	targetDir := t.TempDir()
	sourceDir := t.TempDir()

	// Write package source files.
	writeSourceFile := func(pkg, ref, relPath, content string) {
		t.Helper()
		full := filepath.Join(sourceDir, pkg, ref, relPath)
		if err := os.MkdirAll(filepath.Dir(full), 0750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}

	writeSourceFile("lib-common", "latest", "lib/common.txt", "lib-common content")
	writeSourceFile("lib-common", "latest", "shared.txt", "from lib-common")
	writeSourceFile("pkg-a", "latest", "pkg-a.txt", "pkg-a content")
	writeSourceFile("pkg-a", "latest", "shared.txt", "from pkg-a")
	writeSourceFile("pkg-b", "latest", "pkg-b.txt", "pkg-b content")
	writeSourceFile("pkg-b", "latest", "shared.txt", "from pkg-b")

	packages := []*Package{
		{Name: "lib-common"},
		{Name: "pkg-a", Dependencies: []string{"lib-common"}},
		{Name: "pkg-b", Dependencies: []string{"lib-common"}},
	}

	b := &Builder{
		platformDir: platformDir,
		targetDir:   targetDir,
		sourceDir:   sourceDir,
		packages:    packages,
		requiredBy:  map[string][]string{},
	}

	if err := b.build(context.Background()); err != nil {
		t.Fatalf("build() error: %v", err)
	}

	// Unique files from each package must appear.
	for _, f := range []string{"lib/common.txt", "pkg-a.txt", "pkg-b.txt"} {
		if _, err := os.Stat(filepath.Join(targetDir, f)); err != nil {
			t.Errorf("expected %q in build: %v", f, err)
		}
	}

	// shared.txt: pkg-b is last in YAML → wins.
	content, err := os.ReadFile(filepath.Clean(filepath.Join(targetDir, "shared.txt")))
	if err != nil {
		t.Fatalf("shared.txt not found: %v", err)
	}
	if string(content) != "from pkg-b" {
		t.Errorf("shared.txt: got %q, want %q", string(content), "from pkg-b")
	}
}
