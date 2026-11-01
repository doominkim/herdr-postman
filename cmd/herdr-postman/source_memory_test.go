package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSourceMemoryRoundTripAndPrivateFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "state", "source.json")
	saved := sourceSelection{Mode: "cloud", Workspace: "w1", LocalRoot: t.TempDir()}
	if err := saveSourceSelection(file, saved); err != nil {
		t.Fatal(err)
	}
	got := readSourceSelection(file)
	if got != saved {
		t.Fatal("last source not restored")
	}
	info, _ := os.Stat(file)
	if info.Mode().Perm() != 0600 {
		t.Fatal("source state is not private")
	}
	for _, body := range []string{`{`, `{"mode":"cloud"}`, `{"mode":"unknown","localRoot":"/tmp"}`} {
		os.WriteFile(file, []byte(body), 0600)
		if got := readSourceSelection(file); got.Mode != "" {
			t.Fatal("invalid source state accepted")
		}
	}
}

func TestSidebarMemoryIsSeparateForEachSource(t *testing.T) {
	dir := t.TempDir()
	local := sourceSelection{Mode: "local", LocalRoot: "/fixture/local"}
	cloud := sourceSelection{Mode: "cloud", Workspace: "w1", LocalRoot: local.LocalRoot}
	if err := saveSidebarState(dir, cloud, []string{"collection-a", "collection-a/folder"}); err != nil {
		t.Fatal(err)
	}
	if got := readSidebarState(dir, local); len(got) != 0 {
		t.Fatal("Cloud state leaked into Local")
	}
	other := cloud
	other.Workspace = "w2"
	if got := readSidebarState(dir, other); len(got) != 0 {
		t.Fatal("Cloud state leaked into another workspace")
	}
	if got := readSidebarState(dir, cloud); len(got) != 2 || got[0] != "collection-a" {
		t.Fatal("collapsed folders not restored")
	}
	if err := saveSidebarState(dir, cloud, []string{}); err != nil {
		t.Fatal(err)
	}
	if got := readSidebarState(dir, cloud); len(got) != 0 {
		t.Fatal("reopening every folder did not clear collapsed state")
	}
}
