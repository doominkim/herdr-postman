package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darrenburns/posting/v3/internal/client"
	"github.com/darrenburns/posting/v3/internal/collection"
	"github.com/darrenburns/posting/v3/internal/model"
	terma "github.com/darrenburns/terma"
)

func TestNativeGraphQLEditorSendsThroughPostmanHTTP(t *testing.T) {
	bin, err := exec.LookPath("postman")
	if err != nil {
		t.Skip("Postman CLI not installed")
	}
	received := make(chan map[string]any, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if r.Method != "POST" || json.NewDecoder(r.Body).Decode(&body) != nil {
			w.WriteHeader(400)
			return
		}
		received <- body
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":null,"errors":[{"message":"fixture GraphQL error"}]}`))
	}))
	defer server.Close()
	dir := t.TempDir()
	store := collection.NewNativeStore(dir)
	req := model.NewRequest()
	req.Name = "GraphQL fixture"
	req.URL = server.URL
	req.Payload = model.GraphQL{Query: "query Before { viewer { name } }", OperationName: "Before"}
	app := New(Config{Store: store, Sender: &client.PostmanCollection{Binary: bin, Root: dir, Render: store.Render}, SessionVariables: map[string]string{"user_id": "fixture-id"}})
	defer app.Close()
	s := app.openSession(req)
	editor := s.payloads[model.KindGraphQL].(*graphQLEditor)
	editor.query.SetText("query User($id: ID!) { user(id: $id) { name } }")
	editor.variables.SetText(`{"id":"{{user_id}}"}`)
	editor.operation.SetText("User")
	s.touch()
	updates := make(chan func(), 2)
	s.dispatch = func(fn func()) { updates <- fn }
	send := func() {
		t.Helper()
		app.send()
		select {
		case fn := <-updates:
			fn()
		case <-time.After(15 * time.Second):
			s.Cancel()
			t.Fatal("GraphQL CLI send timed out")
		}
		if err := s.err.Peek(); err != nil {
			t.Fatal(err)
		}
		select {
		case body := <-received:
			if body["operationName"] != "User" || body["query"] != editor.query.GetText() {
				t.Fatal("GraphQL document changed")
			}
			vars, ok := body["variables"].(map[string]any)
			if !ok || vars["id"] != "fixture-id" {
				t.Fatal("Postman environment variables not applied")
			}
		default:
			t.Fatal("GraphQL endpoint not called")
		}
		if s.response.Peek().StatusCode != 200 || s.responseStatus.Class != model.StatusClassWarning {
			t.Fatal("GraphQL errors did not retain HTTP response and warning status")
		}
	}
	send()
	app.saveRequest()
	app.save.name.SetText("GraphQL fixture")
	app.submitSave()
	if s.file.Peek() != "graphql-fixture.request.yaml" {
		t.Fatal("GraphQL save failed", app.save.err.Peek())
	}
	source, err := os.ReadFile(filepath.Join(dir, s.file.Peek()))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(source), "$kind: http-request") {
		t.Fatal("GraphQL was not stored as HTTP")
	}
	if output, err := exec.Command(bin, "collection", "lint", dir).CombinedOutput(); err != nil {
		t.Fatalf("CLI rejected HTTP GraphQL file: %v %s", err, output)
	}
	coll, problems := store.Load()
	if len(problems) > 0 {
		t.Fatal(problems)
	}
	app.replaceCollection(coll)
	s.Load(coll.Requests[0])
	editor = s.payloads[model.KindGraphQL].(*graphQLEditor)
	send()
	after, _ := os.ReadFile(filepath.Join(dir, s.file.Peek()))
	if string(after) != string(source) {
		t.Fatal("sending changed saved request")
	}
}

func TestNativeGraphQLHighlightKeepsGraphQLVariablesDistinct(t *testing.T) {
	app := New(Config{Store: collection.NewNativeStore(t.TempDir()), SessionVariables: map[string]string{"field": "name"}})
	defer app.Close()
	text := "query User($id: ID!) { user(id: $id) { {{field}} } }"
	chars := strings.Split(text, "")
	base := app.queryHighlighter(terma.ThemeData{}, false).Highlight(text, chars)
	marked := app.queryHighlighter(terma.ThemeData{}, true).Highlight(text, chars)
	start := strings.Index(text, "{{field}}")
	if len(marked) != len(base)+1 || marked[len(marked)-1].Start != start || marked[len(marked)-1].End != start+9 {
		t.Fatal("native reference not distinguished from GraphQL $id")
	}
}
