package remote

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/darrenburns/posting/v3/internal/model"
)

func TestPostmanRunSurvivesRemoteSerialization(t *testing.T) {
	response := &model.Response{StatusCode: 200, Body: []byte("ok"), PostmanRun: &model.PostmanRun{ExitCode: 1, Output: "fixture assertion failed"}}
	wire := NewResponse(response, model.Status{Code: "200", Text: "OK"})
	data, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Status     string `json:"status"`
		Body       string `json:"body"`
		PostmanRun *struct {
			ExitCode int    `json:"exitCode"`
			Output   string `json:"output"`
		} `json:"postmanRun"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if result.PostmanRun == nil || result.PostmanRun.ExitCode != 1 || result.PostmanRun.Output != "fixture assertion failed" || result.Status != "200" || result.Body != "ok" {
		t.Fatal("remote response lost independent run result")
	}
	response.PostmanRun = nil
	data, err = json.Marshal(NewResponse(response, model.Status{Code: "200"}))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "postmanRun") {
		t.Fatal("legacy response acquired empty run metadata")
	}
}
