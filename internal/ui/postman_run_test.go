package ui

import (
	"github.com/darrenburns/posting/v3/internal/model"
	t "github.com/darrenburns/terma"
	"strings"
	"testing"
)

func TestPostmanRunTabSeparatesAssertionFailureFromHTTPResponse(tt *testing.T) {
	app := testApp()
	defer app.Close()
	s := app.current()
	resp := fixedResponse()
	resp.PostmanRun = &model.PostmanRun{ExitCode: 1, Output: "AssertionError: expected status 201"}
	s.showResponse(resp, nil)
	found := false
	for _, tab := range s.responseTabs(resp).Tabs {
		if tab.Key == "run" {
			found = true
			if tab.Label != "Run failed" {
				tt.Fatalf("label: %s", tab.Label)
			}
		}
	}
	if !found {
		tt.Fatal("Postman run tab missing")
	}
	s.responseTab.Set("run")
	s.showResponse(resp, nil)
	if s.responseTab.Peek() != "run" {
		tt.Fatal("selected run tab lost")
	}
	if s.response.Peek().StatusCode != 200 {
		tt.Fatal("HTTP status overwritten by assertion failure")
	}
	buf := t.RenderToBuffer(responseRun{session: s, response: resp}, 100, 20)
	svg := t.BufferToSVG(buf, 100, 20, t.DefaultSVGOptions())
	if !strings.Contains(svg, "AssertionError") {
		tt.Fatal("run output not rendered")
	}
	s.showResponse(fixedResponse(), nil)
	if s.responseTab.Peek() != "body" {
		tt.Fatal("unavailable run tab not reset")
	}
}
