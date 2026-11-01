package ui

import (
	"fmt"
	"github.com/darrenburns/posting/v3/internal/client"
	"github.com/darrenburns/posting/v3/internal/collection"
	"github.com/darrenburns/posting/v3/internal/model"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestSourceSwitchRequiresCleanIdleTabs(t *testing.T) {
	app := New(Config{})
	defer app.Close()
	if err := app.CanSwitchSource(); err != nil {
		t.Fatal(err)
	}
	app.newTab()
	app.current().url.SetText("https://example.test/draft")
	app.current().urlEdited()
	if err := app.CanSwitchSource(); err == nil {
		t.Fatal("dirty draft can be discarded by switching")
	}
	app.current().dirty.Set(false)
	app.current().phase.Set(exchangeSending)
	if err := app.CanSwitchSource(); err == nil {
		t.Fatal("running request can be abandoned by switching")
	}
	app.current().phase.Set(exchangeIdle)
	if err := app.CanSwitchSource(); err != nil {
		t.Fatal(err)
	}
}

func TestRefreshSourcePreservesDraftTabsAndCollapsedFolders(t *testing.T) {
	app := testApp()
	defer app.Close()
	app.openRequest(sampleRequest(t, "List users"))
	clean := app.current()
	app.openSession(sampleRequest(t, "Get user"))
	dirty := app.current()
	dirty.url.SetText("https://draft.test/keep")
	dirty.urlEdited()
	active := app.active.Peek()
	count := len(app.sessions.Peek())
	app.RestoreCollapsedFolders([]string{"users"})
	root := model.SampleCollection()
	root.Requests = append(root.Requests, model.Request{Name: "New cloud request", File: "new.request.yaml"})
	root.Walk(func(c *model.Collection, _ model.Request) {
		for i := range c.Requests {
			c.Requests[i].Description = "Cloud changed"
		}
	})
	if err := app.RefreshSource(Config{Collection: root, Store: collection.Memory{}, Sender: client.Fake{}}); err != nil {
		t.Fatal(err)
	}
	if len(app.sessions.Peek()) != count || app.active.Peek() != active {
		t.Fatal("tabs or selection lost")
	}
	if dirty.url.GetText() != "https://draft.test/keep" || !dirty.dirty.Peek() {
		t.Fatal("draft lost")
	}
	if clean.Snapshot().Description != "Cloud changed" {
		t.Fatal("clean tab not refreshed")
	}
	if app.collection.Peek() != root {
		t.Fatal("new cloud requests absent")
	}
	if !slices.Contains(app.CollapsedFolders(), "users") {
		t.Fatal("collapsed folder lost")
	}
}

func TestRefreshSourceRejectsRunningRequestsWithoutChangingCollection(t *testing.T) {
	app := testApp()
	defer app.Close()
	original := app.collection.Peek()
	app.current().phase.Set(exchangeSending)
	if err := app.RefreshSource(Config{Collection: &model.Collection{Name: "New"}}); err == nil {
		t.Fatal("running request allowed")
	}
	if app.collection.Peek() != original {
		t.Fatal("failed refresh changed collection")
	}
	app.current().phase.Set(exchangeIdle)
}

func TestRefreshSourceKeepsDraftConflictProtection(t *testing.T) {
	for _, changed := range []bool{false, true} {
		t.Run(fmt.Sprint(changed), func(t *testing.T) {
			makeStore := func(url string) *collection.NativeStore {
				dir := t.TempDir()
				err := os.WriteFile(filepath.Join(dir, "get.request.yaml"), []byte("$kind: http-request\nname: Get\nmethod: GET\nurl: "+url+"\n"), 0600)
				if err != nil {
					t.Fatal(err)
				}
				return collection.NewNativeStore(dir)
			}
			old := makeStore("https://example.test/original")
			root, _ := old.Load()
			app := New(Config{Store: old, Collection: root})
			defer app.Close()
			app.openRequest(root.Requests[0])
			app.current().url.SetText("https://example.test/draft")
			app.current().urlEdited()
			url := "https://example.test/original"
			if changed {
				url = "https://example.test/gui"
			}
			fresh := makeStore(url)
			next, _ := fresh.Load()
			if err := app.RefreshSource(Config{Store: fresh, Collection: next}); err != nil {
				t.Fatal(err)
			}
			app.saveRequest()
			saved, _ := fresh.Load()
			if changed {
				if !app.current().dirty.Peek() || saved.Requests[0].URL != url {
					t.Fatal("refresh allowed overwriting a concurrent GUI edit")
				}
			} else if app.current().dirty.Peek() || saved.Requests[0].URL != "https://example.test/draft" {
				t.Fatal("unchanged refreshed request could not save draft")
			}
		})
	}
}
