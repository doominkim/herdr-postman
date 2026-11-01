package ui

import (
	"context"
	"fmt"
	"github.com/darrenburns/posting/v3/internal/client"
	"github.com/darrenburns/posting/v3/internal/env"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darrenburns/posting/v3/internal/collection"
	"github.com/darrenburns/posting/v3/internal/model"
	terma "github.com/darrenburns/terma"
)

func TestNativeGUIRefreshUpdatesCleanEditorRevision(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "get.request.yaml")
	initial := "$kind: http-request\nmethod: GET\nurl: https://example.test/old\n"
	if err := os.WriteFile(file, []byte(initial), 0600); err != nil {
		t.Fatal(err)
	}
	store := collection.NewNativeStore(dir)
	coll, _ := store.Load()
	app := New(Config{Collection: coll, Store: store, UserHost: "user@host"})
	defer app.Close()
	app.openRequest(coll.Requests[0])
	if err := os.WriteFile(file, []byte(initial+"examples: './.resources/gui.resources/examples/'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	coll, _ = store.Load()
	app.replaceCollection(coll)
	app.current().url.SetText("https://example.test/new")
	app.current().urlEdited()
	app.saveRequest()
	if app.current().dirty.Peek() {
		t.Fatal("clean editor was not refreshed for GUI-only metadata change")
	}
	data, _ := os.ReadFile(file)
	if !strings.Contains(string(data), "gui.resources/examples/") {
		t.Fatal("GUI change lost")
	}
}

func TestNativeCookieCheckboxSavesAndReopens(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "cookies.request.yaml"), []byte("$kind: http-request\nmethod: GET\nurl: https://example.test\nsettings:\n  disableCookies: true\n  maxRedirects: 7\n"), 0600); err != nil {
		t.Fatal(err)
	}
	store := collection.NewNativeStore(dir)
	coll, problems := store.Load()
	if len(problems) != 0 {
		t.Fatal(problems)
	}
	app := New(Config{Collection: coll, Store: store})
	defer app.Close()
	app.openRequest(coll.Requests[0])
	s := app.current()
	if s.cookies.IsChecked() {
		t.Fatal("GUI disabled cookie option not shown")
	}
	s.cookies.SetChecked(true)
	s.touch()
	app.saveRequest()
	if s.dirty.Peek() {
		t.Fatal("cookie option could not be saved")
	}
	coll, problems = store.Load()
	if len(problems) != 0 || !coll.Requests[0].Options.AttachCookies {
		t.Fatal("cookie option not persisted", problems)
	}
	s.Load(coll.Requests[0])
	if !s.cookies.IsChecked() {
		t.Fatal("saved cookie option not restored")
	}
	data, err := os.ReadFile(filepath.Join(dir, "cookies.request.yaml"))
	if err != nil || !strings.Contains(string(data), "maxRedirects: 7") {
		t.Fatal("GUI advanced setting lost")
	}
}

func TestEditorCreatesPostmanNativeRequest(t *testing.T) {
	dir := t.TempDir()
	store := collection.NewNativeStore(dir)
	coll, _ := store.Load()
	app := New(Config{Collection: coll, Store: store, UserHost: "user@host"})
	defer app.Close()
	app.newTab()
	app.current().url.SetText("https://example.test/users")
	app.current().urlEdited()
	app.saveRequest()
	app.save.name.SetText("Get users")
	app.submitSave()
	if app.current().file.Peek() != "get-users.request.yaml" {
		t.Fatalf("native save failed: %s (%s)", app.current().file.Peek(), app.save.err.Peek())
	}
	if _, err := os.Stat(filepath.Join(dir, "get-users.request.yaml")); err != nil {
		t.Fatal(err)
	}
}

func TestEditorSavesNativeRequestWithoutDroppingGUIFields(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "get.request.yaml")
	if err := os.WriteFile(file, []byte("$kind: http-request\nmethod: GET\nurl: https://example.test/old\nexamples: './.resources/get.resources/examples/'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	store := collection.NewNativeStore(dir)
	coll, problems := store.Load()
	if len(problems) > 0 {
		t.Fatal(problems)
	}
	app := New(Config{Collection: coll, Store: store, UserHost: "user@host"})
	defer app.Close()
	app.openRequest(coll.Requests[0])
	s := app.current()
	s.url.SetText("https://example.test/new")
	s.urlEdited()
	app.saveRequest()
	if s.dirty.Peek() {
		t.Fatal("editor could not save native request")
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "https://example.test/new") || !strings.Contains(string(data), "examples/") {
		t.Fatalf("native document not preserved: %s", data)
	}
}

func TestNativeWatcherReloadsCleanTabAndPreservesDirtyDraft(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "get.request.yaml")
	write := func(url string) {
		t.Helper()
		if err := os.WriteFile(file, []byte("$kind: http-request\nmethod: GET\nurl: "+url+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("https://example.test/original")
	store := collection.NewNativeStore(dir)
	coll, _ := store.Load()
	app := New(Config{Collection: coll, Store: store})
	defer app.Close()
	app.openRequest(coll.Requests[0])
	updates := make(chan func(), 10)
	app.dispatch = func(fn func()) { updates <- fn }
	app.watchCollection(store, 5*time.Millisecond)
	receive := func() {
		t.Helper()
		select {
		case fn := <-updates:
			fn()
		case <-time.After(2 * time.Second):
			t.Fatal("watcher did not dispatch reload")
		}
	}
	if err := os.WriteFile(file, []byte("$kind: [invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	receive()
	if app.current().url.GetText() != "https://example.test/original" || len(app.collection.Peek().Requests) != 1 {
		t.Fatal("invalid GUI file discarded valid editor state")
	}
	write("https://example.test/gui")
	receive()
	if got := app.current().url.GetText(); got != "https://example.test/gui" {
		t.Fatalf("GUI change not applied: %s", got)
	}
	app.current().url.SetText("https://example.test/draft")
	app.current().urlEdited()
	write("https://example.test/gui-again")
	receive()
	if got := app.current().url.GetText(); got != "https://example.test/draft" {
		t.Fatal("dirty draft replaced")
	}
	app.saveRequest()
	if !app.current().dirty.Peek() {
		t.Fatal("conflicting draft incorrectly saved")
	}
	data, _ := os.ReadFile(file)
	if !strings.Contains(string(data), "gui-again") {
		t.Fatal("GUI change overwritten")
	}
	if err := app.Close(); err != nil {
		t.Fatal(err)
	}
	write("https://example.test/closed")
	select {
	case <-updates:
		t.Fatal("watcher dispatched after close")
	case <-time.After(30 * time.Millisecond):
	}
}

func TestNativeExportUsesPostmanFormatWithoutSaving(t *testing.T) {
	for _, saved := range []bool{false, true} {
		t.Run(fmt.Sprint(saved), func(t *testing.T) {
			dir := t.TempDir()
			store := collection.NewNativeStore(dir)
			original := "$kind: http-request\nmethod: GET\nurl: https://example.test/original\nauth:\n  type: bearer\n  credentials:\n    - key: token\n      value: '{{token}}'\nscripts:\n  - type: afterResponse\n    code: pm.test('preserved', () => {});\n"
			file := filepath.Join(dir, "get.request.yaml")
			if saved {
				if err := os.WriteFile(file, []byte(original), 0600); err != nil {
					t.Fatal(err)
				}
			}
			coll, _ := store.Load()
			app := New(Config{Collection: coll, Store: store})
			defer app.Close()
			if saved {
				app.openRequest(coll.Requests[0])
			}
			app.current().url.SetText("https://example.test/draft")
			app.current().urlEdited()
			app.exportYAML()
			exported := app.curlDialog.text.GetText()
			if !strings.Contains(exported, "$kind: http-request") || !strings.Contains(exported, "https://example.test/draft") {
				t.Fatalf("not a native draft export: %s", exported)
			}
			if saved {
				if !strings.Contains(exported, "{{token}}") || !strings.Contains(exported, "preserved") {
					t.Fatal("GUI fields dropped")
				}
				current, _ := os.ReadFile(file)
				if string(current) != original {
					t.Fatal("export saved draft")
				}
			} else {
				files, _ := os.ReadDir(dir)
				if len(files) != 0 {
					t.Fatal("export created files")
				}
			}
		})
	}
}

func TestNativeAuthEditorShowsInheritedAndManagedState(t *testing.T) {
	for _, tc := range []struct{ auth, label string }{{"", "Inherit from parent"}, {"auth:\n  type: oauth2\n  credentials: []\n", "Managed in Postman"}} {
		t.Run(tc.label, func(t *testing.T) {
			dir := t.TempDir()
			os.WriteFile(filepath.Join(dir, "get.request.yaml"), []byte("$kind: http-request\nmethod: GET\nurl: https://example.test\n"+tc.auth), 0600)
			store := collection.NewNativeStore(dir)
			coll, _ := store.Load()
			app := New(Config{Collection: coll, Store: store})
			defer app.Close()
			app.openRequest(coll.Requests[0])
			s := app.current()
			buf := terma.RenderToBuffer(authEditor{session: s}, 120, 20)
			svg := terma.BufferToSVG(buf, 120, 20, terma.DefaultSVGOptions())
			for _, word := range strings.Fields(tc.label) {
				if !strings.Contains(svg, ">"+word+"</text>") {
					t.Fatalf("native auth label missing %s", word)
				}
			}
			s.authType.Set(model.AuthBearer)
			s.token.SetText("{{api_token}}")
			s.touch()
			app.saveRequest()
			if s.dirty.Peek() {
				t.Fatal("native bearer edit failed")
			}
			reloaded, _ := store.Load()
			if reloaded.Requests[0].Auth.Token != "{{api_token}}" {
				t.Fatal("edited token reference lost")
			}
		})
	}
}

func TestNativeFormEditorSavesEnabledAndDuplicateRows(t *testing.T) {
	dir := t.TempDir()
	store := collection.NewNativeStore(dir)
	req := model.NewRequest()
	req.File = "form.request.yaml"
	req.Method = model.MethodPost
	req.URL = "https://example.test"
	req.Body = model.Body{Type: model.BodyForm, Form: []model.KeyValue{{Name: "tag", Value: "first", Enabled: true}, {Name: "tag", Value: "disabled", Enabled: false}}}
	if err := store.Create(req); err != nil {
		t.Fatal(err)
	}
	coll, _ := store.Load()
	app := New(Config{Collection: coll, Store: store})
	defer app.Close()
	app.openRequest(coll.Requests[0])
	s := app.current()
	rows := s.form.Values()
	rows[0].Value = "edited"
	s.form.Load(rows)
	s.touch()
	app.saveRequest()
	if s.dirty.Peek() {
		t.Fatal("form edit failed to save")
	}
	coll, _ = store.Load()
	got := coll.Requests[0].Body.Form
	if len(got) != 2 || got[0].Value != "edited" || got[1].Enabled {
		t.Fatal("form editor changed row semantics")
	}
}

func TestNativeUploadPathsUseCollectionRoot(t *testing.T) {
	dir := t.TempDir()
	app := New(Config{Store: collection.NewNativeStore(dir)})
	defer app.Close()
	if app.uploadRoot() != dir {
		t.Fatal("file picker resolves uploads outside native collection")
	}
}

func TestNativeFileBodyEditorSavesPathAndMIME(t *testing.T) {
	dir := t.TempDir()
	store := collection.NewNativeStore(dir)
	req := model.NewRequest()
	req.File = "file.request.yaml"
	req.URL = "https://example.test"
	req.Method = model.MethodPost
	req.Body = model.Body{Type: model.BodyFile, File: "old.bin", ContentType: "application/octet-stream"}
	if err := store.Create(req); err != nil {
		t.Fatal(err)
	}
	coll, _ := store.Load()
	app := New(Config{Store: store, Collection: coll})
	defer app.Close()
	app.openRequest(coll.Requests[0])
	s := app.current()
	s.upload.text.SetText("new.bin")
	s.uploadType.SetText("application/x-fixture")
	s.touch()
	app.saveRequest()
	if s.dirty.Peek() {
		t.Fatal("file body save failed")
	}
	coll, _ = store.Load()
	body := coll.Requests[0].Body
	if body.File != "new.bin" || body.ContentType != "application/x-fixture" {
		t.Fatal("file editor lost path or MIME")
	}
}

func TestNativeMultipartArrayEditorKeepsAllFiles(t *testing.T) {
	dir := t.TempDir()
	input := "$kind: http-request\nmethod: POST\nurl: https://example.test\nbody:\n  type: formdata\n  content:\n    - type: file\n      key: uploads\n      src: [one.bin, two.bin]\n      description: GUI note\n"
	if err := os.WriteFile(filepath.Join(dir, "array.request.yaml"), []byte(input), 0600); err != nil {
		t.Fatal(err)
	}
	store := collection.NewNativeStore(dir)
	coll, _ := store.Load()
	app := New(Config{Store: store, Collection: coll})
	defer app.Close()
	app.openRequest(coll.Requests[0])
	s := app.current()
	parts := s.multipart.values()
	if len(parts) != 2 || parts[1].File != "two.bin" {
		t.Fatal("UI hid a GUI file")
	}
	s.url.SetText("https://example.test/edited")
	s.urlEdited()
	app.saveRequest()
	if s.dirty.Peek() {
		t.Fatal("save failed")
	}
	coll, _ = store.Load()
	if len(coll.Requests[0].Body.Multipart) != 2 {
		t.Fatal("UI save dropped a file")
	}
}

func TestNativeEnvironmentSelectionReachesSender(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "dev.environment.yaml")
	os.WriteFile(file, []byte("name: Dev\nvalues:\n  - key: base_url\n    value: https://example.test\n  - key: route\n    value: environment\n  - key: disabled\n    value: skip\n    enabled: false\n"), 0600)
	calls := make(chan client.Call, 1)
	app := New(Config{Store: collection.NewNativeStore(dir), Environments: env.NativeFiles{Roots: []string{dir}}, Sender: client.SenderFunc(func(_ context.Context, call client.Call) (*model.Response, error) {
		calls <- call
		return fixedResponse(), nil
	})})
	defer app.Close()
	app.switchEnvironment([]string{file})
	app.setSessionVars(map[string]string{"route": "session"})
	s := app.current()
	s.url.SetText("{{base_url}}/{{route}}")
	s.urlEdited()
	updates := make(chan func(), 1)
	s.dispatch = func(fn func()) { updates <- fn }
	app.send()
	select {
	case call := <-calls:
		if len(call.Variables) != 2 || call.Variables["route"] != "session" || call.Variables["base_url"] != "https://example.test" {
			t.Fatal("native variable precedence or snapshot incorrect")
		}
	case <-time.After(time.Second):
		t.Fatal("request not sent")
	}
	select {
	case fn := <-updates:
		fn()
	case <-time.After(time.Second):
		t.Fatal("send not completed")
	}
}
