package ui

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	t "github.com/darrenburns/terma"

	"github.com/darrenburns/posting/v3/internal/collection"
	"github.com/darrenburns/posting/v3/internal/model"
)

// codeTabApp is the Posting workbench with "List users" open on its Code tab.
func codeTabApp(tt *testing.T) (*App, *Session, *t.Pilot) {
	tt.Helper()
	app := testApp()
	tt.Cleanup(func() { app.Close() })
	app.openRequest(sampleRequest(tt, "List users"))
	p := t.NewPilot(tt, app, 150, 45)
	p.Click(tabID(requestTabsID, "code"))
	return app, app.current(), p
}

func TestCodeTabShowsTheRequestURLAndCommandAsSent(tt *testing.T) {
	_, _, p := codeTabApp(tt)
	const want = "http://localhost:8000/users?page=1&per_page=20"
	if got := strings.TrimSpace(p.TextOf(codeURLID)); got != want {
		tt.Fatalf("request URL = %q, want %q", got, want)
	}
	command := p.TextOf(codeCommandID)
	if !strings.Contains(command, "curl") || !strings.Contains(command, want) || !strings.Contains(command, "Accept: application/json") {
		tt.Fatalf("command does not describe the request:\n%s", command)
	}
}

func TestCodeTabFollowsEditsAndEnvironment(tt *testing.T) {
	app, s, p := codeTabApp(tt)
	s.url.SetText("${BASE_URL}/teams")
	s.urlEdited()
	if got := strings.TrimSpace(p.TextOf(codeURLID)); got != "http://localhost:8000/teams" {
		tt.Fatalf("after editing the URL, request URL = %q", got)
	}
	// Nothing else on the tab shows the path, so only the tab's own
	// subscription to the request can bring this edit in.
	s.url.SetText("${BASE_URL}/groups")
	s.urlEdited()
	if got := strings.TrimSpace(p.TextOf(codeURLID)); got != "http://localhost:8000/groups" {
		tt.Fatalf("after editing the path, request URL = %q", got)
	}
	s.headers.Load([]model.KeyValue{{Name: "X-Team", Value: "core", Enabled: true}})
	s.touch()
	if command := p.TextOf(codeCommandID); !strings.Contains(command, "X-Team: core") {
		tt.Fatalf("after adding a header, command:\n%s", command)
	}
	app.env.active.Set(model.SampleEnvironments()[1])
	if got := strings.TrimSpace(p.TextOf(codeURLID)); got != "https://staging.example.com/api/groups" {
		tt.Fatalf("after switching environment, request URL = %q", got)
	}
}

func TestCodeTabShowsVariablesOnRequest(tt *testing.T) {
	_, _, p := codeTabApp(tt)
	p.Click(tabID(codeResolveID, "variables"))
	if got := strings.TrimSpace(p.TextOf(codeURLID)); got != "${BASE_URL}/users?page=1&per_page=20" {
		tt.Fatalf("request URL with variables = %q", got)
	}
	if command := p.TextOf(codeCommandID); !strings.Contains(command, "${BASE_URL}") {
		tt.Fatalf("command with variables:\n%s", command)
	}
}

func TestCodeTabExplainsAnUndefinedVariable(tt *testing.T) {
	_, s, p := codeTabApp(tt)
	s.url.SetText("${NOPE}/x")
	s.urlEdited()
	screen := p.ScreenText()
	if !strings.Contains(screen, "Variable not defined: $NOPE") {
		tt.Fatalf("undefined variable not explained:\n%s", screen)
	}
	if command := p.TextOf(codeCommandID); !strings.Contains(command, "${NOPE}/x") {
		tt.Fatalf("command should keep the variable in:\n%s", command)
	}
}

func TestCodeTabCopiesURLAndCommand(tt *testing.T) {
	app, _, p := codeTabApp(tt)
	p.Click(codeCopyURLID)
	if got := p.Clipboard(); got != "http://localhost:8000/users?page=1&per_page=20" {
		tt.Fatalf("copied URL = %q", got)
	}
	p.Click(codeCopyCommandID)
	want, err := app.exportCommand(true)
	if err != nil {
		tt.Fatal(err)
	}
	if got := p.Clipboard(); got != want {
		tt.Fatalf("copied command = %q, want %q", got, want)
	}
}

func TestCodeTabOnlyForExportableKinds(tt *testing.T) {
	app := testApp()
	defer app.Close()
	s := app.current()
	for kind, want := range map[model.KindID]bool{model.KindHTTP: true, model.KindGraphQL: true, model.KindGRPC: true, model.KindWebSocket: false} {
		s.setKind(kind)
		has := slices.ContainsFunc(s.requestTabList(), func(tab requestTab) bool { return tab.key == "code" })
		if has != want {
			tt.Errorf("%s: Code tab shown = %v, want %v", kind, has, want)
		}
	}
}

func TestCodeTabResolvesPostmanVariables(tt *testing.T) {
	root := tt.TempDir()
	os.MkdirAll(filepath.Join(root, ".resources"), 0700)
	os.WriteFile(filepath.Join(root, ".resources", "definition.yaml"), []byte("$kind: collection\nvariables:\n  - key: base_url\n    value: https://collection.test\n"), 0600)
	os.WriteFile(filepath.Join(root, "get.request.yaml"), []byte("$kind: http-request\nname: Get item\nmethod: GET\nurl: '{{base_url}}/items/{{item_id}}/{{later}}'\n"), 0600)
	store := collection.NewNativeStore(root)
	coll, _ := store.Load()
	app := New(Config{AppName: "herdr-postman", PostmanUI: true, Store: store, Collection: coll, SessionVariables: map[string]string{"item_id": "7"}})
	defer app.Close()
	app.openRequest(coll.Requests[0])
	tabs := app.current().requestTabList()
	if tabs[len(tabs)-1].key != "code" {
		tt.Fatal("the Code tab should come last in the Postman layout")
	}
	p := t.NewPilot(tt, app, 150, 45)
	p.Click(tabID(requestTabsID, "code"))
	// Collection variables fill in, as Postman does; a variable a script
	// sets only exists when Postman runs, so it is left for Postman.
	if got := strings.TrimSpace(p.TextOf(codeURLID)); got != "https://collection.test/items/7/{{later}}" {
		tt.Fatalf("request URL = %q", got)
	}
	if screen := p.ScreenText(); !strings.Contains(screen, "Postman adds it when sending") {
		tt.Fatalf("inherited authorization not explained:\n%s", screen)
	}
}
