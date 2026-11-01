package collection

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/darrenburns/posting/v3/internal/model"
)

func TestNativeGraphQLReopensDesktopCompactHeaders(t *testing.T) {
	dir := t.TempDir()
	source := `$kind: http-request
name: Desktop GraphQL
method: POST
url: https://example.test/graphql
headers:
  Content-Type: application/json
  Accept: application/graphql-response+json
  X-Fixture: desktop
body:
  type: json
  content: '{"query":"query FromDesktop { hello }","variables":{"message":"desktop"},"operationName":"FromDesktop"}'
`
	if err := os.WriteFile(filepath.Join(dir, "graphql.request.yaml"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	store := NewNativeStore(dir)
	coll, problems := store.Load()
	if len(problems) != 0 {
		t.Fatal(problems)
	}
	req := coll.Requests[0]
	want := model.GraphQL{Query: "query FromDesktop { hello }", Variables: `{"message":"desktop"}`, OperationName: "FromDesktop"}
	if req.Payload != want {
		t.Fatalf("Desktop GraphQL editor state lost: %#v", req.Payload)
	}
	if len(req.Headers) != 3 || req.Headers[2].Value != "desktop" || !req.Headers[2].Enabled {
		t.Fatal("Desktop headers not loaded")
	}
	req.Headers[2].Value = "edited"
	req.Headers = append(req.Headers, model.KeyValue{Name: "X-Fixture", Value: "disabled duplicate", Enabled: false})
	if err := store.Save(req); err != nil {
		t.Fatal(err)
	}
	reopened := NewNativeStore(dir)
	coll, problems = reopened.Load()
	if len(problems) != 0 {
		t.Fatal(problems)
	}
	got := coll.Requests[0]
	if got.Payload != want || !reflect.DeepEqual(got.Headers, req.Headers) {
		t.Fatal("editing compact headers lost GraphQL or header rows")
	}
}

func TestNativeGraphQLUsesHTTPAndReopensEditor(t *testing.T) {
	dir := t.TempDir()
	store := NewNativeStore(dir)
	req := model.NewRequest()
	req.File = "graphql.request.yaml"
	req.Name = "GraphQL fixture"
	req.URL = "https://example.test/graphql"
	req.Payload = model.GraphQL{Query: "query User($id: ID!) { user(id: $id) { name } }", Variables: `{"id":"{{user_id}}"}`, OperationName: "User"}
	if err := store.Create(req); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, req.File))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "$kind: http-request") || !strings.Contains(string(data), "method: POST") || strings.Contains(string(data), "graphql-request") {
		t.Fatal("GraphQL not stored as ordinary HTTP")
	}
	coll, problems := store.Load()
	if len(problems) > 0 {
		t.Fatal(problems)
	}
	got := coll.Requests[0]
	if got.Payload != req.Payload {
		t.Fatalf("GraphQL editor payload lost: %#v", got.Payload)
	}
	g := got.Payload.(model.GraphQL)
	g.OperationName = "Other"
	g.Query = "query Other { viewer { name } }"
	got.Payload = g
	if err := store.Save(got); err != nil {
		t.Fatal(err)
	}
	coll, problems = store.Load()
	if len(problems) > 0 || coll.Requests[0].Payload != g {
		t.Fatal("GraphQL edits not round-tripped", problems)
	}
}

func TestNativeGraphQLDoesNotReinterpretLossyOrUnmarkedJSON(t *testing.T) {
	for _, test := range []struct {
		name, body string
		accept     bool
	}{
		{"unmarked", `{"query":"select * from users"}`, false},
		{"extension", `{"query":"{ viewer { name } }","extensions":{"persisted":true}}`, true},
		{"duplicate", `{"query":"{ first }","query":"{ second }"}`, true},
		{"null-vars", `{"query":"{ viewer }","variables":null}`, true},
		{"invalid-vars", `{"query":"{ viewer }","variables":[]}`, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := NewNativeStore(t.TempDir())
			req := model.NewRequest()
			req.File = "raw.request.yaml"
			req.Method = model.MethodPost
			req.URL = "https://example.test"
			req.Body = model.Body{Type: model.BodyRaw, ContentType: "application/json", Raw: test.body}
			if test.accept {
				req.Headers = []model.KeyValue{{Name: "Accept", Value: "application/graphql-response+json", Enabled: true}}
			}
			if err := store.Create(req); err != nil {
				t.Fatal(err)
			}
			coll, problems := store.Load()
			if len(problems) > 0 {
				t.Fatal(problems)
			}
			got := coll.Requests[0]
			if got.Payload != nil || got.Body.Raw != test.body {
				t.Fatal("HTTP JSON was reinterpreted with information loss")
			}
		})
	}
}
