package env

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSessionMemory(t *testing.T) {
	m := SessionMemory{File: filepath.Join(t.TempDir(), "state", "session-variables.json")}
	if got := m.Recall("/c"); len(got) != 0 {
		t.Fatalf("nothing remembered yet: %v", got)
	}
	if err := m.Remember("/c", map[string]string{"TOKEN": "abc", "ID": "7"}); err != nil {
		t.Fatal(err)
	}
	if err := m.Remember("/other", map[string]string{"TOKEN": "xyz"}); err != nil {
		t.Fatal(err)
	}
	if got, want := m.Recall("/c"), map[string]string{"TOKEN": "abc", "ID": "7"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Recall = %v, want %v", got, want)
	}
	info, err := os.Stat(m.File)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("session variables hold tokens, so only the user may read them: mode %o", mode)
	}
	if err := m.Remember("/c", map[string]string{}); err != nil {
		t.Fatal(err)
	}
	if got := m.Recall("/c"); len(got) != 0 {
		t.Fatalf("no variables forgets the collection: %v", got)
	}
	if got := m.Recall("/other"); got["TOKEN"] != "xyz" {
		t.Fatalf("forgetting one collection should keep the other: %v", got)
	}
}
