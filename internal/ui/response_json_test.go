package ui

import (
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	t "github.com/darrenburns/terma"

	"github.com/darrenburns/posting/v3/internal/model"
)

// clickTreeRow clicks the response tree row showing label, away from its
// expand indicator.
func clickTreeRow(tt *testing.T, p *t.Pilot, label string) {
	tt.Helper()
	bounds, _ := p.Bounds("resp-body")
	for y, row := range strings.Split(p.TextOf("resp-body"), "\n") {
		if i := strings.Index(row, label); i >= 0 {
			p.ClickAt(bounds.X+utf8.RuneCountInString(row[:i])+1, bounds.Y+y)
			return
		}
	}
	tt.Fatalf("no tree row %q:\n%s", label, p.TextOf("resp-body"))
}

func TestResponseJSONTree(tt *testing.T) {
	const body = `{"page":1,"data":[{"product":{"name":"Ada","id":9007199254740993},"funding":null},{"product":{"name":"Grace","id":2}}]}`
	app, s := bodyApp(tt, "application/json", body)
	p := t.NewPilot(tt, app, snapW, snapH)
	p.Click("resp-body")
	p.Press("g", "/")
	p.Type("Ada")
	p.Press("up")
	searchBounds, _ := p.Bounds(responseSearchID)
	textCursor := s.responseBody.CursorIndex.Peek()
	p.Click(tabID(responseBodyViewID, "tree"))
	got := p.TextOf("resp-body")
	if p.FocusedID() != "resp-body" || !strings.Contains(got, "page: 1") || !strings.Contains(got, "data (2)") {
		tt.Fatalf("JSON tree did not open with focus and counts:\n%s", p.ScreenText())
	}
	if strings.Contains(got, "product") || strings.Contains(got, "$") {
		tt.Fatalf("the tree should open with only the top level showing:\n%s", got)
	}
	if strings.Index(got, "page") > strings.Index(got, "data") {
		tt.Fatalf("keys lost the response's order:\n%s", got)
	}
	if bounds, ok := p.Bounds(responseJSONQueryID); !ok || bounds.Y != searchBounds.Y {
		tt.Fatalf("JSONPath bar is on row %d, want Find's row %d", bounds.Y, searchBounds.Y)
	}
	clickTreeRow(tt, p, "data (2)")
	got = p.TextOf("resp-body")
	if !strings.Contains(got, "[0] {product: {…}, funding: null}") || !strings.Contains(got, "[1] {product: {…}}") || strings.Contains(got, "Ada") {
		tt.Fatalf("clicking a row should open just that level, previewing closed objects:\n%s", got)
	}
	clickTreeRow(tt, p, "[0] {product")
	got = p.TextOf("resp-body")
	if !strings.Contains(got, "[0] {2}") || !strings.Contains(got, `product {name: "Ada", id: 9007199254740993}`) {
		tt.Fatalf("an open object should show its size and its closed children a preview:\n%s", got)
	}
	clickTreeRow(tt, p, "product {name")
	got = p.TextOf("resp-body")
	if !strings.Contains(got, `name: "Ada"`) || !strings.Contains(got, "funding: null") || strings.Index(got, "name") > strings.Index(got, "id:") {
		tt.Fatalf("nested rows are missing or reordered:\n%s", got)
	}
	p.AssertSnapshot("tree", "JSON tree opened by clicking rows, in response order, with counts and a Pretty/Raw/Tree switch")
	clickTreeRow(tt, p, "data (2)")
	if strings.Contains(p.TextOf("resp-body"), "[0]") {
		tt.Fatal("clicking an open row should close it")
	}
	p.Press("home", "down", "right", "down", "y")
	if got := p.Clipboard(); got != "{\n  \"product\": {\n    \"name\": \"Ada\",\n    \"id\": 9007199254740993\n  },\n  \"funding\": null\n}" {
		tt.Fatalf("copy value lost order or number precision: %q", got)
	}
	app.toasts.Clear()
	p.Press("ctrl+f")
	if p.FocusedID() != responseJSONQueryID {
		tt.Fatal("ctrl+f should focus JSONPath in tree mode")
	}
	p.Type("$.data[1].product.name")
	if got := strings.TrimSpace(p.TextOf(responseJSONQueryID + "-count")); got != "1 match" {
		tt.Fatalf("query count = %q", got)
	}
	if got := p.TextOf("resp-body"); !strings.Contains(got, "Grace") || strings.Contains(got, "Ada") || !strings.Contains(got, "[1]") {
		tt.Fatalf("filter lost original indices or included a non-match:\n%s", got)
	}
	p.Press("enter", "end", "y")
	if got := p.Clipboard(); got != `"Grace"` {
		tt.Fatalf("copy value = %q", got)
	}
	p.Press("Y")
	if got := p.Clipboard(); got != "$['data'][1]['product']['name']" {
		tt.Fatalf("copy path = %q", got)
	}
	app.toasts.Clear() // The copy notifications overlay the bottom search bar.
	if got := strings.TrimSpace(p.TextOf("resp-json-path")); got != "$['data'][1]['product']['name']" {
		tt.Fatalf("cursor path = %q", got)
	}
	p.Press("/", "end")
	p.Type("[")
	if !strings.Contains(p.TextOf(responseJSONQueryID+"-error"), "Invalid JSONPath") || !strings.Contains(p.TextOf("resp-body"), "Grace") {
		tt.Fatal("an invalid query should explain that the last valid results remain")
	}
	if bounds, _ := p.Bounds(responseJSONQueryID); bounds.Y != searchBounds.Y {
		tt.Fatal("a query error moved the JSONPath bar away from Find's row")
	}
	p.Press("ctrl+a")
	p.Type("$.missing")
	if !strings.Contains(p.ScreenText(), "No JSONPath matches") || strings.TrimSpace(p.TextOf(responseJSONQueryID+"-count")) != "0 matches" {
		tt.Fatalf("an empty result should be explicit:\n%s", p.ScreenText())
	}
	p.Press("enter", "y")
	if p.Clipboard() != "$['data'][1]['product']['name']" {
		tt.Fatal("copy with no results should leave the clipboard alone")
	}
	p.Press("ctrl+f", "ctrl+a")
	p.Type("$")
	if got := p.TextOf("resp-body"); strings.TrimSpace(p.TextOf(responseJSONQueryID+"-count")) != "1 match" || !strings.Contains(got, "page: 1") || !strings.Contains(got, "data (2)") {
		tt.Fatalf("matching the whole document should show every top-level row:\n%s", got)
	}
	p.Press("ctrl+a")
	p.Type("$.data[*].product.id")
	if got := strings.TrimSpace(p.TextOf(responseJSONQueryID + "-count")); got != "2 matches" {
		tt.Fatalf("query count = %q", got)
	}
	p.Press("enter", "home", "down", "down", "down", "y")
	if got := p.Clipboard(); got != "9007199254740993" {
		tt.Fatalf("copy rounded the JSON number: %q", got)
	}
	p.Press("t")
	if s.responseBody.CursorIndex.Peek() != textCursor || s.responseBody.GetSelectedText() != "Ada" || s.responseBody.SearchQuery.Peek() != "Ada" {
		tt.Fatal("switching views changed the text cursor, selection or search")
	}
	p.Press("y")
	if got := p.Clipboard(); got != "Ada" {
		tt.Fatalf("text view copy = %q", got)
	}
	p.Press("t", "/")
	p.Type("$.data[1].product.name")
	p.Resize(82, snapH)
	t.SetTheme("catppuccin-latte")
	app.toasts.Clear()
	p.AssertSnapshot("filtered_light", "Narrow light-theme JSONPath results retain ancestors and the original array index")
	p.Click(tabID(responseBodyViewID, "pretty"))
	if p.FocusedID() != "resp-body" || !strings.Contains(p.TextOf("resp-body"), `"data": [`) {
		tt.Fatal("clicking Pretty should restore the focused text body")
	}
}

func TestResponseBodyRawAndPretty(tt *testing.T) {
	const body = `{"page":1,"data":[{"id":2}]}`
	// Misleading content types still offer the JSON views.
	app, s := bodyApp(tt, "text/plain", body)
	p := t.NewPilot(tt, app, snapW, snapH)
	if got := p.TextOf("resp-body"); !strings.Contains(got, `"page": 1`) {
		tt.Fatalf("JSON should open pretty-printed:\n%s", got)
	}
	p.Click(tabID(responseBodyViewID, "raw"))
	if got := strings.TrimSpace(p.TextOf("resp-body")); got != body || p.FocusedID() != "resp-body" {
		tt.Fatalf("Raw should show the body exactly as received:\n%s", got)
	}
	app.copyResponseBody()
	if got := p.Clipboard(); got != body {
		tt.Fatalf("copying the whole body in Raw = %q", got)
	}
	p.Press("t", "t")
	if got := strings.TrimSpace(p.TextOf("resp-body")); got != body {
		tt.Fatalf("leaving the tree should return to Raw:\n%s", got)
	}
	resp := fixedResponse()
	resp.Body = []byte(`{"next":true}`)
	s.showResponse(resp, nil)
	if got := strings.TrimSpace(p.TextOf("resp-body")); got != `{"next":true}` {
		tt.Fatalf("the next response should stay in Raw:\n%s", got)
	}
	p.Click(tabID(responseBodyViewID, "pretty"))
	if got := p.TextOf("resp-body"); !strings.Contains(got, `"next": true`) {
		tt.Fatalf("Pretty should indent again:\n%s", got)
	}
	resp.Body = []byte("plain text")
	s.showResponse(resp, nil)
	if _, ok := p.Bounds(responseBodyViewID); ok {
		tt.Fatal("a body that isn't JSON has no Pretty/Raw/Tree switch")
	}
}

func TestResponseJSONReplacementAndSessions(tt *testing.T) {
	app, first := bodyApp(tt, "application/problem+json", `{"old":"first"}`)
	p := t.NewPilot(tt, app, snapW, snapH)
	p.Click(tabID(responseBodyViewID, "tree"))
	p.Press("/")
	p.Type("$.old")
	p.Press("enter")
	second := app.openSession(sampleRequest(tt, "Create user"))
	second.showResponse(fixedResponse(), nil)
	second.phase.Set(exchangeDone)
	t.RequestFocus("resp-body")
	p.Advance(0)
	if _, ok := p.Bounds(responseJSONQueryID); !ok || strings.Contains(p.TextOf(responseJSONQueryID), "$.old") {
		tt.Fatal("a new request tab should open in the tree view, without another tab's query")
	}
	p.Press("alt+left")
	if app.current() != first || !strings.Contains(p.TextOf(responseJSONQueryID), "$.old") {
		tt.Fatal("returning to a session lost its JSON query")
	}
	resp := fixedResponse()
	resp.Body = []byte(`{"new":"replacement"}`)
	first.showResponse(resp, nil)
	p.Advance(0)
	if got := p.TextOf("resp-body"); !strings.Contains(got, "replacement") || strings.Contains(got, "first") {
		tt.Fatalf("replacement response shows stale or filtered data:\n%s", got)
	}
	for _, body := range []string{`{"broken":`, `{} {}`, "not JSON", ""} {
		resp.Body = []byte(body)
		first.showResponse(resp, nil)
		p.Advance(0)
		if _, ok := p.Bounds(tabID(responseBodyViewID, "tree")); ok {
			tt.Fatalf("offered a JSON tree for %q", body)
		}
		if _, ok := p.Bounds(responseJSONQueryID); ok {
			tt.Fatalf("left the old JSON view visible for %q", body)
		}
		if body != "" && !strings.Contains(p.TextOf("resp-body"), body) {
			tt.Fatalf("fallback lost the raw response %q", body)
		}
	}
	// History and misleading/missing content types use the same viewer, and
	// scalar JSON (including null) is not mistaken for an invalid document.
	for _, body := range []string{"null", "false", `"scalar"`, "[]", "{}"} {
		resp.Body, resp.Headers = []byte(body), nil
		entry := sentEntry(1, sampleRequest(tt, "List users"), resp, time.Now())
		first.showResponse(resp, &entry)
		p.Advance(0)
		p.Click(tabID(responseBodyViewID, "tree"))
		p.Press("home", "y")
		if got := p.Clipboard(); got != body {
			tt.Fatalf("history scalar copy = %q, want %q", got, body)
		}
		app.toasts.Clear()
		p.Press("t")
	}
}

func TestResponseJSONObjectPreview(tt *testing.T) {
	state, err := newJSONTreeState([]byte(`[{"id":1,"description":"a description far too long to show in full here","tags":["a"],"meta":{},"list":[],"more":true}, {"a b ":1}, {}]`))
	if err != nil {
		tt.Fatal(err)
	}
	rows := state.Tree.Nodes.Peek()
	for _, c := range []struct {
		width int
		want  string
	}{
		{56, `{id: 1, description: "a description far…", tags: […], …}`},
		{55, `{id: 1, description: "a description far…", …}`},
		{200, `{id: 1, description: "a description far…", tags: […], meta: {}, list: [], more: true}`},
		{6, `{…}`},
	} {
		got := rows[0].Data.preview(c.width)
		if got != c.want || utf8.RuneCountInString(got) > max(c.width, 3) {
			tt.Fatalf("preview in %d cells = %q, want %q", c.width, got, c.want)
		}
	}
	if got := rows[1].Data.preview(60); got != `{"a b ": 1}` {
		tt.Fatalf("awkward keys should be quoted in previews too: %q", got)
	}
	if got := rows[2].Data.preview(60); got != "" {
		tt.Fatalf("an empty object has nothing to preview: %q", got)
	}
}

func TestResponseJSONPreviewFillsTheRow(tt *testing.T) {
	const body = `{"data":[{"id":1,"name":"Ada","email":"ada@example.com","role":"admin","team":"engines","city":"London","active":true,"score":42,"joined":"1843-07-10","notes":null}]}`
	app, _ := bodyApp(tt, "application/json", body)
	p := t.NewPilot(tt, app, snapW, snapH)
	p.Click(tabID(responseBodyViewID, "tree"))
	clickTreeRow(tt, p, "data (1)")
	bounds, _ := p.Bounds("resp-body")
	for _, row := range strings.Split(p.TextOf("resp-body"), "\n") {
		if i := strings.Index(row, "[0] {"); i >= 0 {
			preview := row[i+len("[0] "):]
			if n := utf8.RuneCountInString(preview); n <= 60 || utf8.RuneCountInString(row) > bounds.Width || !strings.HasSuffix(preview, "…}") {
				tt.Fatalf("preview should fill the row (%d cells) and end with …}: %q", bounds.Width, row)
			}
			return
		}
	}
	tt.Fatalf("no [0] row:\n%s", p.TextOf("resp-body"))
}

func TestResponseViewIsSharedAndRemembered(tt *testing.T) {
	const body = `{"page":1}`
	var remembered []ResponseView
	app := New(Config{RememberResponseView: func(view ResponseView) { remembered = append(remembered, view) }})
	show := func(s *Session, body string) {
		resp := fixedResponse()
		resp.Body = []byte(body)
		s.showResponse(resp, nil)
		s.phase.Set(exchangeDone)
	}
	first := app.openSession(sampleRequest(tt, "List users"))
	show(first, body)
	p := t.NewPilot(tt, app, snapW, snapH)
	p.Click(tabID(responseBodyViewID, "raw"))
	second := app.openSession(sampleRequest(tt, "Create user"))
	show(second, body)
	if got := strings.TrimSpace(p.TextOf("resp-body")); got != body {
		tt.Fatalf("a new tab should open in Raw too:\n%s", got)
	}
	p.Click(tabID(responseBodyViewID, "pretty"))
	p.Press("alt+left")
	if app.current() != first || !strings.Contains(p.TextOf("resp-body"), `"page": 1`) {
		tt.Fatalf("the other tab should follow the switch to Pretty:\n%s", p.TextOf("resp-body"))
	}
	p.Click(tabID(responseBodyViewID, "tree"))
	show(first, "plain text")
	if !strings.Contains(p.TextOf("resp-body"), "plain text") {
		tt.Fatal("a body that isn't JSON should still show as text")
	}
	show(first, body)
	if !strings.Contains(p.TextOf("resp-body"), "page: 1") {
		tt.Fatalf("the tree should come back for the next JSON body:\n%s", p.TextOf("resp-body"))
	}
	want := []ResponseView{{Pretty: false}, {Pretty: true}, {Pretty: true, Tree: true}}
	if !slices.Equal(remembered, want) {
		tt.Fatalf("remembered %v, want %v", remembered, want)
	}
}

func TestResponseViewRestoredOnStart(tt *testing.T) {
	const body = `{"page":1}`
	app := New(Config{ResponseView: &ResponseView{Pretty: false, Tree: true}})
	s := app.openSession(sampleRequest(tt, "List users"))
	resp := fixedResponse()
	resp.Body = []byte(body)
	s.showResponse(resp, nil)
	s.phase.Set(exchangeDone)
	p := t.NewPilot(tt, app, snapW, snapH)
	if !strings.Contains(p.TextOf("resp-body"), "page: 1") {
		tt.Fatalf("a restart should open in the remembered view:\n%s", p.ScreenText())
	}
	p.Click("resp-body")
	p.Press("t")
	if got := strings.TrimSpace(p.TextOf("resp-body")); got != body {
		tt.Fatalf("leaving the remembered tree should return to Raw:\n%s", got)
	}
}

func TestResponseJSONStreamInspector(tt *testing.T) {
	app, s, updates := streamApp(tt)
	tt.Cleanup(s.Stop)
	app.toggleStream()
	drainUntil(tt, updates, func() bool { return s.streaming.Peek() == streamOpen && bodyHas(s, "hi") })
	p := t.NewPilot(tt, app, snapW, snapH)
	p.Click(messagesID)
	p.Press("end", "up", "enter", "t", "/")
	p.Type("$.text")
	p.Press("enter", "end", "y")
	if got := p.Clipboard(); got != `"hi"` {
		tt.Fatalf("message copy = %q:\n%s", got, p.ScreenText())
	}
	s.payloads[model.KindGRPC].(*grpcEditor).message.SetText(`{"text":"again"}`)
	app.send()
	drainUntil(tt, updates, func() bool { return bodyHas(s, "again") })
	p.Advance(0)
	p.Press("y")
	if got := p.Clipboard(); got != `"hi"` || !strings.Contains(p.TextOf(responseJSONQueryID), "$.text") {
		tt.Fatal("arriving messages changed the inspected JSON or its filter")
	}
	p.Press("escape", "escape") // Clear the query, then return to the log.
	if p.FocusedID() != messagesID || s.phase.Peek() != exchangeSending {
		tt.Fatal("escape should return to the log without cancelling the stream")
	}
	p.Press("end", "enter", "end", "y")
	if got := p.Clipboard(); got != `"again"` {
		tt.Fatalf("newly selected message copy = %q", got)
	}
}
