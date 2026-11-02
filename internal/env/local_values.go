package env

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LocalValues keeps each environment's current values in a private file per
// environment, so they stay on this machine like Postman Desktop's.
type LocalValues struct {
	Dir string
}

func (l LocalValues) file(files []string) string {
	sum := sha256.Sum256([]byte(strings.Join(files, "\x00")))
	return filepath.Join(l.Dir, fmt.Sprintf("%x.json", sum))
}

func (l LocalValues) Load(files []string) map[string]string {
	if len(files) == 0 {
		return nil
	}
	data, err := os.ReadFile(l.file(files))
	if err != nil {
		return nil
	}
	values := map[string]string{}
	if json.Unmarshal(data, &values) != nil {
		return nil
	}
	return values
}

func (l LocalValues) Save(files []string, vars map[string]string) error {
	if len(files) == 0 {
		return nil
	}
	file := l.file(files)
	if len(vars) == 0 {
		if err := os.Remove(file); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	data, err := json.MarshalIndent(vars, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(l.Dir, 0o700); err != nil {
		return err
	}
	tmp := file + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, file)
}
