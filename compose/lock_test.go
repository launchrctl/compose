package compose

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestLookupLock(t *testing.T) {
	const pkgA = "pkg-a"
	const reqRoot = "root"

	validLock := `packages:
    - name: pkg-a
      type: git
      url: https://github.com/example/pkg-a.git
      ref: v1.0.0
      path: .compose/packages/pkg-a/v1.0.0
      required_by:
        - root
    - name: lib-common
      type: git
      url: https://github.com/example/lib-common.git
      ref: v0.5.0
      path: .compose/packages/lib-common/v0.5.0
      required_by:
        - pkg-a
        - pkg-b
`

	t.Run("valid lock file — parsed correctly", func(t *testing.T) {
		fsys := fstest.MapFS{
			LockFile: {Data: []byte(validLock)},
		}
		lock, err := LookupLock(fsys)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(lock.Packages) != 2 {
			t.Fatalf("packages len: got %d, want 2", len(lock.Packages))
		}

		pkg := lock.Packages[0]
		if pkg.Name != pkgA {
			t.Errorf("name: got %q, want %s", pkg.Name, pkgA)
		}
		if pkg.Type != "git" {
			t.Errorf("type: got %q, want git", pkg.Type)
		}
		if pkg.Ref != "v1.0.0" {
			t.Errorf("ref: got %q, want v1.0.0", pkg.Ref)
		}
		if len(pkg.RequiredBy) != 1 || pkg.RequiredBy[0] != reqRoot {
			t.Errorf("required_by: got %v, want [%s]", pkg.RequiredBy, reqRoot)
		}

		lib := lock.Packages[1]
		if len(lib.RequiredBy) != 2 || lib.RequiredBy[0] != pkgA || lib.RequiredBy[1] != "pkg-b" {
			t.Errorf("required_by: got %v, want [pkg-a pkg-b]", lib.RequiredBy)
		}
	})

	t.Run("missing lock file — returns error", func(t *testing.T) {
		fsys := fstest.MapFS{}
		_, err := LookupLock(fsys)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if err != errLockNotExists {
			t.Errorf("error: got %v, want errLockNotExists", err)
		}
	})

	t.Run("malformed lock file — returns parse error", func(t *testing.T) {
		fsys := fstest.MapFS{
			LockFile: {Data: []byte("packages: [\ninvalid yaml")},
		}
		_, err := LookupLock(fsys)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("round-trip: buildLock → writeLock → LookupLock", func(t *testing.T) {
		packages := []*Package{
			{Name: pkgA, Source: Source{Type: "git", URL: "https://github.com/example/pkg-a.git", Ref: "v1.0.0"}},
			{Name: "lib-common", Source: Source{Type: "git", URL: "https://github.com/example/lib-common.git", Ref: "v0.5.0"}},
		}
		requiredBy := map[string][]string{
			pkgA:         {reqRoot},
			"lib-common": {pkgA, "pkg-b"},
		}

		platformDir := t.TempDir()
		packagesDir := filepath.Join(platformDir, ".compose", "packages")
		targetDir := t.TempDir()

		lock, err := buildLock(packages, platformDir, packagesDir, requiredBy)
		if err != nil {
			t.Fatalf("buildLock: %v", err)
		}
		if err = writeLock(lock, targetDir); err != nil {
			t.Fatalf("writeLock: %v", err)
		}

		fsys := os.DirFS(targetDir)
		got, err := LookupLock(fsys)
		if err != nil {
			t.Fatalf("LookupLock: %v", err)
		}
		if len(got.Packages) != 2 {
			t.Fatalf("packages len: got %d, want 2", len(got.Packages))
		}
		if got.Packages[0].Name != pkgA {
			t.Errorf("packages[0].Name: got %q, want %s", got.Packages[0].Name, pkgA)
		}
		if got.Packages[1].Name != "lib-common" {
			t.Errorf("packages[1].Name: got %q, want lib-common", got.Packages[1].Name)
		}
		if len(got.Packages[1].RequiredBy) != 2 {
			t.Errorf("packages[1].RequiredBy: got %v, want [pkg-a pkg-b]", got.Packages[1].RequiredBy)
		}
	})
}
