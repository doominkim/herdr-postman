package client

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/darrenburns/posting/v3/internal/collection"
)

func TestPostmanCollectionSelectsRequestInsideRenamedFolders(t *testing.T) {
	bin, err := exec.LookPath("postman")
	if err != nil {
		t.Skip("Postman CLI not installed")
	}
	var selected, other atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/selected" {
			selected.Add(1)
		} else {
			other.Add(1)
		}
		w.Write([]byte("selected response"))
	}))
	defer server.Close()
	dir := t.TempDir()
	files := map[string]string{
		".resources/definition.yaml":             "$kind: collection\nname: Selection fixture\n",
		"outer/.resources/definition.yaml":       "$kind: collection\nname: GUI folder\n",
		"outer/inner/.resources/definition.yaml": "$kind: collection\nname: Nested display name\n",
		"outer/inner/get.request.yaml":           fmt.Sprintf("$kind: http-request\nname: Shared name\nmethod: GET\nurl: '%s/selected'\n", server.URL),
		"other.request.yaml":                     fmt.Sprintf("$kind: http-request\nname: Shared name\nmethod: GET\nurl: '%s/other'\n", server.URL),
	}
	for file, data := range files {
		full := filepath.Join(dir, filepath.FromSlash(file))
		if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	store := collection.NewNativeStore(dir)
	coll, problems := store.Load()
	if len(problems) != 0 {
		t.Fatal(problems)
	}
	req := coll.Children[0].Children[0].Requests[0]
	resp, err := (&PostmanCollection{Binary: bin, Root: dir, Render: store.Render}).Send(context.Background(), Call{Request: req})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 || string(resp.Body) != "selected response" || selected.Load() != 1 || other.Load() != 0 {
		t.Fatalf("selection: status=%d selected=%d other=%d", resp.StatusCode, selected.Load(), other.Load())
	}
	for file, original := range files {
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(file)))
		if err != nil || string(data) != original {
			t.Fatalf("source changed: %s", file)
		}
	}
}

func TestPostmanCollectionPreservesIDAndNameWithAmbiguousSibling(t *testing.T) {
	bin, err := exec.LookPath("postman")
	if err != nil {
		t.Skip("Postman CLI not installed")
	}
	var calls atomic.Int32
	identities := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		select {
		case identities <- r.Header.Get("X-Request-ID"):
		default:
			t.Error("unexpected extra request")
		}
		if r.URL.Path != "/selected" || r.Header.Get("X-Request-Name") != "Same/name" {
			t.Error("wrong request or changed script-visible identity")
		}
		w.Write([]byte("selected"))
	}))
	defer server.Close()
	dir := t.TempDir()
	source := fmt.Sprintf("$kind: http-request\nid: fixture-stable-id\nname: Same/name\nmethod: GET\nurl: '%s/selected'\nscripts:\n  - type: beforeRequest\n    language: text/javascript\n    code: \"pm.request.headers.add({key:'X-Request-ID', value:pm.info.requestId}); pm.request.headers.add({key:'X-Request-Name',value:pm.info.requestName});\"\n", server.URL)
	for file, data := range map[string]string{
		"selected.request.yaml": source,
		"other.request.yaml":    fmt.Sprintf("$kind: http-request\nname: Same/name\nmethod: GET\nurl: '%s/other'\n", server.URL),
	} {
		if err := os.WriteFile(filepath.Join(dir, file), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	store := collection.NewNativeStore(dir)
	coll, problems := store.Load()
	if len(problems) != 0 {
		t.Fatal(problems)
	}
	var req = coll.Requests[0]
	for _, candidate := range coll.Requests {
		if candidate.File == "selected.request.yaml" {
			req = candidate
		}
	}
	resp, err := (&PostmanCollection{Binary: bin, Root: dir, Render: store.Render}).Send(context.Background(), Call{Request: req})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 || string(resp.Body) != "selected" || calls.Load() != 1 {
		t.Fatal("did not execute exactly the selected request")
	}
	if <-identities != "fixture-stable-id" {
		t.Fatal("existing ID changed")
	}
	duplicate, err := store.Duplicate(req, "duplicate.request.yaml")
	if err != nil {
		t.Fatal(err)
	}
	resp, err = (&PostmanCollection{Binary: bin, Root: dir, Render: store.Render}).Send(context.Background(), Call{Request: duplicate})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 || calls.Load() != 2 {
		t.Fatal("duplicate did not execute exactly once")
	}
	if id := <-identities; id == "" || id == "fixture-stable-id" {
		t.Fatal("duplicate reused original identity")
	}
	data, err := os.ReadFile(filepath.Join(dir, "selected.request.yaml"))
	if err != nil || string(data) != source {
		t.Fatal("source changed")
	}
}

func TestPostmanCollectionRejectsDuplicateIDsBeforeSending(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer server.Close()
	dir := t.TempDir()
	for _, name := range []string{"first", "second"} {
		source := fmt.Sprintf("$kind: http-request\nid: duplicated-id\nname: %s\nmethod: GET\nurl: '%s'\n", name, server.URL)
		if err := os.WriteFile(filepath.Join(dir, name+".request.yaml"), []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
	}
	store := collection.NewNativeStore(dir)
	coll, problems := store.Load()
	if len(problems) != 0 {
		t.Fatal(problems)
	}
	_, err := (&PostmanCollection{Root: dir, Render: store.Render}).Send(context.Background(), Call{Request: coll.Requests[0]})
	if err == nil || !strings.Contains(err.Error(), "duplicate Postman item ID") || calls.Load() != 0 {
		t.Fatalf("must reject duplicate identity without sending: error=%v calls=%d", err, calls.Load())
	}
}
