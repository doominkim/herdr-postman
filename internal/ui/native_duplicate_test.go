package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darrenburns/posting/v3/internal/collection"
	"github.com/darrenburns/posting/v3/internal/model"
)

func TestNativeDuplicatePreservesGUIDocumentAndDraft(t *testing.T) {
	dir := t.TempDir()
	source := "$kind: http-request\nname: Original\nmethod: GET\nurl: https://example.test/original\nauth:\n  type: apikey\n  apikey:\n    key: X-Key\n    value: '{{token}}'\nscripts:\n  - type: afterResponse\n    code: pm.test('fixture', () => {});\n    language: text/javascript\n"
	if err := os.WriteFile(filepath.Join(dir, "get.request.yaml"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	store := collection.NewNativeStore(dir)
	coll, problems := store.Load()
	if len(problems) > 0 {
		t.Fatal(problems)
	}
	app := New(Config{Collection: coll, Store: store})
	defer app.Close()
	req := coll.Requests[0]
	req.URL = "https://example.test/draft"
	app.duplicateRequests([]model.Request{req})
	dup := app.current().Snapshot()
	if dup.File != "get-copy.request.yaml" {
		t.Fatalf("duplicate file = %q", dup.File)
	}
	data, err := os.ReadFile(filepath.Join(dir, dup.File))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"name: Original (copy)", "https://example.test/draft", "type: apikey", "{{token}}", "pm.test('fixture'"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("duplicate lost %q", want)
		}
	}
	original, _ := os.ReadFile(filepath.Join(dir, req.File))
	if string(original) != source {
		t.Fatal("source modified")
	}
	dup.URL = "https://example.test/edited-copy"
	if err := store.Save(dup); err != nil {
		t.Fatalf("copy not independently editable: %v", err)
	}
	app.duplicateRequests([]model.Request{req})
	if app.current().file.Peek() != "get-copy-2.request.yaml" {
		t.Fatal("collision did not select next copy name")
	}
}
