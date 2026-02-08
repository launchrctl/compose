package compose

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func TestGetVersionedMap(t *testing.T) {
	// Create a temporary git repo with some files.
	repoDir := t.TempDir()

	repo, err := git.PlainInit(repoDir, false)
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	// Create test files.
	files := []string{"file1.txt", "dir/file2.txt"}
	for _, f := range files {
		p := filepath.Join(repoDir, f)
		if err := os.MkdirAll(filepath.Dir(p), 0750); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
		if err := os.WriteFile(p, []byte("content"), 0600); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}
	}

	// Stage and commit.
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("failed to get worktree: %v", err)
	}
	for _, f := range files {
		if _, err := wt.Add(f); err != nil {
			t.Fatalf("failed to add file: %v", err)
		}
	}
	_, err = wt.Commit("initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "test",
			Email: "test@test.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		t.Fatalf("failed to commit: %v", err)
	}

	versionedMap, err := getVersionedMap(repoDir)
	if err != nil {
		t.Fatalf("getVersionedMap failed: %v", err)
	}

	// Verify files are in the map.
	for _, f := range files {
		if !versionedMap[f] {
			t.Errorf("expected %q in versioned map", f)
		}
	}

	// Verify parent dir is in the map.
	if !versionedMap["dir"] {
		t.Error("expected 'dir' in versioned map")
	}
}

func TestGetVersionedMapWorktree(t *testing.T) {
	// Create a temporary git repo.
	repoDir := t.TempDir()

	repo, err := git.PlainInit(repoDir, false)
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	// Create a file and commit on main.
	testFile := "file.txt"
	if err := os.WriteFile(filepath.Join(repoDir, testFile), []byte("main content"), 0600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("failed to get worktree: %v", err)
	}
	if _, err := wt.Add(testFile); err != nil {
		t.Fatalf("failed to add file: %v", err)
	}
	_, err = wt.Commit("initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "test",
			Email: "test@test.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		t.Fatalf("failed to commit: %v", err)
	}

	// Create a branch for the worktree.
	headRef, err := repo.Head()
	if err != nil {
		t.Fatalf("failed to get HEAD: %v", err)
	}

	// Use git command to create the worktree, as go-git doesn't have
	// a built-in worktree add command.
	worktreeDir := t.TempDir()
	cmd := testGitCommand(t, repoDir, "worktree", "add", worktreeDir, "-b", "test-branch", headRef.Hash().String())
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to create worktree: %v\n%s", err, out)
	}

	// Verify the worktree has a .git file (not directory).
	gitPath := filepath.Join(worktreeDir, ".git")
	fi, err := os.Lstat(gitPath)
	if err != nil {
		t.Fatalf("failed to stat .git: %v", err)
	}
	if fi.IsDir() {
		t.Fatal("expected .git to be a file in worktree, got directory")
	}

	// This is the key test: getVersionedMap must work on a worktree.
	versionedMap, err := getVersionedMap(worktreeDir)
	if err != nil {
		t.Fatalf("getVersionedMap failed on worktree: %v", err)
	}

	if !versionedMap[testFile] {
		t.Errorf("expected %q in versioned map from worktree", testFile)
	}
}
