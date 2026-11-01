package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Only source identities are persisted, never credentials or request documents.
type sourceSelection struct {
	Mode      string `json:"mode"`
	Workspace string `json:"workspace,omitempty"`
	LocalRoot string `json:"localRoot"`
}

func (s sourceSelection) valid() bool {
	return filepath.IsAbs(s.LocalRoot) && ((s.Mode == "local" && s.Workspace == "") || (s.Mode == "cloud" && s.Workspace != ""))
}
func readSourceSelection(file string) sourceSelection {
	var selection sourceSelection
	data, err := os.ReadFile(file)
	if err != nil || json.Unmarshal(data, &selection) != nil || !selection.valid() {
		return sourceSelection{}
	}
	return selection
}
func saveSourceSelection(file string, selection sourceSelection) error {
	if !selection.valid() {
		return fmt.Errorf("invalid source selection")
	}
	return writePrivateJSON(file, selection)
}

func writePrivateJSON(file string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(file), ".source-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), file)
}

func sidebarStateFile(dir string, source sourceSelection) string {
	identity := source.LocalRoot
	if source.Mode == "cloud" {
		identity = source.Workspace
	}
	return filepath.Join(dir, fmt.Sprintf("%x.json", sha256.Sum256([]byte(source.Mode+"\x00"+identity))))
}
func readSidebarState(dir string, source sourceSelection) []string {
	var collapsed []string
	data, err := os.ReadFile(sidebarStateFile(dir, source))
	if err != nil || json.Unmarshal(data, &collapsed) != nil {
		return nil
	}
	return collapsed
}
func saveSidebarState(dir string, source sourceSelection, collapsed []string) error {
	return writePrivateJSON(sidebarStateFile(dir, source), collapsed)
}
