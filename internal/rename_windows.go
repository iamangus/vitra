package internal

import "os"

func renameNoReplace(root *os.Root, old, new string) error {
	info, err := root.Stat(old)
	if err != nil {
		return err
	}
	if info.IsDir() {
		err = root.Rename(old, new)
		if os.IsExist(err) {
			return ErrConflict
		}
		return err
	}
	if err := root.Link(old, new); err != nil {
		if os.IsExist(err) {
			return ErrConflict
		}
		return err
	}
	return root.Remove(old)
}
