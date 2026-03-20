package compose

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// newTestDM creates a DownloadManager suitable for tests (no real keyring needed).
func newTestDM() DownloadManager {
	return CreateDownloadManager(&keyringWrapper{})
}

// fixturePath returns the absolute path to a named fixture directory.
func fixturePath(name string) string {
	abs, _ := filepath.Abs(filepath.Join("..", "test", "fixtures", "packages", name))
	return abs
}

// writeTestCompose writes a plasma-compose.yaml into dir.
func writeTestCompose(t *testing.T, dir string, deps []Dependency) {
	t.Helper()
	c := YamlCompose{Dependencies: deps}
	data, err := yaml.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, composeFile), data, 0600); err != nil {
		t.Fatal(err)
	}
}

// makePkg creates a temp dir, optionally copies a fixture into it, and optionally
// writes a plasma-compose.yaml with the given dependencies.
func makePkg(t *testing.T, fixtureName string, deps []Dependency) string {
	t.Helper()
	dir := t.TempDir()
	if fixtureName != "" {
		if err := copyDir(fixturePath(fixtureName), dir); err != nil {
			t.Fatalf("copy fixture %q: %v", fixtureName, err)
		}
	}
	if len(deps) > 0 {
		writeTestCompose(t, dir, deps)
	}
	return dir
}

// pathDep builds a Dependency with type "path" and ref "latest".
func pathDep(name, url string) Dependency {
	return Dependency{Name: name, Source: Source{Type: PathType, URL: url}}
}

// pathDepRef builds a Dependency with type "path" and a specific ref.
func pathDepRef(name, url, ref string) Dependency {
	return Dependency{Name: name, Source: Source{Type: PathType, URL: url, Ref: ref}}
}

// sortedPkgNames returns sorted package names from a slice.
func sortedPkgNames(pkgs []*Package) []string {
	names := make([]string, len(pkgs))
	for i, p := range pkgs {
		names[i] = p.GetName()
	}
	sort.Strings(names)
	return names
}

// --- TestDownloadManager ---

func TestDownloadManager(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T) *YamlCompose
		wantNames []string // sorted expected package names; nil skips check
		wantErr   string   // non-empty: expect error containing this substring
	}{
		{
			name: "flat packages — both downloaded",
			setup: func(_ *testing.T) *YamlCompose {
				return &YamlCompose{Dependencies: []Dependency{
					pathDep("pkg-a", fixturePath("pkg-a")),
					pathDep("pkg-b", fixturePath("pkg-b")),
				}}
			},
			wantNames: []string{"pkg-a", "pkg-b"},
		},
		{
			name: "deep dependency chain — all transitive deps downloaded",
			// Root → pkg-a → lib-common → lib-base
			setup: func(t *testing.T) *YamlCompose {
				libBase := makePkg(t, "lib-base", nil)
				libCommon := makePkg(t, "lib-common", []Dependency{
					pathDep("lib-base", libBase),
				})
				pkgA := makePkg(t, "pkg-a", []Dependency{
					pathDep("lib-common", libCommon),
				})
				return &YamlCompose{Dependencies: []Dependency{
					pathDep("pkg-a", pkgA),
				}}
			},
			wantNames: []string{"lib-base", "lib-common", "pkg-a"},
		},
		{
			name: "shared dependency — downloaded exactly once",
			// pkg-a and pkg-b both depend on lib-common → lib-common appears once.
			setup: func(t *testing.T) *YamlCompose {
				libCommon := makePkg(t, "lib-common", nil)
				pkgA := makePkg(t, "pkg-a", []Dependency{
					pathDep("lib-common", libCommon),
				})
				pkgB := makePkg(t, "pkg-b", []Dependency{
					pathDep("lib-common", libCommon),
				})
				return &YamlCompose{Dependencies: []Dependency{
					pathDep("pkg-a", pkgA),
					pathDep("pkg-b", pkgB),
				}}
			},
			wantNames: []string{"lib-common", "pkg-a", "pkg-b"},
		},
		{
			name: "version conflict — same name, different refs",
			setup: func(t *testing.T) *YamlCompose {
				libCore := makePkg(t, "lib-core", nil)
				pkgA := makePkg(t, "pkg-a", []Dependency{
					pathDepRef("lib-core", libCore, "v1"),
				})
				pkgB := makePkg(t, "pkg-b", []Dependency{
					pathDepRef("lib-core", libCore, "v2"),
				})
				return &YamlCompose{Dependencies: []Dependency{
					pathDep("pkg-a", pkgA),
					pathDep("pkg-b", pkgB),
				}}
			},
			wantErr: "version conflict",
		},
		{
			name: "version conflict — same name, different urls",
			setup: func(t *testing.T) *YamlCompose {
				libCoreA := makePkg(t, "lib-core", nil)
				libCoreB := makePkg(t, "lib-core", nil) // different path = different source
				pkgA := makePkg(t, "pkg-a", []Dependency{
					pathDep("lib-core", libCoreA),
				})
				pkgB := makePkg(t, "pkg-b", []Dependency{
					pathDep("lib-core", libCoreB),
				})
				return &YamlCompose{Dependencies: []Dependency{
					pathDep("pkg-a", pkgA),
					pathDep("pkg-b", pkgB),
				}}
			},
			wantErr: "version conflict",
		},
		{
			name: "circular dependency — error shows exact chain",
			// pkg-circ-a → pkg-circ-b → pkg-circ-a
			setup: func(t *testing.T) *YamlCompose {
				dirA := t.TempDir()
				dirB := t.TempDir()
				writeTestCompose(t, dirA, []Dependency{pathDep("pkg-circ-b", dirB)})
				writeTestCompose(t, dirB, []Dependency{pathDep("pkg-circ-a", dirA)})
				return &YamlCompose{Dependencies: []Dependency{
					pathDep("pkg-circ-a", dirA),
				}}
			},
			wantErr: "pkg-circ-a → pkg-circ-b → pkg-circ-a",
		},
		{
			name: "malformed nested compose — error includes package name",
			setup: func(_ *testing.T) *YamlCompose {
				return &YamlCompose{Dependencies: []Dependency{
					pathDep("pkg-bad", fixturePath("pkg-bad")),
				}}
			},
			wantErr: `"pkg-bad"`,
		},
		{
			name: "deep shared diamond — lib-core downloaded once",
			// pkg-a → lib-common → lib-base → lib-core
			// pkg-b → lib-common (same version, shared)
			// lib-core must appear exactly once.
			setup: func(t *testing.T) *YamlCompose {
				libCore := makePkg(t, "lib-core", nil)
				libBase := makePkg(t, "lib-base", []Dependency{
					pathDep("lib-core", libCore),
				})
				libCommon := makePkg(t, "lib-common", []Dependency{
					pathDep("lib-base", libBase),
				})
				pkgA := makePkg(t, "pkg-a", []Dependency{
					pathDep("lib-common", libCommon),
				})
				pkgB := makePkg(t, "pkg-b", []Dependency{
					pathDep("lib-common", libCommon),
				})
				return &YamlCompose{Dependencies: []Dependency{
					pathDep("pkg-a", pkgA),
					pathDep("pkg-b", pkgB),
				}}
			},
			wantNames: []string{"lib-base", "lib-common", "lib-core", "pkg-a", "pkg-b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yc := tt.setup(t)
			targetDir := t.TempDir()
			dm := newTestDM()

			pkgs, _, err := dm.Download(context.Background(), yc, targetDir)

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil (packages: %v)", tt.wantErr, sortedPkgNames(pkgs))
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantNames != nil {
				got := sortedPkgNames(pkgs)
				want := make([]string, len(tt.wantNames))
				copy(want, tt.wantNames)
				sort.Strings(want)
				if !reflect.DeepEqual(got, want) {
					t.Errorf("packages: got %v, want %v", got, want)
				}
			}
		})
	}
}
