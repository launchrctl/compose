package compose

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/stevenle/topsort"
)

const (
	// DependencyRoot is dependecies graph main node
	DependencyRoot = "root"
)

var excludedFolders = map[string]bool{".idea": true, ".compose": true}
var excludedFiles = map[string]bool{composeFile: true, composeLock: true}

// Builder struct, provides methods to merge packages into build
type Builder struct {
	platformDir string
	targetDir   string
	sourcedir   string
	graph       topsort.Graph
}

type fsEntry struct {
	Prefix   string
	Path     string
	Entry    fs.FileInfo
	Excluded bool
}

func createBuilder(platformDir, targetDir, sourcedir string, graph topsort.Graph) *Builder {
	return &Builder{platformDir, targetDir, sourcedir, graph}
}

func (b *Builder) build() error {
	err := EnsureDirExists(b.targetDir)
	if err != nil {
		return err
	}

	items, _ := b.graph.TopSort(DependencyRoot)
	baseFs := os.DirFS(b.platformDir)

	entriesMap := make(map[string]*fsEntry)
	var entriesTree []*fsEntry

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

		finfo, _ := d.Info()
		entry := &fsEntry{Prefix: b.platformDir, Path: path, Entry: finfo, Excluded: false}
		entriesTree = append(entriesTree, entry)
		entriesMap[path] = entry
		return nil
	})

	if err != nil {
		return err
	}

	for i := 0; i < len(items); i++ {
		if items[i] != DependencyRoot {
			// Place for merge strategies

			pkgPath := filepath.Join(b.sourcedir, items[i])
			packageFs := os.DirFS(pkgPath)
			err = fs.WalkDir(packageFs, ".", func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}

				finfo, _ := d.Info()
				entry := &fsEntry{Prefix: pkgPath, Path: path, Entry: finfo, Excluded: false}

				if _, ok := entriesMap[path]; !ok {
					entriesTree = append(entriesTree, entry)
					entriesMap[path] = entry
				}

				return nil
			})

			if err != nil {
				return err
			}
		}
	}

	for _, treeItem := range entriesTree {
		sourcePath := filepath.Join(treeItem.Prefix, treeItem.Path)
		destPath := filepath.Join(b.targetDir, treeItem.Path)
		isSymlink := false

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
			if err := copy(sourcePath, destPath); err != nil {
				return err
			}
		}

		if !isSymlink {
			if err := os.Chmod(destPath, treeItem.Entry.Mode()); err != nil {
				return err
			}
		}
	}

	return nil
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

func copy(src, dst string) error {
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
