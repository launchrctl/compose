package compose

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func testGitCommand(t *testing.T, dir string, args ...string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	return cmd
}

func TestPlainOpenWorktree(t *testing.T) {
	// Create a temporary git repo.
	repoDir := t.TempDir()

	repo, err := git.PlainInit(repoDir, false)
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	// Create a file and commit.
	testFile := "hello.txt"
	if err := os.WriteFile(filepath.Join(repoDir, testFile), []byte("hello"), 0600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("failed to get worktree: %v", err)
	}
	if _, err := wt.Add(testFile); err != nil {
		t.Fatalf("failed to add file: %v", err)
	}
	commitHash, err := wt.Commit("initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "test",
			Email: "test@test.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		t.Fatalf("failed to commit: %v", err)
	}

	// Create a worktree using git CLI.
	worktreeDir := t.TempDir()
	cmd := testGitCommand(t, repoDir, "worktree", "add", worktreeDir, "-b", "wt-branch", commitHash.String())
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to create worktree: %v\n%s", err, out)
	}

	// Verify .git is a file.
	fi, err := os.Lstat(filepath.Join(worktreeDir, ".git"))
	if err != nil {
		t.Fatalf("failed to stat .git: %v", err)
	}
	if fi.IsDir() {
		t.Fatal("expected .git to be a file in worktree, got directory")
	}

	// PlainOpenWithOptions with EnableDotGitCommonDir should work.
	r, err := git.PlainOpenWithOptions(worktreeDir, &git.PlainOpenOptions{EnableDotGitCommonDir: true})
	if err != nil {
		t.Fatalf("PlainOpenWithOptions failed on worktree: %v", err)
	}

	// Should be able to resolve HEAD.
	head, err := r.Head()
	if err != nil {
		t.Fatalf("failed to get HEAD from worktree: %v", err)
	}

	// Should be able to access objects (this is what fails without EnableDotGitCommonDir).
	commit, err := r.CommitObject(head.Hash())
	if err != nil {
		t.Fatalf("failed to get commit object from worktree: %v", err)
	}

	tree, err := commit.Tree()
	if err != nil {
		t.Fatalf("failed to get tree from worktree: %v", err)
	}

	// Verify we can iterate files.
	var files []string
	err = tree.Files().ForEach(func(f *object.File) error {
		files = append(files, f.Name)
		return nil
	})
	if err != nil {
		t.Fatalf("failed to iterate files: %v", err)
	}

	if len(files) != 1 || files[0] != testFile {
		t.Errorf("expected [%s], got %v", testFile, files)
	}
}
