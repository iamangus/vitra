package internal

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/google/uuid"
)

var ErrInvalidPath = errors.New("invalid vault path")
var ErrConflict = errors.New("note changed or destination exists")

func safeVaultPath(vaultPath, input string) (string, error) {
	if strings.ContainsAny(input, "\\\x00") || filepath.IsAbs(input) {
		return "", ErrInvalidPath
	}
	rel := filepath.Clean(input)
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", ErrInvalidPath
	}
	base, err := filepath.Abs(vaultPath)
	if err != nil {
		return "", err
	}
	current := base
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if strings.HasPrefix(part, ".") && part != "." {
			return "", ErrInvalidPath
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return "", ErrInvalidPath
		}
	}
	return filepath.Join(base, rel), nil
}

func noteName(path string) string {
	return strings.TrimSuffix(filepath.ToSlash(path), ".md")
}

func noteID(path string) string { return filepath.ToSlash(filepath.Clean(path)) }

func revision(content []byte) string {
	return fmt.Sprintf(`"%x"`, sha256.Sum256(content))
}

func (fsys *FileSystem) rootPath(input string, destructive bool) (*os.Root, string, error) {
	full, err := safeVaultPath(fsys.VaultPath, input)
	if err != nil {
		return nil, "", err
	}
	base, err := filepath.Abs(fsys.VaultPath)
	if err != nil {
		return nil, "", err
	}
	rel, err := filepath.Rel(base, full)
	if err != nil || (destructive && rel == ".") {
		return nil, "", ErrInvalidPath
	}
	root, err := os.OpenRoot(base)
	return root, rel, err
}

func (fsys *FileSystem) ReadVaultFile(path string) ([]byte, error) {
	root, rel, err := fsys.rootPath(path, false)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return root.ReadFile(rel)
}

func (fsys *FileSystem) readDirectory(full string) ([]os.DirEntry, error) {
	rel, err := filepath.Rel(fsys.VaultPath, full)
	if err != nil {
		return nil, err
	}
	root, rel, err := fsys.rootPath(rel, false)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	dir, err := root.Open(rel)
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	entries, err := dir.ReadDir(-1)
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	return entries, err
}

func (fsys *FileSystem) resolveEntry(input string) (*os.Root, string, fs.FileInfo, error) {
	root, rel, err := fsys.rootPath(input, true)
	if err != nil {
		return nil, "", nil, err
	}
	info, err := root.Stat(rel)
	if os.IsNotExist(err) && !strings.HasSuffix(rel, ".md") {
		rel += ".md"
		info, err = root.Stat(rel)
	}
	if err != nil {
		root.Close()
		return nil, "", nil, err
	}
	if !info.IsDir() && !info.Mode().IsRegular() {
		root.Close()
		return nil, "", nil, ErrInvalidPath
	}
	return root, rel, info, nil
}

func atomicNote(root *os.Root, path string, data []byte, exclusive bool) error {
	if err := root.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmp := filepath.Join(filepath.Dir(path), ".vitra-"+uuid.NewString())
	f, err := root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer root.Remove(tmp)
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if exclusive {
		err = root.Link(tmp, path)
	} else {
		err = root.Rename(tmp, path)
	}
	if os.IsExist(err) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		return nil
	}
	dir, err := root.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
