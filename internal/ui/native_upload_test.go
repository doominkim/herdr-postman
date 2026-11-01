package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darrenburns/posting/v3/internal/collection"
	"github.com/darrenburns/posting/v3/internal/model"
	terma "github.com/darrenburns/terma"
)

func TestNativeUploadPreviewResolvesPostmanVariables(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "fixtures"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "fixtures/payload.txt"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	app := New(Config{Store: collection.NewNativeStore(dir), SessionVariables: map[string]string{"upload_dir": "fixtures"}})
	defer app.Close()
	s := app.current()
	if got := app.resolvedUploadPath(s, "{{upload_dir}}/${literal}.txt"); got != "fixtures/${literal}.txt" {
		t.Fatalf("native preview resolves %q", got)
	}
	updates := make(chan func(), 2)
	app.dispatch = func(fn func()) { updates <- fn }
	path := newUploadPath("{{upload_dir}}/payload.txt")
	inspect := func() {
		t.Helper()
		path.inspect(app, s)
		select {
		case fn := <-updates:
			fn()
		case <-time.After(time.Second):
			t.Fatal("upload preview timed out")
		}
	}
	inspect()
	if path.problem.Peek() || !strings.Contains(path.hint.Peek(), "7 B") {
		t.Fatal("native variable path not inspected", path.hint.Peek())
	}
	path.text.SetText("{{script_created}}/payload.txt")
	path.inspect(app, s)
	if path.problem.Peek() || !strings.Contains(path.hint.Peek(), "Postman") {
		t.Fatal("unresolved runtime variable falsely reported missing", path.hint.Peek())
	}
}

func TestNativeUploadVariableCompletionThroughInput(tt *testing.T) {
	app := New(Config{Store: collection.NewNativeStore(tt.TempDir()), SessionVariables: map[string]string{"upload_dir": "fixtures"}})
	defer app.Close()
	app.dispatch = func(func()) {}
	s := app.current()
	s.bodyType.Set(model.BodyFile)
	s.requestTab.Set("body")
	screen := sidebarScreen(tt, app, "req-file-path")
	if !screen.focus.Focused().(terma.PasteHandler).HandlePaste("{{up") {
		tt.Fatal("paste rejected")
	}
	screen.render()
	screen.pressKey(tt, "enter")
	if got := s.upload.text.GetText(); got != "{{upload_dir}}" {
		tt.Fatalf("upload completion inserted %q", got)
	}
}
