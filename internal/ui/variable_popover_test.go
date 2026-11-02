package ui

import (
	"strings"
	"testing"

	t "github.com/darrenburns/terma"
	"github.com/rivo/uniseg"

	"github.com/darrenburns/posting/v3/internal/model"
)

func graphemesOf(s string) []string {
	var out []string
	g := uniseg.NewGraphemes(s)
	for g.Next() {
		out = append(out, g.Str())
	}
	return out
}

func TestVariableAtCursorBoundaries(tt *testing.T) {
	for _, tc := range []struct {
		text   string
		native bool
		cursor int
		want   string
	}{
		{"a{{id}}b", true, 0, ""},
		{"a{{id}}b", true, 1, "id"}, // on the first brace
		{"a{{id}}b", true, 6, "id"}, // on the last brace
		{"a{{id}}b", true, 7, ""},   // just after the reference
		{"é{{id}}", true, 1, "id"},  // grapheme, not byte, positions
		{"${ID}/x", false, 0, "ID"}, // legacy references
		{"${ID}/x", false, 4, "ID"}, // on the closing brace
		{"${ID}/x", false, 5, ""},   // after it
		{"{{open", true, 3, ""},     // unterminated
	} {
		name, _, ok := variableAt(graphemesOf(tc.text), tc.cursor, tc.native)
		if tc.want == "" && ok || tc.want != "" && name != tc.want {
			tt.Errorf("variableAt(%q, %d) = %q %v, want %q", tc.text, tc.cursor, name, ok, tc.want)
		}
	}
}

func popoverApp(tt *testing.T) (*App, *Session, *t.Pilot) {
	tt.Helper()
	app := New(Config{AppName: "herdr-postman", PostmanUI: true, Collection: model.SampleCollection(), Sender: &countingSender{}})
	tt.Cleanup(func() { app.Close() })
	app.openRequest(sampleRequest(tt, "List users"))
	s := app.current()
	s.url.SetText("${BASE_URL}/products/${PRODUCT_ID}")
	p := t.NewPilot(tt, app, 150, 45)
	return app, s, p
}

func TestVariablePopoverEditsTheValueUnderTheCursor(tt *testing.T) {
	app, s, p := popoverApp(tt)
	p.Click(urlInputID)
	s.url.CursorIndex.Set(len(graphemesOf("${BASE_URL}/products/${PROD")))
	p.Advance(0)
	if _, ok := p.Bounds(variablePopoverID); !ok {
		tt.Fatalf("no popover with the cursor inside ${PRODUCT_ID}:\n%s", p.ScreenText())
	}
	if text := p.TextOf(variablePopoverID); !strings.Contains(text, "PRODUCT_ID") || !strings.Contains(text, "Not defined") {
		tt.Fatalf("popover = %q", text)
	}
	p.Click(variablePopoverEditID)
	if p.FocusedID() != variablePopoverValueID {
		tt.Fatalf("clicking the value focused %q", p.FocusedID())
	}
	p.Type("42")
	p.Press("enter")
	if got := app.sessionVars.Peek()["PRODUCT_ID"]; got != "42" {
		tt.Fatalf("PRODUCT_ID = %q after enter", got)
	}
	if p.FocusedID() != urlInputID {
		tt.Fatalf("focus after commit = %q, want the URL", p.FocusedID())
	}
	if text := p.TextOf(variablePopoverID); !strings.Contains(text, "42") {
		tt.Fatalf("popover after commit = %q", text)
	}

	s.url.CursorIndex.Set(len(graphemesOf("${BASE_URL}/prod")))
	p.Advance(0)
	if _, ok := p.Bounds(variablePopoverID); ok {
		tt.Fatal("popover stays with the cursor outside any variable")
	}
}

func TestVariablePopoverEscapeKeepsTheValue(tt *testing.T) {
	app, s, p := popoverApp(tt)
	app.setSessionVars(map[string]string{"PRODUCT_ID": "7"})
	p.Click(urlInputID)
	s.url.CursorIndex.Set(len(graphemesOf("${BASE_URL}/products/${P")))
	p.Advance(0)
	p.Click(variablePopoverEditID)
	p.Press("ctrl+a")
	p.Type("99")
	p.Press("escape")
	if got := app.sessionVars.Peek()["PRODUCT_ID"]; got != "7" {
		tt.Fatalf("escape changed PRODUCT_ID to %q", got)
	}
	if p.FocusedID() != urlInputID {
		tt.Fatalf("focus after escape = %q", p.FocusedID())
	}
}

func TestVariablePopoverFollowsTheBodyCursor(tt *testing.T) {
	_, s, p := popoverApp(tt)
	s.requestTab.Set("body")
	s.bodyType.Set(model.BodyRaw)
	s.body.SetText(`{"id": "${PRODUCT_ID}"}`)
	p.Advance(0)
	p.Click("req-body-text")
	s.body.CursorIndex.Set(len(graphemesOf(`{"id": "${PRO`)))
	p.Advance(0)
	if text := p.TextOf(variablePopoverID); !strings.Contains(text, "PRODUCT_ID") {
		tt.Fatalf("body popover = %q\n%s", text, p.ScreenText())
	}
}
