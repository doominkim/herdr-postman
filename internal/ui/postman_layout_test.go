package ui

import (
	"github.com/darrenburns/posting/v3/internal/collection"
	"github.com/darrenburns/posting/v3/internal/model"
	t "github.com/darrenburns/terma"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestPostmanLayoutPlacesSaveAboveURLAndOrdersRequestTabs(tt *testing.T) {
	req := model.NewRequest()
	req.Name = "Get users"
	req.URL = "https://api.example.test/users"
	req.File = "users/get.request.yaml"
	app := New(Config{AppName: "herdr-postman", PostmanUI: true, UserHost: "fixture@host", Collection: &model.Collection{Name: "My Workspace", Children: []*model.Collection{{Name: "Users API", Path: "users", Requests: []model.Request{req}}}}})
	app.openRequest(req)
	defer app.Close()
	rendered := t.RenderToBuffer(app, 150, 45)
	if file := os.Getenv("HERDR_POSTMAN_UI_PREVIEW"); file != "" {
		if err := os.WriteFile(file, []byte(t.BufferToSVG(rendered, 150, 45, t.DefaultSVGOptions())), 0600); err != nil {
			tt.Fatal(err)
		}
	}
	buf := t.SerializeBuffer(rendered, 150, 45)
	var out strings.Builder
	for i, c := range buf.Cells {
		out.WriteString(c.Content)
		if (i+1)%150 == 0 {
			out.WriteByte('\n')
		}
	}
	screen := out.String()
	rows := strings.Split(screen, "\n")
	save, send := -1, -1
	for i, row := range rows {
		if strings.Contains(row, "Save") {
			if save < 0 {
				save = i
			}
		}
		if strings.Contains(row, "Send") {
			if send < 0 {
				send = i
			}
		}
	}
	if save < 0 || send <= save {
		tt.Fatal("Save must be above the URL/Send row", screen)
	}
	for _, label := range []string{"Collections", "Params", "Authorization", "Settings"} {
		if !strings.Contains(screen, label) {
			tt.Fatal("missing Postman navigation label", label)
		}
	}
	tabs := app.current().requestTabs().Tabs
	if tabs[0].Key != "query" || tabs[1].Key != "auth" || tabs[2].Key != "headers" || tabs[3].Key != "body" {
		tt.Fatal("Postman request tab order not used")
	}
	if app.settings.SidebarGradient != "off" {
		tt.Fatal("Postman surface must not use Posting gradient")
	}
}

func TestPostmanSaveButtonPersistsEditorDraft(tt *testing.T) {
	root := tt.TempDir()
	file := filepath.Join(root, "get.request.yaml")
	if err := os.WriteFile(file, []byte("$kind: http-request\nname: Get item\nmethod: GET\nurl: https://example.test/old\n"), 0600); err != nil {
		tt.Fatal(err)
	}
	store := collection.NewNativeStore(root)
	coll, _ := store.Load()
	app := New(Config{AppName: "herdr-postman", PostmanUI: true, Store: store, Collection: coll})
	defer app.Close()
	app.openRequest(coll.Requests[0])
	app.current().url.SetText("https://example.test/new")
	app.current().urlEdited()
	clickText(tt, app, "Save", 0)
	if app.current().dirty.Peek() {
		tt.Fatal("visible Save button did not save")
	}
	data, _ := os.ReadFile(file)
	if !strings.Contains(string(data), "https://example.test/new") {
		tt.Fatal("saved URL missing")
	}
}

func TestPostmanKeyboardUsesRequestAndCommandShortcuts(tt *testing.T) {
	app := New(Config{PostmanUI: true})
	defer app.Close()
	for id, key := range map[string]string{"send-request": "ctrl+enter", "new-request": "ctrl+t", "commands": "ctrl+shift+p"} {
		if !slices.Contains(app.keysFor(id), key) {
			tt.Fatalf("%s missing shortcut %s", id, key)
		}
	}
	if slices.Contains(app.keysFor("focus-method"), "ctrl+t") {
		tt.Fatal("new request shortcut still changes HTTP method")
	}
}

func TestPostmanQuestionLinkOpensProjectIssues(t *testing.T) {
	opened := make(chan string, 1)
	app := New(Config{AppName: "herdr-postman", PostmanUI: true, Collection: model.SampleCollection(), OpenURL: func(url string) error { opened <- url; return nil }})
	defer app.Close()
	screen := newScreen(app, 100, 35)
	link := screen.renderer.WidgetByID("postman-question")
	if link == nil || link.Visible.IsEmpty() {
		t.Fatal("Question link is missing")
	}
	if link.Visible.X < 70 || link.Visible.Y != 34 {
		t.Fatalf("Question is not at bottom right: %v", link.Visible)
	}
	clickID(t, app, "postman-question")
	if got := nextOpened(t, opened); got != "https://github.com/doominkim/herdr-postman/issues" {
		t.Fatalf("wrong question destination: %s", got)
	}
}
