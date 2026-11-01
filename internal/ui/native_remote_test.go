package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darrenburns/posting/v3/internal/collection"
	"github.com/darrenburns/posting/v3/internal/remote"
)

func TestNativeRemoteOpenShowAndSavePreserveDocument(t *testing.T) {
	dir := t.TempDir()
	source := "$kind: http-request\nname: Remote fixture\nmethod: GET\nurl: https://example.test\nscripts:\n  - type: afterResponse\n    language: text/javascript\n    code: pm.test('fixture', () => {});\n"
	if err := os.WriteFile(filepath.Join(dir, "get.request.yaml"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	store := collection.NewNativeStore(dir)
	coll, problems := store.Load()
	if len(problems) > 0 {
		t.Fatal(problems)
	}
	app := New(Config{Store: store, Collection: coll})
	defer app.Close()
	h := &remoteHarness{tt: t, app: app, queue: make(chan func(), 64)}
	app.dispatch = func(fn func()) { h.queue <- fn }
	app.markRunning()
	result, err := h.call(remote.Request{Command: remote.CommandOpen, Ref: "get"})
	if err != nil {
		t.Fatal(err)
	}
	shown := result.(remote.Shown)
	if shown.Tab.File != "get.request.yaml" || !strings.Contains(shown.YAML, "$kind: http-request") || !strings.Contains(shown.YAML, "pm.test") {
		t.Fatal("remote open did not preserve native format")
	}
	result, err = h.call(remote.Request{Command: remote.CommandSave, File: "get-copy", Name: "Remote copy"})
	if err != nil {
		t.Fatal(err)
	}
	shown = result.(remote.Shown)
	if shown.Tab.File != "get-copy.request.yaml" || !strings.Contains(shown.YAML, "pm.test") {
		t.Fatal("save as lost native metadata")
	}
	copied, err := os.ReadFile(filepath.Join(dir, "get-copy.request.yaml"))
	if err != nil || !strings.Contains(string(copied), "pm.test") {
		t.Fatal("native copy not saved", err)
	}
	before, _ := os.ReadFile(filepath.Join(dir, "get.request.yaml"))
	if string(before) != source {
		t.Fatal("remote copy changed original")
	}
	if _, err := h.call(remote.Request{Command: remote.CommandOpen, YAML: source}); err == nil {
		t.Fatal("native YAML accepted by legacy importer")
	}
	if app.current().file.Peek() != "get-copy.request.yaml" {
		t.Fatal("failed import changed active tab")
	}
}
