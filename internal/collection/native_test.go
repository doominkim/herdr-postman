package collection

import (
	"bytes"
	"errors"
	"github.com/darrenburns/posting/v3/internal/model"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestNativeRenderIncludesDraftWithoutSaving(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "get.request.yaml")
	original := []byte("$kind: http-request\nmethod: GET\nurl: https://example.test/saved\nexamples: ./keep/\n")
	if err := os.WriteFile(file, original, 0600); err != nil {
		t.Fatal(err)
	}
	store := NewNativeStore(dir)
	coll, _ := store.Load()
	req := coll.Requests[0]
	req.URL = "https://example.test/draft"
	data, err := store.Render(req)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte("/draft")) || !bytes.Contains(data, []byte("./keep/")) {
		t.Fatalf("draft not preserved: %s", data)
	}
	onDisk, _ := os.ReadFile(file)
	if !bytes.Equal(onDisk, original) {
		t.Fatal("preview saved changes")
	}
}

func TestNativeTLSAndRedirectOptionsPreserveAdvancedSettings(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "get.request.yaml")
	if err := os.WriteFile(file, []byte("$kind: http-request\nmethod: GET\nurl: https://example.test\nsettings:\n  strictSSL: false\n  followRedirects: false\n  maxRedirects: 7\n"), 0600); err != nil {
		t.Fatal(err)
	}
	store := NewNativeStore(dir)
	coll, problems := store.Load()
	if len(problems) > 0 {
		t.Fatal(problems)
	}
	req := coll.Requests[0]
	if req.Options.VerifySSL || req.Options.FollowRedirects {
		t.Fatal("GUI options not loaded")
	}
	req.Options.VerifySSL = true
	if err := store.Save(req); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(file)
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"strictSSL": true, "followRedirects": false, "maxRedirects": 7}
	if !reflect.DeepEqual(doc["settings"], want) {
		t.Fatalf("settings changed: %s", data)
	}
}

func TestNativeCreatedRequestPassesPostmanCLI(t *testing.T) {
	bin, err := exec.LookPath("postman")
	if err != nil {
		t.Skip("Postman CLI is not installed")
	}
	dir := t.TempDir()
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	store := NewNativeStore(dir)
	req := model.NewRequest()
	req.Name, req.File, req.URL = "Create user", "Create user.request.yaml", "{{base_url}}/users"
	req.Method = model.MethodPost
	req.Body = model.Body{Type: model.BodyRaw, ContentType: "application/json", Raw: `{"name":"Alice"}`}
	if err := store.Create(req); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(bin, "collection", "lint", dir).CombinedOutput(); err != nil {
		t.Fatalf("Postman rejected native collection: %v\n%s", err, output)
	}
	command := exec.Command(bin, "collection", "request", "get", "Create user", "--collection", dir, "--json")
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("Postman cannot read created request: %v\n%s", err, output)
	}
	if !bytes.Contains(output, []byte("Alice")) || !bytes.Contains(output, []byte("{{base_url}}/users")) {
		t.Fatalf("Postman read lost body or URL: %s", output)
	}
}

func TestNativeRequestLifecycle(t *testing.T) {
	dir := t.TempDir()
	store := NewNativeStore(dir)
	req := model.NewRequest()
	req.Name, req.File, req.URL = "Create user", "users/create.request.yaml", "{{base_url}}/users"
	req.Method = model.MethodPost
	req.Options.VerifySSL = false
	req.Options.FollowRedirects = false
	req.Body = model.Body{Type: model.BodyRaw, ContentType: "application/json", Raw: `{"name":"Alice"}`}
	if err := store.Create(req); err != nil {
		t.Fatal(err)
	}
	if err := store.Create(req); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("duplicate must fail without overwriting, got %v", err)
	}
	coll, problems := store.Load()
	if len(problems) > 0 || len(coll.Children) != 1 || len(coll.Children[0].Requests) != 1 {
		t.Fatalf("collection: %+v, %v", coll, problems)
	}
	loaded := coll.Children[0].Requests[0]
	if loaded.Options.VerifySSL || loaded.Options.FollowRedirects {
		t.Fatal("new request options were silently dropped")
	}
	if loaded.Name != req.Name || loaded.Body.Raw != req.Body.Raw || loaded.Method != req.Method {
		t.Fatalf("created request changed: %+v", loaded)
	}
	if err := store.Delete(loaded.File); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, req.File)); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("request still exists")
	}
	if err := store.Create(model.Request{File: "../escaped.request.yaml"}); err == nil {
		t.Fatal("path escape accepted")
	}
}

func TestNativeEditBodyAndParameters(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "request.request.yaml")
	input := `$kind: http-request
method: POST
url: '{{base_url}}/users/:id'
queryParams:
  - key: locale
    value: en
    description: GUI query hint
    disabled: true
pathVariables:
  - key: id
    value: '1'
    description: GUI path hint
body:
  type: json
  content: '{"name":"before"}'
scripts:
  - type: afterResponse
    language: text/javascript
    code: pm.test('ok', () => pm.response.to.have.status(200));
`
	if err := os.WriteFile(file, []byte(input), 0600); err != nil {
		t.Fatal(err)
	}
	store := NewNativeStore(dir)
	coll, problems := store.Load()
	if len(problems) != 0 {
		t.Fatal(problems)
	}
	req := coll.Requests[0]
	if req.Body.Type != model.BodyRaw || req.Body.Raw != `{"name":"before"}` || req.Body.ContentType != "application/json" {
		t.Fatalf("body: %+v", req.Body)
	}
	if len(req.Query) != 1 || req.Query[0].Enabled || len(req.PathParams) != 1 {
		t.Fatal("parameter mapping missing")
	}
	req.Body.Raw = `{"name":"after"}`
	req.Query[0].Value = "ko"
	req.PathParams[0].Value = "2"
	if err := store.Save(req); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := yaml.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got["body"].(map[string]any)["content"] != `{"name":"after"}` {
		t.Fatal("body not saved")
	}
	for key, value := range map[string]string{"queryParams": "ko", "pathVariables": "2"} {
		row := got[key].([]any)[0].(map[string]any)
		if row["value"] != value || row["description"] == nil {
			t.Fatalf("parameter metadata lost: %s", data)
		}
	}
	if got["scripts"] == nil {
		t.Fatal("GUI script removed")
	}
}

func TestNativeHeaderEditKeepsDescriptionsAndDisabledRows(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "request.request.yaml")
	data := []byte("$kind: http-request\nmethod: GET\nurl: https://example.test\nheaders:\n  - key: X-Trace\n    value: old\n    description: GUI explanation\n    disabled: true\n")
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	store := NewNativeStore(dir)
	coll, problems := store.Load()
	if len(problems) != 0 {
		t.Fatal(problems)
	}
	req := coll.Requests[0]
	if len(req.Headers) != 1 || req.Headers[0].Enabled {
		t.Fatal("GUI disabled header was not loaded correctly")
	}
	req.Headers[0].Value = "{{trace_id}}"
	if err := store.Save(req); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := yaml.Unmarshal(got, &parsed); err != nil {
		t.Fatal(err)
	}
	want := []any{map[string]any{"key": "X-Trace", "value": "{{trace_id}}", "description": "GUI explanation", "disabled": true}}
	if !reflect.DeepEqual(parsed["headers"], want) {
		t.Fatalf("header metadata lost: %s", got)
	}
}

func TestNativeSaveRejectsUnimplementedEditsWithoutLosingData(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "request.request.yaml")
	data := []byte("$kind: http-request\nmethod: GET\nurl: https://example.test\n")
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	store := NewNativeStore(dir)
	coll, problems := store.Load()
	if len(problems) != 0 {
		t.Fatal(problems)
	}
	req := coll.Requests[0]
	req.Auth.Type = model.AuthOAuth2
	req.Auth.Token = "draft-not-saved"
	if err := store.Save(req); err == nil {
		t.Fatal("unsupported edit reported as saved")
	}
	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Fatal("rejected save changed the GUI document")
	}
}

func TestNativeSavePreservesGUIFields(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "Get user.request.yaml")
	original := `$kind: http-request
name: Get user
method: GET
url: '{{base_url}}/users/1'
order: 2000
auth:
  type: oauth2
  credentials:
    - key: accessToken
      value: '{{access_token}}'
scripts:
  - type: afterResponse
    language: text/javascript
    code: pm.test('OK', () => pm.response.to.have.status(200));
examples: './.resources/Get user.resources/examples/'
headers:
  - key: X-Trace
    value: 'true'
    description: Set by GUI
    disabled: true
`
	if err := os.WriteFile(file, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	store := NewNativeStore(dir)
	coll, problems := store.Load()
	if len(problems) != 0 {
		t.Fatal(problems)
	}
	if len(coll.Requests) != 1 {
		t.Fatalf("want one native request, got %d", len(coll.Requests))
	}
	req := coll.Requests[0]
	if req.URL != "{{base_url}}/users/1" {
		t.Fatalf("variable expression changed: %q", req.URL)
	}
	req.URL = "{{base_url}}/users/2"
	if err := store.Save(req); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var before, after map[string]any
	if err := yaml.Unmarshal([]byte(original), &before); err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(data, &after); err != nil {
		t.Fatal(err)
	}
	before["url"] = "{{base_url}}/users/2"
	want, _ := yaml.Marshal(before)
	got, _ := yaml.Marshal(after)
	if string(got) != string(want) {
		t.Fatalf("GUI fields changed beyond URL:\n%s", data)
	}
	if _, err := os.Stat(filepath.Join(dir, "Get user.posting.yaml")); !os.IsNotExist(err) {
		t.Fatal("created a second Posting-format source of truth")
	}
}

func TestNativeSaveDoesNotOverwriteGUIEditsAfterRefresh(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "request.request.yaml")
	initial := []byte("$kind: http-request\nmethod: GET\nurl: https://example.test/old\n")
	if err := os.WriteFile(file, initial, 0600); err != nil {
		t.Fatal(err)
	}
	store := NewNativeStore(dir)
	coll, _ := store.Load()
	draft := coll.Requests[0]
	draft.URL = "https://example.test/tui-draft"
	gui := []byte("$kind: http-request\nmethod: GET\nurl: https://example.test/gui-edit\n")
	if err := os.WriteFile(file, gui, 0600); err != nil {
		t.Fatal(err)
	}
	store.Load() // A file watcher refresh must not rebase an unsaved editor draft.
	if err := store.Save(draft); err == nil {
		t.Fatal("stale draft overwrote GUI edit")
	}
	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, gui) {
		t.Fatal("GUI edit was lost")
	}
}
