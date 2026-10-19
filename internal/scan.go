package internal

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/iamangus/vitra/internal/okf"
)

func (fs *FileSystem) recordIndexError(err error) {
	fs.statusMu.Lock()
	defer fs.statusMu.Unlock()
	fs.indexError = err.Error()
	fmt.Fprintf(os.Stderr, "index: %v\n", err)
}

func excludedPath(rel, skills string, reserved bool) bool {
	rel = filepath.ToSlash(rel)
	for _, part := range strings.Split(rel, "/") {
		if strings.HasPrefix(part, ".") && part != "." {
			return true
		}
	}
	return (skills != "" && (rel == skills || strings.HasPrefix(rel, skills+"/"))) || (reserved && okf.IsReservedFilename(path.Base(rel)))
}

func walkNotes(vault, scope, skills string, reserved bool, visit func(string, []byte) error) error {
	if _, err := safeVaultPath(vault, scope); err != nil {
		return err
	}
	scope = filepath.Clean(scope)
	root, err := os.OpenRoot(vault)
	if err != nil {
		return err
	}
	defer root.Close()
	if scope == "" {
		scope = "."
	}
	return fs.WalkDir(root.FS(), filepath.ToSlash(scope), func(rel string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if excludedPath(rel, skills, reserved) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() || !d.Type().IsRegular() || !strings.HasSuffix(rel, ".md") {
			return nil
		}
		data, err := root.ReadFile(rel)
		if err != nil {
			return err
		}
		return visit(noteName(rel), data)
	})
}

func (fs *FileSystem) indexSubtree(rel string) {
	scope := rel
	if _, err := os.Stat(filepath.Join(fs.VaultPath, rel+".md")); err == nil {
		scope += ".md"
	}
	if err := walkNotes(fs.VaultPath, scope, fs.skillsDirRel, true, func(path string, data []byte) error {
		fs.autoIndex(path, string(data))
		return nil
	}); err != nil {
		fs.recordIndexError(err)
	}
}
