package compose

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/stevenle/topsort"
	"github.com/yalue/merged_fs"
)

const (
	// DependencyRoot is dependecies graph main node
	DependencyRoot = "root"
)

var excludedFolders = map[string]bool{".idea": true, ".plasma": true, ".git": true}
var excludedFiles = map[string]bool{composeFile: true, composeLock: true}

// Builder struct, provides methods to merge packages into build
type Builder struct {
	platformDir string
	targetDir   string
	sourcedir   string
	graph       topsort.Graph
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
	var filesystems = make([]fs.FS, 0, len(items))

	filesystems = append(filesystems, os.DirFS(b.platformDir))
	for i := 0; i < len(items); i++ {
		var fs fs.FS
		if items[i] != DependencyRoot {
			fs = os.DirFS(filepath.Join(b.sourcedir, items[i]))
			filesystems = append(filesystems, fs)
		}
	}

	merged := merged_fs.MergeMultiple(filesystems...)
	paths := []string{}
	dirpaths := []string{}

	err = fs.WalkDir(merged, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		root := rgxPathRoot.FindString(path)
		if excludedFolders[root] {
			return nil
		}

		if !d.IsDir() {
			filename := filepath.Base(path)
			if excludedFiles[filename] {
				return nil
			}
			paths = append(paths, path)
		} else {

			dirpaths = append(dirpaths, path)
		}

		return nil
	})

	if err != nil {
		return err
	}

	// prepare directories
	for _, d := range dirpaths {
		fmt.Println(fmt.Sprintf("create dir " + d))
		err = os.MkdirAll(filepath.Join(b.targetDir, d), os.ModePerm)
		if err != nil {
			fmt.Println(err.Error())
		}
	}

	// try to copy files into build folder
	fmt.Println("copying files")
	for _, p := range paths {
		err := copyFile(merged, b.targetDir, p)
		if err != nil {
			fmt.Println(err.Error())
		}
	}

	return nil
}

// Copies file from FS to target dir and path
func copyFile(filesys fs.FS, target, path string) error {
	sourceFile, err := filesys.Open(path)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	newFile, err := os.Create(filepath.Clean(filepath.Join(target, path)))
	if err != nil {
		return err
	}

	if _, err := io.Copy(newFile, sourceFile); err != nil {
		return err
	}

	return newFile.Close()
}
