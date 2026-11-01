package ui

import (
	"reflect"
	"testing"

	"github.com/darrenburns/posting/v3/internal/model"
)

func TestSessionVariablesAreRestoredAndRemembered(tt *testing.T) {
	var remembered []map[string]string
	app := New(Config{
		Collection:       model.SampleCollection(),
		Environments:     StaticEnvironments(model.SampleEnvironments()),
		Environment:      []string{"local.env"},
		SessionVariables: map[string]string{"API_TOKEN": "from-last-run"},
		RememberSession:  func(vars map[string]string) { remembered = append(remembered, vars) },
		UserHost:         "user@host",
	})
	if got := app.variableValuesPeek()["API_TOKEN"]; got != "from-last-run" {
		tt.Fatalf("API_TOKEN = %q, want the value from the last run", got)
	}
	if len(remembered) != 0 {
		tt.Fatalf("restoring at startup isn't a change to remember: %v", remembered)
	}

	app.captureResponse(model.Request{Captures: map[string]string{"ROUND_ID": "data.id"}}, &model.Response{Body: []byte(`{"data":{"id":3}}`)})
	want := map[string]string{"API_TOKEN": "from-last-run", "ROUND_ID": "3"}
	if len(remembered) != 1 || !reflect.DeepEqual(remembered[0], want) {
		tt.Fatalf("after a capture remembered %v, want %v", remembered, want)
	}

	app.openVariables()
	app.editVariable(variableRow(tt, app, "BASE_URL"))
	app.variables.value.SetText("http://localhost:9000")
	app.commitVariable()
	if got := remembered[len(remembered)-1]["BASE_URL"]; got != "http://localhost:9000" {
		tt.Fatalf("an edited variable should be remembered: %v", remembered[len(remembered)-1])
	}

	rows := app.variables.table.GetRows()
	for i, row := range rows {
		if row.Name == "API_TOKEN" {
			app.variables.table.SelectIndex(i)
		}
	}
	app.revertVariable()
	if _, ok := remembered[len(remembered)-1]["API_TOKEN"]; ok {
		tt.Fatalf("a reverted variable shouldn't come back next run: %v", remembered[len(remembered)-1])
	}
}
