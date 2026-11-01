package main

import (
	"errors"
	"os"
	"path/filepath"
)

func defaultLocalCollection() (string, error) {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".local", "share")
	}
	root := filepath.Join(base, "herdr-postman", "postman", "collections", "default")
	if err := os.MkdirAll(filepath.Join(root, ".resources"), 0700); err != nil {
		return "", err
	}
	file, err := os.OpenFile(filepath.Join(root, ".resources", "definition.yaml"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		return root, nil
	}
	if err != nil {
		return "", err
	}
	_, writeErr := file.WriteString("$kind: collection\nname: Personal requests\n")
	return root, errors.Join(writeErr, file.Close())
}
