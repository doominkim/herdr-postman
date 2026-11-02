package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	t "github.com/darrenburns/terma"
	uv "github.com/darrenburns/ultraviolet"

	"github.com/darrenburns/posting/v3/internal/collection"
	"github.com/darrenburns/posting/v3/internal/model"
)

// nativeApp is the Postman workbench over a local collection of two saved
// requests, with "Get item" open.
func nativeApp(tt *testing.T) (*App, string, *t.Pilot) {
	tt.Helper()
	root := tt.TempDir()
	for name, doc := range map[string]string{
		"get.request.yaml":  "$kind: http-request\nname: Get item\nmethod: GET\nurl: https://example.test/item\n",
		"list.request.yaml": "$kind: http-request\nname: List items\nmethod: GET\nurl: https://example.test/items\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(doc), 0600); err != nil {
			tt.Fatal(err)
		}
	}
	store := collection.NewNativeStore(root)
	coll, _ := store.Load()
	app := New(Config{AppName: "herdr-postman", PostmanUI: true, Store: store, Collection: coll})
	tt.Cleanup(func() { app.Close() })
	for _, req := range coll.Requests {
		if req.Name == "Get item" {
			app.openRequest(req)
		}
	}
	return app, root, t.NewPilot(tt, app, 150, 40)
}

func savedName(tt *testing.T, root, file string) string {
	tt.Helper()
	data, err := os.ReadFile(filepath.Join(root, file))
	if err != nil {
		tt.Fatal(err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if name, ok := strings.CutPrefix(line, "name: "); ok {
			return name
		}
	}
	return ""
}

// screenCell finds text on screen, in the column range [from, to).
func screenCell(tt *testing.T, p *t.Pilot, text string, from, to int) (int, int) {
	tt.Helper()
	for y, line := range strings.Split(p.ScreenText(), "\n") {
		runes := []rune(line)
		for x := from; x+len([]rune(text)) <= min(to, len(runes)); x++ {
			if string(runes[x:x+len([]rune(text))]) == text {
				return x, y
			}
		}
	}
	tt.Fatalf("%q not on screen:\n%s", text, p.ScreenText())
	return 0, 0
}

func TestPostmanTitleClickRenamesOnlyTheName(tt *testing.T) {
	app, root, p := nativeApp(tt)
	p.Click(postmanTitleID)
	if p.FocusedID() != postmanRenameID {
		tt.Fatalf("clicking the title focused %q", p.FocusedID())
	}
	p.Press("ctrl+a")
	p.Type("Fetch item")
	p.Press("enter")
	settleUntil(p, func() bool { return app.requestActions.saving.Peek() == nil })
	if got := savedName(tt, root, "get.request.yaml"); got != "Fetch item" {
		tt.Fatalf("saved name = %q", got)
	}
	if s := app.current(); s.dirty.Peek() || s.name.GetText() != "Fetch item" {
		tt.Fatalf("tab after rename: dirty=%v name=%q", s.dirty.Peek(), s.name.GetText())
	}
	if !strings.Contains(p.TextOf(postmanTitleID), "Fetch item") {
		tt.Fatalf("title = %q", p.TextOf(postmanTitleID))
	}
}

func TestPostmanTitleRenameEscapeKeepsTheName(tt *testing.T) {
	_, root, p := nativeApp(tt)
	p.Click(postmanTitleID)
	p.Type(" changed")
	p.Press("escape")
	if got := savedName(tt, root, "get.request.yaml"); got != "Get item" {
		tt.Fatalf("escape saved %q", got)
	}
	if !strings.Contains(p.TextOf(postmanTitleID), "Get item") {
		tt.Fatalf("title after escape = %q", p.TextOf(postmanTitleID))
	}
}

func TestPostmanRenameKeepsUnsavedEditsSafe(tt *testing.T) {
	app, root, p := nativeApp(tt)
	s := app.current()
	s.url.SetText("https://example.test/draft")
	s.urlEdited()
	p.Click(postmanTitleID)
	p.Press("ctrl+a")
	p.Type("Renamed")
	p.Press("enter")
	if got := savedName(tt, root, "get.request.yaml"); got != "Get item" {
		tt.Fatalf("renaming a tab with unsaved edits saved %q", got)
	}
}

func rightClickTree(tt *testing.T, p *t.Pilot, label string) {
	tt.Helper()
	x, y := screenCell(tt, p, label, 0, 40)
	p.MouseDown(x, y, uv.MouseRight, 0)
	p.MouseUp(x, y, uv.MouseRight, 0)
}

func TestPostmanTreeMenuDuplicatesAndDeletes(tt *testing.T) {
	_, root, p := nativeApp(tt)
	rightClickTree(tt, p, "List items")
	screen := p.ScreenText()
	for _, item := range []string{"Rename", "Duplicate", "Delete"} {
		if !strings.Contains(screen, item) {
			tt.Fatalf("menu lacks %s after a right click:\n%s", item, screen)
		}
	}
	x, y := screenCell(tt, p, "Duplicate", 0, 150)
	p.ClickAt(x, y)
	if got := savedName(tt, root, "list-copy.request.yaml"); got != "List items (copy)" {
		tt.Fatalf("duplicate saved %q", got)
	}

	rightClickTree(tt, p, "List items")
	x, y = screenCell(tt, p, "Delete", 0, 150)
	p.ClickAt(x, y)
	if !strings.Contains(p.ScreenText(), "Delete request?") {
		tt.Fatalf("delete did not ask first:\n%s", p.ScreenText())
	}
}

func TestPostmanTreeMenuRenameEditsTheTitle(tt *testing.T) {
	app, root, p := nativeApp(tt)
	rightClickTree(tt, p, "List items")
	x, y := screenCell(tt, p, "Rename", 0, 150)
	p.ClickAt(x, y)
	if p.FocusedID() != postmanRenameID || app.current().file.Peek() != "list.request.yaml" {
		tt.Fatalf("rename focused %q on %q", p.FocusedID(), app.current().file.Peek())
	}
	p.Press("ctrl+a")
	p.Type("All items")
	p.Press("enter")
	settleUntil(p, func() bool { return app.requestActions.saving.Peek() == nil })
	if got := savedName(tt, root, "list.request.yaml"); got != "All items" {
		tt.Fatalf("saved name = %q", got)
	}
}

// labeledApp opens a collection whose folder is labelled differently from
// its directory, as Cloud folders and Desktop-named folders are.
func labeledApp(tt *testing.T, store interface {
	collection.Store
	Load() (*model.Collection, []collection.LoadError)
}) (*App, *t.Pilot) {
	tt.Helper()
	coll, _ := store.Load()
	app := New(Config{AppName: "herdr-postman", PostmanUI: true, Store: store, Collection: coll})
	tt.Cleanup(func() { app.Close() })
	req, ok := app.savedRequest("orders/list.request.yaml")
	if !ok {
		tt.Fatal("fixture request missing")
	}
	app.openRequest(req)
	return app, t.NewPilot(tt, app, 150, 40)
}

func labeledRoot(tt *testing.T) string {
	tt.Helper()
	root := tt.TempDir()
	dir := filepath.Join(root, "orders")
	for name, doc := range map[string]string{
		".resources/definition.yaml": "$kind: collection\nname: Orders tab\n",
		"list.request.yaml":          "$kind: http-request\nname: 주문 목록\nmethod: GET\nurl: https://example.test/orders\n",
	} {
		os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0700)
		if err := os.WriteFile(filepath.Join(dir, name), []byte(doc), 0600); err != nil {
			tt.Fatal(err)
		}
	}
	return root
}

func TestPostmanRenameUpdatesTheRowInALabelledFolder(tt *testing.T) {
	app, p := labeledApp(tt, collection.NewNativeStore(labeledRoot(tt)))
	app.renameSession(app.current(), "주문 목록s")
	settleUntil(p, func() bool { return app.requestActions.saving.Peek() == nil })
	if got := strings.Join(shownRows(app), "|"); got != "Orders tab/|GET 주문 목록s" {
		tt.Fatalf("tree after rename = %q", got)
	}
}

func TestPostmanDeleteRemovesTheRowInALabelledFolder(tt *testing.T) {
	app, _ := labeledApp(tt, collection.NewNativeStore(labeledRoot(tt)))
	app.deleteRequests([]string{"orders/list.request.yaml"})
	if got := strings.Join(shownRows(app), "|"); strings.Contains(got, "주문 목록") {
		tt.Fatalf("tree after delete = %q", got)
	}
}

// slowStore holds saves until released, as a Cloud save over the Postman
// CLI takes seconds.
type slowStore struct {
	*collection.NativeStore
	release chan struct{}
}

func (s slowStore) Save(req model.Request) error {
	<-s.release
	return s.NativeStore.Save(req)
}

func TestPostmanSlowRenameShowsProgressAndStaysResponsive(tt *testing.T) {
	root := labeledRoot(tt)
	store := slowStore{NativeStore: collection.NewNativeStore(root), release: make(chan struct{})}
	app, p := labeledApp(tt, store)
	done := make(chan struct{})
	go func() { t.Dispatch(func() { app.renameSession(app.current(), "주문 목록s"); close(done) }) }()
	if !settleUntil(p, func() bool {
		select {
		case <-done:
			return true
		default:
			return false
		}
	}) {
		tt.Fatal("renaming blocked the interface until the save finished")
	}
	if !strings.Contains(p.TextOf(postmanTitleID), "Saving") {
		tt.Fatalf("title while saving = %q", p.TextOf(postmanTitleID))
	}
	close(store.release)
	if !settleUntil(p, func() bool { return app.requestActions.saving.Peek() == nil }) {
		tt.Fatal("rename never finished")
	}
	if got := savedName(tt, filepath.Join(root, "orders"), "list.request.yaml"); got != "주문 목록s" {
		tt.Fatalf("saved name = %q", got)
	}
}
