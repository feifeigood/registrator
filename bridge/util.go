package bridge

import (
	"os"
	"path/filepath"
)

func Contains(x []string, y string) bool {
	for _, v := range x {
		if v == y {
			return true
		}
	}
	return false
}

func IsDirectory(path string) (bool, error) {
	f, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	return f.IsDir(), nil
}

func RecursiveFilesLookup(root string, pattern string) ([]string, error) {
	return recursiveLookup(root, pattern, false)
}

func RecursiveDirsLookup(root string, pattern string) ([]string, error) {
	return recursiveLookup(root, pattern, true)
}

func recursiveLookup(root string, pattern string, dirsLookup bool) ([]string, error) {
	var result []string
	isDir, err := IsDirectory(root)
	if err != nil {
		return nil, err
	}

	if isDir {
		err = filepath.Walk(root, func(root string, f os.FileInfo, err error) error {
			match, err := filepath.Match(pattern, f.Name())
			if err != nil {
				return err
			}
			if match {
				isDir, err := IsDirectory(root)
				if err != nil {
					return err
				}
				if isDir && dirsLookup {
					result = append(result, root)
				} else if !isDir && !dirsLookup {
					result = append(result, root)
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	} else {
		if !dirsLookup {
			result = append(result, root)
		}
	}

	return result, nil
}
