package ui

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/darrenburns/posting/v3/internal/cloud"
)

func TestCloudEditorSaveUpdatesServerAndKeepsFailedDraft(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, "url")
	t.Setenv("CLOUD_UI_STATE", state)
	bin := filepath.Join(dir, "postman")
	os.WriteFile(bin, []byte(`#!/bin/sh
case "$*" in
 'collection get c1 --json') printf '%s' '{"$kind":"collection","id":"c1","name":"Fixture","items":[{"$kind":"http-request","id":"r1"}]}' ;;
 'collection request get r1 --collection c1 --workspace w1 --json')
 if [ -f "$CLOUD_UI_STATE" ]; then url=$(cat "$CLOUD_UI_STATE"); else url=https://example.test/old; fi
 printf '{"$kind":"http-request","id":"r1","name":"Fixture","method":"GET","url":"%s"}' "$url" ;;
 *)
 if [ "$3" = update ] && [ "${12}" = --url ]; then printf '%s' "${13}" > "$CLOUD_UI_STATE"; printf '{}'; else exit 3; fi ;;
esac
`), 0700)
	store, err := (cloud.CLI{Binary: bin}).OpenStore(context.Background(), "w1", "c1")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	coll, problems := store.Load()
	if len(problems) != 0 {
		t.Fatal(problems)
	}
	app := New(Config{Collection: coll, Store: store})
	defer app.Close()
	app.openRequest(coll.Requests[0])
	for _, url := range []string{"https://example.test/new", "https://example.test/again"} {
		app.current().url.SetText(url)
		app.current().urlEdited()
		app.saveRequest()
		if app.current().dirty.Peek() {
			t.Fatal("Cloud save failed", url)
		}
		saved, _ := os.ReadFile(state)
		if string(saved) != url {
			t.Fatal("Cloud did not receive editor URL")
		}
	}
	os.WriteFile(state, []byte("https://example.test/gui"), 0600)
	app.current().url.SetText("https://example.test/draft")
	app.current().urlEdited()
	app.saveRequest()
	if !app.current().dirty.Peek() {
		t.Fatal("conflicting draft was marked saved")
	}
	saved, _ := os.ReadFile(state)
	if string(saved) != "https://example.test/gui" {
		t.Fatal("GUI change overwritten")
	}
}
