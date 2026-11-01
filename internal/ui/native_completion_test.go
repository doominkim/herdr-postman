package ui

import (
	"context"
	"github.com/darrenburns/posting/v3/internal/client"
	"github.com/darrenburns/posting/v3/internal/collection"
	"github.com/darrenburns/posting/v3/internal/model"
	t "github.com/darrenburns/terma"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNativeVariableCompletionUsesPostmanSyntax(tt *testing.T) {
	app := New(Config{Store: collection.NewNativeStore(tt.TempDir()), SessionVariables: map[string]string{"base_url": "https://example.test", "api_token": "fixture"}})
	defer app.Close()
	choices := app.variableChoices()
	if len(choices.list) != 2 {
		tt.Fatal("Posting generators included in native completion")
	}
	for _, choice := range choices.list {
		if !strings.HasPrefix(choice.Value, "{{") || !strings.HasSuffix(choice.Value, "}}") {
			tt.Fatal("wrong variable syntax")
		}
		if choice.Value == "{{api_token}}" && choice.Description != "••••••••" {
			tt.Fatal("secret completion not masked")
		}
	}
	popup := newCompletion().wrap(t.ThemeData{}, t.Text{}, choices, t.Cells(40)).(t.Autocomplete)
	if len(popup.TriggerChars) != 1 || popup.TriggerChars[0] != '{' {
		tt.Fatal("native completion trigger missing")
	}
	for _, tc := range []struct {
		text            string
		cursor, trigger int
		want            string
	}{{"{{ba}}/path", 4, 1, "{{base_url}}/path"}, {"{ba", 3, 0, "{{base_url}}"}, {"한글 {{ba}}", 7, 4, "한글 {{base_url}}"}} {
		got, _ := popup.Insert(tc.text, tc.cursor, t.Suggestion{Value: "{{base_url}}"}, tc.trigger)
		if got != tc.want {
			tt.Fatalf("completion: %q != %q", got, tc.want)
		}
	}
	suggestions := (urlTree{}).suggestions("{{ba}}/path", 4, choices)
	if len(suggestions) != 1 || suggestions[0].Value != "{{base_url}}" {
		tt.Fatal("URL native completion missing")
	}
}

func TestNativeVariableHighlightFindsDoubleBraces(tt *testing.T) {
	text := "{{base_url}}/${literal}/{{missing}}"
	index := make([]int, len(text)+1)
	for i := range index {
		index[i] = i
	}
	spans := variableHighlights(t.ThemeData{}, text, index, func(name string) bool { return name == "base_url" }, true)
	if len(spans) != 2 || spans[0].Start != 0 || spans[0].End != 12 {
		tt.Fatal("native refs not highlighted")
	}
}

func TestNativeCompletionThroughEditorInput(tt *testing.T) {
	for _, field := range []string{urlInputID, "req-auth-token", "req-body-text"} {
		tt.Run(field, func(tt *testing.T) {
			app := New(Config{Store: collection.NewNativeStore(tt.TempDir()), SessionVariables: map[string]string{"base_url": "https://example.test"}})
			defer app.Close()
			s := app.current()
			s.authType.Set(model.AuthBearer)
			s.bodyType.Set(model.BodyRaw)
			if field == "req-auth-token" {
				s.requestTab.Set("auth")
			}
			if field == "req-body-text" {
				s.requestTab.Set("body")
			}
			screen := sidebarScreen(tt, app, field)
			if !screen.focus.Focused().(t.PasteHandler).HandlePaste("{{ba") {
				tt.Fatal("paste rejected")
			}
			screen.render()
			screen.pressKey(tt, "enter")
			got := s.url.GetText()
			if field == "req-auth-token" {
				got = s.token.GetText()
			}
			if field == "req-body-text" {
				got = s.body.GetText()
			}
			if got != "{{base_url}}" {
				tt.Fatalf("editor inserted %q", got)
			}
		})
	}
}

func TestNativeInheritedCompletionDoesNotOverrideRunnerScope(tt *testing.T) {
	dir := tt.TempDir()
	os.MkdirAll(filepath.Join(dir, "folder", ".resources"), 0700)
	os.MkdirAll(filepath.Join(dir, ".resources"), 0700)
	os.WriteFile(filepath.Join(dir, ".resources", "definition.yaml"), []byte("$kind: collection\nvariables:\n  - key: root_var\n    value: inherited\n  - key: shared\n    value: root\n"), 0600)
	os.WriteFile(filepath.Join(dir, "folder", ".resources", "definition.yaml"), []byte("$kind: collection\nvariables:\n  - key: shared\n    value: folder\n"), 0600)
	os.WriteFile(filepath.Join(dir, "folder", "get.request.yaml"), []byte("$kind: http-request\nmethod: GET\nurl: https://example.test/{{root_var}}\n"), 0600)
	store := collection.NewNativeStore(dir)
	coll, _ := store.Load()
	calls := make(chan client.Call, 1)
	app := New(Config{Store: store, Collection: coll, Sender: client.SenderFunc(func(_ context.Context, call client.Call) (*model.Response, error) {
		calls <- call
		return fixedResponse(), nil
	})})
	defer app.Close()
	app.openRequest(coll.Children[0].Requests[0])
	values := app.variableValues()
	if values["shared"] != "root" || values["root_var"] != "inherited" {
		tt.Fatal("inherited values missing from editor")
	}
	app.env.active.Set(model.Environment{Variables: []model.Variable{{Name: "shared", Value: "environment"}}})
	if app.variableValues()["shared"] != "environment" {
		tt.Fatal("environment must override collection display value")
	}
	choices := app.variableChoices()
	found := false
	for _, choice := range choices.list {
		if choice.Value == "{{root_var}}" {
			found = true
		}
	}
	if !found {
		tt.Fatal("collection variable not offered")
	}
	s := app.current()
	updates := make(chan func(), 1)
	s.dispatch = func(fn func()) { updates <- fn }
	app.send()
	select {
	case call := <-calls:
		if len(call.Variables) != 1 || call.Variables["shared"] != "environment" {
			tt.Fatal("inherited display variables flattened into execution environment")
		}
	case <-time.After(time.Second):
		tt.Fatal("send not called")
	}
	select {
	case fn := <-updates:
		fn()
	case <-time.After(time.Second):
		tt.Fatal("send not completed")
	}
}
