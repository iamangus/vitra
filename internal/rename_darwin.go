package internal

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func renameNoReplace(root *os.Root, old, new string) error {
	oldDir, err := root.Open(filepath.Dir(old))
	if err != nil {
		return err
	}
	defer oldDir.Close()
	newDir, err := root.Open(filepath.Dir(new))
	if err != nil {
		return err
	}
	defer newDir.Close()
	err = unix.RenameatxNp(int(oldDir.Fd()), filepath.Base(old), int(newDir.Fd()), filepath.Base(new), unix.RENAME_EXCL)
	if os.IsExist(err) {
		return ErrConflict
	}
	return err
}
