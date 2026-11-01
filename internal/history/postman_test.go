package history

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/darrenburns/posting/v3/internal/model"
)

func TestPostmanRunOutputUsesHistoryByteBudget(t *testing.T) {
	for _, size := range []int{MaxBytes, MaxBytes + 1} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			entry := model.HistoryEntry{ID: 1, Response: &model.Response{PostmanRun: &model.PostmanRun{ExitCode: 1, Output: strings.Repeat("x", size)}}}
			got := Trim([]model.HistoryEntry{entry})
			if size == MaxBytes && len(got) != 1 {
				t.Fatal("last allowed run output rejected")
			}
			if size > MaxBytes && len(got) != 0 {
				t.Fatal("oversized run output bypassed history limit")
			}
		})
	}
	entries := []model.HistoryEntry{
		{ID: 2, Response: &model.Response{Body: []byte("body"), PostmanRun: &model.PostmanRun{Output: strings.Repeat("x", MaxBytes-4)}}},
		{ID: 1, Response: &model.Response{PostmanRun: &model.PostmanRun{Output: "x"}}},
	}
	if got := Trim(entries); len(got) != 1 || got[0].ID != 2 {
		t.Fatal("combined body and run output exceeded budget")
	}
}

func TestPostmanRunAndBinaryResponseSurviveRestart(t *testing.T) {
	dir, root := t.TempDir(), t.TempDir()
	store := ForCollection(dir, root)
	req := model.NewRequest()
	req.File = "get.request.yaml"
	req.URL = "{{base_url}}/fixture"
	response := &model.Response{StatusCode: 200, Body: []byte{0, 255, 128}, Cookies: []model.Cookie{{Name: "fixture", Value: "value", HTTPOnly: true}}, PostmanRun: &model.PostmanRun{ExitCode: 1, Output: "fixture assertion failed"}}
	if _, err := store.Append(model.HistoryEntry{Request: req, Response: response}); err != nil {
		t.Fatal(err)
	}
	restored, err := ForCollection(dir, root).Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(restored) != 1 || !reflect.DeepEqual(restored[0].Response, response) || restored[0].Request.URL != req.URL || restored[0].Request.File != req.File {
		t.Fatal("native history lost request or response fields")
	}
}
