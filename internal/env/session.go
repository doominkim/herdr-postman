package env

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// SessionMemory keeps each collection's session variables, captured from
// responses or set by hand, so the next launch starts with them. It's one
// JSON file keyed by collection directory, readable only by the user since
// captures are often tokens.
type SessionMemory struct {
	File string
}

func (m SessionMemory) read() (map[string]map[string]string, error) {
	data, err := os.ReadFile(m.File)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	saved := map[string]map[string]string{}
	if err := json.Unmarshal(data, &saved); err != nil {
		return nil, err
	}
	return saved, nil
}

// Recall is the session variables remembered for a collection, or nil if
// there are none.
func (m SessionMemory) Recall(collection string) map[string]string {
	saved, err := m.read()
	if err != nil {
		return nil
	}
	return saved[collection]
}

// Remember records a collection's session variables. No variables forgets
// the collection.
func (m SessionMemory) Remember(collection string, vars map[string]string) error {
	saved, err := m.read()
	if err != nil {
		saved = map[string]map[string]string{}
	}
	if len(vars) == 0 {
		delete(saved, collection)
	} else {
		saved[collection] = vars
	}
	data, err := json.MarshalIndent(saved, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(m.File), 0o755); err != nil {
		return err
	}
	tmp := m.File + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, m.File)
}
