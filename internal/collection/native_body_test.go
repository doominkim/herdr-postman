package collection

import (
	"github.com/darrenburns/posting/v3/internal/model"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestNativeFormBodyRoundTripAndSwitch(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "form.request.yaml")
	input := "$kind: http-request\nmethod: POST\nurl: https://example.test\nbody:\n  type: urlencoded\n  content:\n    - key: tag\n      value: old\n      description: GUI note\n    - key: tag\n      value: second\n    - key: skip\n      value: hidden\n      disabled: true\n"
	if err := os.WriteFile(file, []byte(input), 0600); err != nil {
		t.Fatal(err)
	}
	store := NewNativeStore(dir)
	coll, problems := store.Load()
	if len(problems) > 0 {
		t.Fatal(problems)
	}
	req := coll.Requests[0]
	if req.Body.Type != model.BodyForm || len(req.Body.Form) != 3 || req.Body.Form[2].Enabled {
		t.Fatal("native form not loaded")
	}
	req.Body.Form[0].Value = "{{updated}}"
	if err := store.Save(req); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(file)
	if !strings.Contains(string(data), "GUI note") || !strings.Contains(string(data), "{{updated}}") {
		t.Fatal("form metadata or edit lost")
	}
	req.Body = model.Body{Type: model.BodyRaw, Raw: "plain", ContentType: "text/plain"}
	if err := store.Save(req); err != nil {
		t.Fatal(err)
	}
	req.Body = model.Body{Type: model.BodyForm, Form: []model.KeyValue{{Name: "a", Value: "b", Enabled: true}}}
	if err := store.Save(req); err != nil {
		t.Fatal(err)
	}
	coll, problems = store.Load()
	if len(problems) > 0 || len(coll.Requests[0].Body.Form) != 1 {
		t.Fatal("raw/form switch failed")
	}
}

func TestNativeMultipartRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := NewNativeStore(dir)
	req := model.NewRequest()
	req.File = "upload.request.yaml"
	req.Method = model.MethodPost
	req.URL = "https://example.test"
	req.Body = model.Body{Type: model.BodyMultipart, Multipart: []model.Part{{Type: model.PartText, Name: "note", Value: "hello", Enabled: true}, {Type: model.PartFile, Name: "upload", File: "payload.bin", ContentType: "application/octet-stream", Enabled: true}, {Type: model.PartText, Name: "disabled", Value: "skip"}}}
	if err := store.Create(req); err != nil {
		t.Fatal(err)
	}
	coll, problems := store.Load()
	if len(problems) > 0 {
		t.Fatal(problems)
	}
	got := coll.Requests[0]
	if !reflect.DeepEqual(got.Body.Multipart, req.Body.Multipart) {
		t.Fatal("multipart fields changed on reload")
	}
	got.Body.Multipart[0].Value = "edited"
	if err := store.Save(got); err != nil {
		t.Fatal(err)
	}
	coll, _ = store.Load()
	if coll.Requests[0].Body.Multipart[0].Value != "edited" {
		t.Fatal("edit not retained")
	}
	got.Body.Multipart[1].Filename = "custom.bin"
	if err := store.Save(got); err == nil {
		t.Fatal("unmapped filename override silently lost")
	}
}

func TestNativeFileBodyRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := NewNativeStore(dir)
	req := model.NewRequest()
	req.File = "file.request.yaml"
	req.Method = model.MethodPost
	req.URL = "https://example.test"
	req.Body = model.Body{Type: model.BodyFile, File: "payload.bin", ContentType: "application/octet-stream"}
	if err := store.Create(req); err != nil {
		t.Fatal(err)
	}
	coll, problems := store.Load()
	if len(problems) > 0 {
		t.Fatal(problems)
	}
	got := coll.Requests[0]
	if got.Body.Type != model.BodyFile || got.Body.File != "payload.bin" || got.Body.ContentType != "application/octet-stream" {
		t.Fatal("file body not loaded")
	}
	got.Body.File = "replacement.bin"
	if err := store.Save(got); err != nil {
		t.Fatal(err)
	}
	coll, _ = store.Load()
	if coll.Requests[0].Body.File != "replacement.bin" {
		t.Fatal("file edit lost")
	}
	got = coll.Requests[0]
	got.Body = model.Body{Type: model.BodyRaw, ContentType: "text/plain", Raw: "text"}
	if err := store.Save(got); err != nil {
		t.Fatal(err)
	}
	coll, _ = store.Load()
	if !strings.Contains(string(mustReadNativeBody(t, filepath.Join(dir, "file.request.yaml"))), "text/plain") {
		t.Fatal("file MIME remained after switching to text")
	}
	if coll.Requests[0].Body.Raw != "text" {
		t.Fatal("file to text switch failed")
	}
}

func mustReadNativeBody(t *testing.T, file string) []byte {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestNativeFileClearingMIMERemovesEnabledHeader(t *testing.T) {
	dir := t.TempDir()
	store := NewNativeStore(dir)
	req := model.NewRequest()
	req.File = "file.request.yaml"
	req.URL = "https://example.test"
	req.Body = model.Body{Type: model.BodyFile, File: "file.bin", ContentType: "application/octet-stream"}
	req.Headers = []model.KeyValue{{Name: "Content-Type", Value: "disabled/type", Enabled: false}}
	if err := store.Create(req); err != nil {
		t.Fatal(err)
	}
	coll, _ := store.Load()
	req = coll.Requests[0]
	req.Body.ContentType = ""
	if err := store.Save(req); err != nil {
		t.Fatal(err)
	}
	coll, _ = store.Load()
	req = coll.Requests[0]
	if req.Body.ContentType != "" || len(req.Headers) != 1 || req.Headers[0].Enabled || req.Headers[0].Value != "disabled/type" {
		t.Fatal("clearing file MIME damaged header semantics")
	}
}

func TestNativeMultipartGUIFileArrayPreservesEveryFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "files.request.yaml")
	input := "$kind: http-request\nmethod: POST\nurl: https://example.test\nbody:\n  type: formdata\n  content:\n    - type: file\n      key: uploads\n      src: [first.bin, second.bin]\n      description: shared GUI note\n      contentType: application/octet-stream\n    - type: file\n      key: skipped\n      src: [skip1.bin, skip2.bin]\n      disabled: true\n    - type: text\n      key: note\n      value: original\n"
	if err := os.WriteFile(file, []byte(input), 0600); err != nil {
		t.Fatal(err)
	}
	store := NewNativeStore(dir)
	coll, problems := store.Load()
	if len(problems) > 0 {
		t.Fatal(problems)
	}
	req := coll.Requests[0]
	parts := req.Body.Multipart
	if len(parts) != 5 || parts[0].File != "first.bin" || parts[1].File != "second.bin" || parts[2].Enabled || parts[3].Enabled {
		t.Fatal("GUI file array was not expanded faithfully")
	}
	req.Body.Multipart[4].Value = "edited"
	if err := store.Save(req); err != nil {
		t.Fatal(err)
	}
	coll, problems = store.Load()
	if len(problems) > 0 {
		t.Fatal(problems)
	}
	if !reflect.DeepEqual(coll.Requests[0].Body.Multipart, req.Body.Multipart) {
		t.Fatal("saving neighboring text changed files")
	}
	data := mustReadNativeBody(t, file)
	if strings.Count(string(data), "shared GUI note") != 2 {
		t.Fatal("file descriptions not preserved")
	}
	req = coll.Requests[0]
	req.Body.Multipart[1].File = "replacement.bin"
	if err := store.Save(req); err != nil {
		t.Fatal(err)
	}
	coll, _ = store.Load()
	if coll.Requests[0].Body.Multipart[0].File != "first.bin" || coll.Requests[0].Body.Multipart[1].File != "replacement.bin" {
		t.Fatal("individual file edit changed sibling")
	}
}
