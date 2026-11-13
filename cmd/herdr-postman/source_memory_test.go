package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/darrenburns/posting/v3/internal/ui"
)

func TestResponseViewMemory(t *testing.T) {
	file := filepath.Join(t.TempDir(), "state", "response-view.json")
	if readResponseView(file) != nil {
		t.Fatal("a view was restored before any was chosen")
	}
	saved := ui.ResponseView{Tree: true}
	if err := writePrivateJSON(file, saved); err != nil {
		t.Fatal(err)
	}
	if got := readResponseView(file); got == nil || *got != saved {
		t.Fatalf("restored %v, want %v", got, saved)
	}
	os.WriteFile(file, []byte("{"), 0600)
	if readResponseView(file) != nil {
		t.Fatal("a damaged file should fall back to the setting")
	}
}

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
