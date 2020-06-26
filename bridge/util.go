package bridge

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strconv"
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

const shortLen = 12

func TruncateID(id string) string {
	trimTo := shortLen
	if len(id) < shortLen {
		trimTo = len(id)
	}
	return id[:trimTo]
}

func GenerateRandomID() string {
	b := make([]byte, 32)
	var r io.Reader = rand.Reader
	for {
		if _, err := io.ReadFull(r, b); err != nil {
			panic(err)
		}
		id := hex.EncodeToString(b)

		if _, err := strconv.ParseInt(TruncateID(id), 10, 64); err == nil {
			continue
		}

		// return short id
		return id[:12]
	}
}
