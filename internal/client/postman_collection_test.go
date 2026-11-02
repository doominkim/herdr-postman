package client

import (
	"bytes"
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sync/atomic"
	"testing"

	"github.com/darrenburns/posting/v3/internal/collection"
	"github.com/darrenburns/posting/v3/internal/model"
)

func TestPostmanCollectionPreservesInheritedBehaviorAndBinaryResponse(t *testing.T) {
	bin, err := exec.LookPath("postman")
	if err != nil {
		t.Skip("Postman CLI not installed")
	}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/draft" {
			t.Errorf("unsaved draft not executed: %s", r.URL.Path)
		}
		if r.Header.Get("X-Inherited") != "from-collection" {
			t.Errorf("inherited script did not run")
		}
		u, p, ok := r.BasicAuth()
		if !ok || u != "demo" || p != "fixture" {
			t.Errorf("inherited auth missing")
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(201)
		w.Write([]byte{0, 255, 65, 10})
	}))
	defer server.Close()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".resources"), 0700); err != nil {
		t.Fatal(err)
	}
	definition := fmt.Sprintf(`$kind: collection
name: Integration
variables:
  - key: base_url
    value: '%s'
auth:
  type: basic
  credentials:
    - key: username
      value: demo
    - key: password
      value: fixture
scripts:
  - type: http:beforeRequest
    language: text/javascript
    code: "pm.request.headers.add({key:'X-Inherited',value:'from-collection'});"
`, server.URL)
	if err := os.WriteFile(filepath.Join(dir, ".resources", "definition.yaml"), []byte(definition), 0600); err != nil {
		t.Fatal(err)
	}
	input := []byte("$kind: http-request\nmethod: GET\nurl: '{{base_url}}/binary'\nscripts:\n  - type: afterResponse\n    language: text/javascript\n    code: \"pm.test('created', () => pm.response.to.have.status(201));\"\n")
	file := filepath.Join(dir, "binary.request.yaml")
	if err := os.WriteFile(file, input, 0600); err != nil {
		t.Fatal(err)
	}
	store := collection.NewNativeStore(dir)
	coll, problems := store.Load()
	if len(problems) > 0 {
		t.Fatal(problems)
	}
	req := coll.Requests[0]
	req.URL = "{{base_url}}/draft"
	resp, err := (&PostmanCollection{Binary: bin, Root: dir, Render: store.Render}).Send(context.Background(), Call{Request: req})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 201 || !bytes.Equal(resp.Body, []byte{0, 255, 65, 10}) || resp.Header("Content-Type") != "application/octet-stream" {
		t.Fatalf("response mismatch: %+v", resp)
	}
	if calls.Load() != 1 {
		t.Fatalf("request sent %d times", calls.Load())
	}
	if resp.PostmanRun == nil || resp.PostmanRun.ExitCode != 0 || !bytes.Contains([]byte(resp.PostmanRun.Output), []byte("created")) {
		t.Fatal("assertion result missing")
	}
	current, _ := os.ReadFile(file)
	if !bytes.Equal(current, input) {
		t.Fatal("original request modified")
	}
}

func TestPostmanCollectionReturnsResponseWhenAssertionFails(t *testing.T) {
	bin, err := exec.LookPath("postman")
	if err != nil {
		t.Skip("Postman CLI not installed")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("response retained")) }))
	defer server.Close()
	dir := t.TempDir()
	input := fmt.Sprintf("$kind: http-request\nmethod: GET\nurl: '%s'\nscripts:\n  - type: afterResponse\n    language: text/javascript\n    code: \"pm.test('intentional failure', () => pm.response.to.have.status(201));\"\n", server.URL)
	if err := os.WriteFile(filepath.Join(dir, "failure.request.yaml"), []byte(input), 0600); err != nil {
		t.Fatal(err)
	}
	store := collection.NewNativeStore(dir)
	coll, problems := store.Load()
	if len(problems) > 0 {
		t.Fatal(problems)
	}
	resp, err := (&PostmanCollection{Binary: bin, Root: dir, Render: store.Render}).Send(context.Background(), Call{Request: coll.Requests[0]})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 || string(resp.Body) != "response retained" {
		t.Fatalf("HTTP response lost: %+v", resp)
	}
	if resp.PostmanRun == nil || resp.PostmanRun.ExitCode == 0 || !bytes.Contains([]byte(resp.PostmanRun.Output), []byte("intentional failure")) {
		t.Fatal("failed assertion missing")
	}
}

func TestPostmanCollectionRunsUnsavedDraftWithoutCreatingSourceFile(t *testing.T) {
	bin, err := exec.LookPath("postman")
	if err != nil {
		t.Skip("Postman CLI not installed")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method: %s", r.Method)
		}
		data, _ := io.ReadAll(r.Body)
		if string(data) != "draft body" {
			t.Errorf("body: %q", data)
		}
		w.WriteHeader(201)
		w.Write([]byte("draft response"))
	}))
	defer server.Close()
	dir := t.TempDir()
	store := collection.NewNativeStore(dir)
	req := model.NewRequest()
	req.Method = model.MethodPost
	req.URL = server.URL
	req.Body = model.Body{Type: model.BodyRaw, Raw: "draft body", ContentType: "text/plain"}
	resp, err := (&PostmanCollection{Binary: bin, Root: dir, Render: store.Render}).Send(context.Background(), Call{Request: req})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 201 || string(resp.Body) != "draft response" {
		t.Fatalf("response: %+v", resp)
	}
	files, _ := os.ReadDir(dir)
	if len(files) != 0 {
		t.Fatal("execution created source files")
	}
}

func TestPostmanNativeAuthExecution(t *testing.T) {
	bin, err := exec.LookPath("postman")
	if err != nil {
		t.Skip("Postman CLI not installed")
	}
	for _, scheme := range []model.AuthType{model.AuthBasic, model.AuthBearer, model.AuthNone, model.AuthInherit, model.AuthDigest} {
		t.Run(string(scheme), func(t *testing.T) {
			var authenticated atomic.Bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				header := r.Header.Get("Authorization")
				valid := false
				switch scheme {
				case model.AuthBasic:
					u, p, ok := r.BasicAuth()
					valid = ok && u == "fixture" && p == "password"
				case model.AuthBearer:
					valid = header == "Bearer fixture-token"
				case model.AuthNone:
					valid = header == ""
				case model.AuthInherit:
					valid = header == "Bearer inherited-token"
				case model.AuthDigest:
					if header == "" {
						w.Header().Set("WWW-Authenticate", `Digest realm="fixture", nonce="test-nonce", qop="auth", algorithm=MD5`)
						w.WriteHeader(401)
						return
					}
					fields := map[string]string{}
					for _, m := range regexp.MustCompile(`(\w+)=(?:"([^"]*)"|([^, ]+))`).FindAllStringSubmatch(header, -1) {
						fields[m[1]] = m[2] + m[3]
					}
					hash := func(s string) string { return fmt.Sprintf("%x", md5.Sum([]byte(s))) }
					want := hash(hash("fixture:fixture:password") + ":test-nonce:" + fields["nc"] + ":" + fields["cnonce"] + ":auth:" + hash(r.Method+":"+r.URL.RequestURI()))
					valid = fields["username"] == "fixture" && fields["response"] == want
				}
				if !valid {
					t.Error("authorization mismatch")
					w.WriteHeader(401)
					return
				}
				authenticated.Store(true)
				w.Write([]byte("authorized"))
			}))
			defer server.Close()
			dir := t.TempDir()
			os.Mkdir(filepath.Join(dir, ".resources"), 0700)
			os.WriteFile(filepath.Join(dir, ".resources", "definition.yaml"), []byte("$kind: collection\nauth:\n  type: bearer\n  credentials:\n    - key: token\n      value: inherited-token\n"), 0600)
			req := model.NewRequest()
			req.URL = server.URL
			req.Auth = model.Auth{Type: scheme, Username: "fixture", Password: "password", Token: "fixture-token"}
			store := collection.NewNativeStore(dir)
			resp, err := (&PostmanCollection{Binary: bin, Root: dir, Render: store.Render}).Send(context.Background(), Call{Request: req})
			if err != nil {
				t.Fatal(err)
			}
			if !authenticated.Load() || resp.StatusCode != 200 {
				t.Fatal("authenticated response not received")
			}
		})
	}
}

func TestPostmanNativeFormExecution(t *testing.T) {
	bin, err := exec.LookPath("postman")
	if err != nil {
		t.Skip("Postman CLI not installed")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if got := r.PostForm["tag"]; len(got) != 2 || got[0] != "한글 + &" || got[1] != "second" {
			t.Errorf("form values: %v", got)
		}
		if _, ok := r.PostForm["disabled"]; ok {
			t.Error("disabled form row sent")
		}
		if value, ok := r.PostForm["empty"]; !ok || len(value) != 1 || value[0] != "" {
			t.Error("empty value missing")
		}
		w.Write([]byte("form received"))
	}))
	defer server.Close()
	dir := t.TempDir()
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	store := collection.NewNativeStore(dir)
	req := model.NewRequest()
	req.File = "form.request.yaml"
	req.URL = server.URL
	req.Method = model.MethodPost
	req.Body = model.Body{Type: model.BodyForm, Form: []model.KeyValue{{Name: "tag", Value: "한글 + &", Enabled: true}, {Name: "tag", Value: "second", Enabled: true}, {Name: "disabled", Value: "not sent"}, {Name: "empty", Enabled: true}}}
	if err := store.Create(req); err != nil {
		t.Fatal(err)
	}
	lint := exec.Command(bin, "collection", "lint", dir)
	lint.Dir = dir
	if output, err := lint.CombinedOutput(); err != nil {
		t.Fatalf("native form lint: %v %s", err, output)
	}
	coll, problems := store.Load()
	if len(problems) > 0 {
		t.Fatal(problems)
	}
	resp, err := (&PostmanCollection{Binary: bin, Root: dir, Render: store.Render}).Send(context.Background(), Call{Request: coll.Requests[0]})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 || string(resp.Body) != "form received" {
		t.Fatal("form response missing")
	}
}

func TestPostmanNativeMultipartExecution(t *testing.T) {
	bin, err := exec.LookPath("postman")
	if err != nil {
		t.Skip("Postman CLI not installed")
	}
	payload := []byte{0, 255, 1, 2, 10}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1024 * 1024); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		defer r.MultipartForm.RemoveAll()
		if r.FormValue("note") != "한글 text" {
			t.Error("text part missing")
		}
		if _, ok := r.MultipartForm.Value["disabled"]; ok {
			t.Error("disabled part sent")
		}
		f, header, err := r.FormFile("upload")
		if err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		defer f.Close()
		data, _ := io.ReadAll(f)
		if !bytes.Equal(data, payload) || header.Filename != "payload.bin" {
			t.Error("uploaded file changed")
		}
		w.Write([]byte("uploaded"))
	}))
	defer server.Close()
	dir := t.TempDir()
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "payload.bin"), payload, 0600); err != nil {
		t.Fatal(err)
	}
	store := collection.NewNativeStore(dir)
	req := model.NewRequest()
	req.File = "upload.request.yaml"
	req.Method = model.MethodPost
	req.URL = server.URL
	req.Body = model.Body{Type: model.BodyMultipart, Multipart: []model.Part{{Type: model.PartText, Name: "note", Value: "한글 text", Enabled: true}, {Type: model.PartFile, Name: "upload", File: "payload.bin", ContentType: "application/octet-stream", Enabled: true}, {Type: model.PartText, Name: "disabled", Value: "skip"}}}
	if err := store.Create(req); err != nil {
		t.Fatal(err)
	}
	lint := exec.Command(bin, "collection", "lint", dir)
	lint.Dir = dir
	if output, err := lint.CombinedOutput(); err != nil {
		t.Fatalf("multipart lint: %v %s", err, output)
	}
	coll, _ := store.Load()
	resp, err := (&PostmanCollection{Binary: bin, Root: dir, Render: store.Render}).Send(context.Background(), Call{Request: coll.Requests[0]})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 || string(resp.Body) != "uploaded" {
		t.Fatal("multipart response missing")
	}
}

func TestPostmanNativeFileBodyExecution(t *testing.T) {
	bin, err := exec.LookPath("postman")
	if err != nil {
		t.Skip("Postman CLI not installed")
	}
	payload := []byte{0, 255, 13, 10, 128, 65}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		if !bytes.Equal(data, payload) {
			t.Error("file bytes changed")
		}
		if r.Header.Get("Content-Type") != "application/octet-stream" {
			t.Error("file content type missing")
		}
		w.Write([]byte("binary received"))
	}))
	defer server.Close()
	for _, uploadPath := range []string{"payload.bin", "{{upload_file}}"} {
		t.Run(uploadPath, func(t *testing.T) {
			dir := t.TempDir()
			dir, err = filepath.EvalSymlinks(dir)
			if err != nil {
				t.Fatal(err)
			}
			os.WriteFile(filepath.Join(dir, "payload.bin"), payload, 0600)
			store := collection.NewNativeStore(dir)
			req := model.NewRequest()
			req.File = "binary.request.yaml"
			req.URL = server.URL
			req.Method = model.MethodPost
			req.Body = model.Body{Type: model.BodyFile, File: uploadPath, ContentType: "application/octet-stream"}
			if err := store.Create(req); err != nil {
				t.Fatal(err)
			}
			lint := exec.Command(bin, "collection", "lint", dir)
			lint.Dir = dir
			if output, err := lint.CombinedOutput(); err != nil {
				t.Fatalf("file lint: %v %s", err, output)
			}
			coll, _ := store.Load()
			resp, err := (&PostmanCollection{Binary: bin, Root: dir, Render: store.Render}).Send(context.Background(), Call{Request: coll.Requests[0], Variables: map[string]string{"upload_file": "payload.bin"}})
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != 200 || string(resp.Body) != "binary received" {
				t.Fatal("file response missing")
			}
		})
	}
}

func TestPostmanGUIFileArrayExecutesAfterNativeEdit(t *testing.T) {
	bin, err := exec.LookPath("postman")
	if err != nil {
		t.Skip("Postman CLI not installed")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1024 * 1024); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		defer r.MultipartForm.RemoveAll()
		files := r.MultipartForm.File["uploads"]
		if len(files) != 2 {
			t.Errorf("received %d files", len(files))
			w.WriteHeader(400)
			return
		}
		for i, header := range files {
			f, err := header.Open()
			if err != nil {
				t.Error(err)
				continue
			}
			data, _ := io.ReadAll(f)
			f.Close()
			if string(data) != []string{"first bytes", "second bytes"}[i] {
				t.Error("file order or bytes changed")
			}
		}
		if r.FormValue("note") != "edited" {
			t.Error("draft text edit missing")
		}
		w.Write([]byte("both uploaded"))
	}))
	defer server.Close()
	dir := t.TempDir()
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{"first.bin": "first bytes", "second.bin": "second bytes"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	input := fmt.Sprintf("$kind: http-request\nmethod: POST\nurl: '%s'\nbody:\n  type: formdata\n  content:\n    - type: file\n      key: uploads\n      src: [first.bin, second.bin]\n      description: shared note\n    - type: text\n      key: note\n      value: original\n", server.URL)
	if err := os.WriteFile(filepath.Join(dir, "array.request.yaml"), []byte(input), 0600); err != nil {
		t.Fatal(err)
	}
	store := collection.NewNativeStore(dir)
	coll, problems := store.Load()
	if len(problems) > 0 {
		t.Fatal(problems)
	}
	req := coll.Requests[0]
	req.Body.Multipart[2].Value = "edited"
	if err := store.Save(req); err != nil {
		t.Fatal(err)
	}
	lint := exec.Command(bin, "collection", "lint", dir)
	lint.Dir = dir
	if output, err := lint.CombinedOutput(); err != nil {
		t.Fatalf("edited array lint: %v %s", err, output)
	}
	resp, err := (&PostmanCollection{Binary: bin, Root: dir, Render: store.Render}).Send(context.Background(), Call{Request: req})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 || string(resp.Body) != "both uploaded" {
		t.Fatal("multi-file response missing")
	}
}

func TestPostmanNativeEnvironmentOverridesCollectionVariables(t *testing.T) {
	bin, err := exec.LookPath("postman")
	if err != nil {
		t.Skip("Postman CLI not installed")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/environment" || r.Header.Get("X-Empty") != "" {
			t.Error("selected environment not applied")
		}
		w.Write([]byte("environment applied"))
	}))
	defer server.Close()
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, ".resources"), 0700)
	os.WriteFile(filepath.Join(dir, ".resources", "definition.yaml"), []byte("$kind: collection\nvariables:\n  - key: base_url\n    value: http://127.0.0.1:1\n  - key: route\n    value: collection\n"), 0600)
	req := model.NewRequest()
	req.URL = "{{base_url}}/{{route}}"
	req.Headers = []model.KeyValue{{Name: "X-Empty", Value: "{{empty}}", Enabled: true}}
	store := collection.NewNativeStore(dir)
	resp, err := (&PostmanCollection{Binary: bin, Root: dir, Render: store.Render}).Send(context.Background(), Call{Request: req, Variables: map[string]string{"base_url": server.URL, "route": "environment", "empty": ""}})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 || string(resp.Body) != "environment applied" {
		t.Fatal("environment response missing")
	}
}

func TestPostmanNativeCollectionVariablesApplyInsideFolders(t *testing.T) {
	bin, err := exec.LookPath("postman")
	if err != nil {
		t.Skip("Postman CLI not installed")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Folder") != "{{folder_only}}" {
			t.Error("nonstandard folder variable unexpectedly promoted")
		}
		if r.URL.Path != "/root" {
			t.Errorf("folder variable resolved to %q", r.URL.Path)
		}
		w.Write([]byte("folder inherited"))
	}))
	defer server.Close()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".resources"), 0700)
	os.MkdirAll(filepath.Join(dir, "folder", ".resources"), 0700)
	os.WriteFile(filepath.Join(dir, ".resources", "definition.yaml"), []byte(fmt.Sprintf("$kind: collection\nvariables:\n  - key: base_url\n    value: '%s'\n  - key: route\n    value: root\n", server.URL)), 0600)
	os.WriteFile(filepath.Join(dir, "folder", ".resources", "definition.yaml"), []byte("$kind: collection\nvariables:\n  - key: route\n    value: folder\n  - key: folder_only\n    value: unique-folder-value\n"), 0600)
	os.WriteFile(filepath.Join(dir, "folder", "get.request.yaml"), []byte("$kind: http-request\nmethod: GET\nurl: '{{base_url}}/{{route}}'\nheaders:\n  - key: X-Folder\n    value: '{{folder_only}}'\n"), 0600)
	store := collection.NewNativeStore(dir)
	coll, problems := store.Load()
	if len(problems) > 0 {
		t.Fatal(problems)
	}
	req := coll.Children[0].Requests[0]
	resp, err := (&PostmanCollection{Binary: bin, Root: dir, Render: store.Render}).Send(context.Background(), Call{Request: req})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 || string(resp.Body) != "folder inherited" {
		t.Fatal("folder execution failed")
	}
}

func TestPostmanCollectionExplainsUnreachableServer(t *testing.T) {
	bin, err := exec.LookPath("postman")
	if err != nil {
		t.Skip("Postman CLI not installed")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	cases := []struct {
		name, url, want string
	}{
		{"refused", fmt.Sprintf("http://127.0.0.1:%d/orders", port), fmt.Sprintf("Couldn't connect to GET http://127.0.0.1:%d/orders: connection refused (ECONNREFUSED). Is the server running?", port)},
		{"unknown host", "http://herdr-postman-missing.invalid/orders", "Couldn't connect to GET http://herdr-postman-missing.invalid/orders: host not found (ENOTFOUND)."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			input := fmt.Sprintf("$kind: http-request\nmethod: GET\nurl: '%s'\n", tc.url)
			if err := os.WriteFile(filepath.Join(dir, "orders.request.yaml"), []byte(input), 0600); err != nil {
				t.Fatal(err)
			}
			store := collection.NewNativeStore(dir)
			coll, problems := store.Load()
			if len(problems) > 0 {
				t.Fatal(problems)
			}
			_, err := (&PostmanCollection{Binary: bin, Root: dir, Render: store.Render}).Send(context.Background(), Call{Request: coll.Requests[0]})
			if err == nil || err.Error() != tc.want {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}
