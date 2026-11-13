package ui

import (
	"fmt"

	t "github.com/darrenburns/terma"
)

const (
	responseJSONQueryID = "resp-jsonpath"
	// responseBodyViewID is the Pretty/Raw/Tree switch for JSON bodies.
	responseBodyViewID = "resp-body-view"
)

// A new body starts with an unfiltered tree. Invalid JSON must discard the
// old tree rather than leave another response's data on screen. Detect JSON
// from the body, including when the server omitted or mislabelled its type.
func (s *Session) setResponseJSON(body []byte) {
	state, _ := newJSONTreeState(body)
	s.responseJSON.Set(state)
	s.responseJSONQuery.SetText("")
	s.responseJSONError.Set("")
}

// ResponseView is how JSON response bodies are shown. Every request tab
// shares it, and it is remembered between runs.
type ResponseView struct {
	// Pretty indents the text view; otherwise it shows the body as received.
	Pretty bool `json:"pretty"`
	// Tree shows JSON bodies as a tree instead of text. Bodies that aren't
	// JSON still show as text.
	Tree bool `json:"tree"`
}

// key is the Pretty/Raw/Tree switch's key for the view.
func (v ResponseView) key() string {
	switch {
	case v.Tree:
		return "tree"
	case v.Pretty:
		return "pretty"
	}
	return "raw"
}

// setResponseView changes how every tab shows JSON bodies, and remembers it.
// s is the tab where it changed, which keeps focus on its body.
func (a *App) setResponseView(s *Session, view ResponseView) {
	old := a.responseView.Peek()
	a.responseView.Set(view)
	if view.Pretty != old.Pretty {
		for _, session := range a.sessions.Peek() {
			session.reformatJSONBody(view.Pretty)
		}
	}
	if view != old && a.rememberResponseView != nil {
		a.rememberResponseView(view)
	}
	s.messages.following.Set(false)
	t.RequestFocus("resp-body")
}

// reformatJSONBody shows a JSON body's text pretty-printed or as received,
// from the top.
func (s *Session) reformatJSONBody(pretty bool) {
	state := s.responseJSON.Peek()
	if state == nil {
		return
	}
	s.responseBody.SetText(formatJSON(state.body, pretty))
	s.responseBody.CursorIndex.Set(0)
	s.responseBody.ClearSelection()
	s.responseBodyScroll.SetOffset(0)
	s.responseVisual.Set(false)
}

func (a *App) toggleBodyView(s *Session) {
	view := a.responseView.Peek()
	view.Tree = !view.Tree
	a.setResponseView(s, view)
}

// bodyViewSwitch switches JSON bodies between Pretty, Raw and Tree.
func (a *App) bodyViewSwitch(s *Session) tabStrip {
	return tabStrip{
		ID:     responseBodyViewID,
		Tabs:   []tabItem{{Key: "pretty", Label: "Pretty"}, {Key: "raw", Label: "Raw"}, {Key: "tree", Label: "Tree"}},
		Active: a.responseView.Get().key(),
		OnChange: func(key string) {
			view := a.responseView.Peek()
			view.Tree = key == "tree"
			if !view.Tree {
				view.Pretty = key == "pretty"
			}
			a.setResponseView(s, view)
		},
		Up: func() { t.RequestFocus("resp-body") },
	}
}

// responseJSON is the optional tree view of the same body used by the text
// viewer and external tools.
type responseJSON struct {
	fillParent
	app     *App
	session *Session
	state   *jsonTreeState
}

func (r responseJSON) focusQuery() {
	r.session.responseJSONQuery.SelectAll()
	t.RequestFocus(responseJSONQueryID)
}

func (r responseJSON) setQuery(query string) {
	message := ""
	if err := r.state.SetQuery(query); err != nil {
		message = "Invalid JSONPath · showing last valid results"
	}
	r.session.responseJSONError.Set(message)
}

func (r responseJSON) clearQuery() {
	r.session.responseJSONQuery.SetText("")
	r.setQuery("")
	t.RequestFocus("resp-body")
}

func (r responseJSON) copy(path bool) {
	node, ok := r.state.Tree.CursorNode()
	if !ok {
		return
	}
	text, message := node.Path, "Copied JSONPath"
	if !path {
		text, message = node.copyText(), "Copied JSON value"
	}
	t.SetClipboard(t.SystemClipboard, text)
	r.app.notify(message, toastSuccess)
}

func (r responseJSON) Keybinds() []t.Keybind {
	binds := []t.Keybind{
		{Key: "ctrl+f", Name: "JSONPath", Action: r.focusQuery},
		{Key: "/", Action: r.focusQuery, Hidden: true},
		{Key: "y", Name: "Copy value", Action: func() { r.copy(false) }},
		{Key: "c", Action: func() { r.copy(false) }, Hidden: true},
		{Key: "Y", Name: "Copy path", Action: func() { r.copy(true) }},
		{Key: "e", Name: "Expand all", Action: r.state.expandAll},
		{Key: "E", Name: "Collapse all", Action: r.state.collapseAll},
	}
	if r.session.responseJSONQuery.GetText() != "" {
		binds = append(binds, t.Keybind{Key: "escape", Name: "Clear filter", Action: r.clearQuery})
	}
	return append(binds, r.app.externalKeybinds(r.session.responseBody, "json", nil)...)
}

func (r responseJSON) Build(ctx t.BuildContext) t.Widget {
	s, theme := r.session, ctx.Theme()
	count := ""
	color := theme.TextMuted
	if r.state.Query() != "" {
		count = fmt.Sprintf("%d matches", r.state.MatchCount())
		if r.state.MatchCount() == 1 {
			count = "1 match"
		}
		if r.state.MatchCount() == 0 {
			color = theme.WarningText
		}
	}
	query := t.Row{
		Style:   t.Style{Width: t.Flex(1), Height: t.Cells(1), BackgroundColor: theme.Surface, Padding: inset},
		Spacing: 1,
		Children: []t.Widget{
			t.Text{Content: "JSONPath", Style: t.Style{ForegroundColor: theme.AccentText}},
			t.TextInput{
				ID: responseJSONQueryID, State: s.responseJSONQuery,
				Placeholder: "e.g. $.users[*]", Style: t.Style{Width: t.Flex(1), BackgroundColor: theme.Surface},
				OnChange:      r.setQuery,
				OnSubmit:      func(string) { t.RequestFocus("resp-body") },
				ExtraKeybinds: []t.Keybind{{Key: "escape", Name: "Clear filter", Action: r.clearQuery}},
			},
			t.Text{ID: responseJSONQueryID + "-count", Content: count, Style: t.Style{ForegroundColor: color}},
			t.Text{ID: responseJSONQueryID + "-clear", Content: "×", Style: t.Style{ForegroundColor: theme.TextMuted}, Click: func(t.MouseEvent) { r.clearQuery() }},
		},
	}
	children := []t.Widget{r.tree(ctx)}
	if message := s.responseJSONError.Get(); message != "" {
		children = append(children, t.Text{
			ID: responseJSONQueryID + "-error", Content: message,
			Style: t.Style{Width: t.Flex(1), Height: t.Cells(1), ForegroundColor: theme.ErrorText, Padding: inset},
		})
	}
	children = append(children,
		query,
		responseJSONStatus{view: r},
	)
	return t.Column{Style: t.Style{Width: t.Flex(1), Height: t.Flex(1)}, Children: children}
}

// tree lists the body's values as "key (2)" for arrays, "key {3}" for open
// objects, a preview such as "key {id: 1, …}" for closed ones and
// "key: value" otherwise. Clicking a row opens or closes it.
func (r responseJSON) tree(ctx t.BuildContext) t.Widget {
	theme, state := ctx.Theme(), r.state
	tree := t.Tree[jsonNode]{
		ID:              "resp-body",
		State:           state.Tree,
		NodeID:          func(node jsonNode) string { return node.Path },
		ScrollState:     state.scroll,
		Style:           t.Style{Width: t.Flex(1)},
		ActivateOnClick: true,
		OnSelect:        func(jsonNode, []jsonNode) { state.toggleCursor() },
	}
	focused := ctx.IsFocused(tree)
	tree.RenderNode = func(node jsonNode, nodeCtx t.TreeNodeContext) t.Widget {
		style := t.Style{Width: t.Flex(1), ForegroundColor: theme.Text}
		separator, value, color := jsonNodeSummary(node, theme)
		if !nodeCtx.Expanded && node.Preview != "" {
			value = node.Preview
		}
		keyColor, separatorColor := theme.Primary, theme.TextMuted
		if node.ancestor {
			keyColor, color = theme.TextMuted, theme.TextMuted
		}
		switch {
		case nodeCtx.Active && focused:
			style.BackgroundColor, style.ForegroundColor = theme.ActiveCursor, theme.SelectionText
			keyColor, separatorColor, color = theme.SelectionText, theme.SelectionText, theme.SelectionText
		case nodeCtx.Selected:
			style.BackgroundColor = theme.Selection
		}
		return t.Text{Style: style, Spans: []t.Span{
			{Text: node.Label, Style: t.SpanStyle{Foreground: keyColor}},
			{Text: separator, Style: t.SpanStyle{Foreground: separatorColor}},
			{Text: value, Style: t.SpanStyle{Foreground: color}},
		}}
	}
	var child t.Widget = tree
	if len(state.Tree.Nodes.Get()) == 0 {
		// Keep the empty tree focusable, so clearing the query restores navigation.
		child = t.Column{Style: t.Style{Width: t.Flex(1)}, Children: []t.Widget{tree, t.Text{
			Content: "No JSONPath matches",
			Style:   t.Style{ForegroundColor: theme.TextMuted},
		}}}
	}
	return t.Scrollable{
		State:               state.scroll,
		Style:               t.Style{Width: t.Flex(1), Height: t.Flex(1)},
		ScrollbarThumbColor: theme.ScrollbarThumb,
		ScrollbarTrackColor: theme.ScrollbarTrack,
		Child:               child,
	}
}

// Only the status row needs to rebuild when the cursor's path changes.
type responseJSONStatus struct {
	fillWidth
	view responseJSON
}

func (status responseJSONStatus) Build(ctx t.BuildContext) t.Widget {
	r := status.view
	theme := ctx.Theme()
	r.state.Tree.CursorPath.Get()
	node, _ := r.state.Tree.CursorNode()
	return t.Row{
		Style:   t.Style{Width: t.Flex(1), Height: t.Cells(1), BackgroundColor: theme.Surface, Padding: inset},
		Spacing: 2,
		Children: []t.Widget{
			r.app.bodyViewSwitch(r.session),
			t.Text{ID: "resp-json-path", Content: node.Path, Style: t.Style{Width: t.Flex(1), ForegroundColor: theme.TextMuted}},
			t.Text{Content: "copy value", Style: t.Style{ForegroundColor: theme.TextMuted}, Click: func(t.MouseEvent) { r.copy(false) }},
		},
	}
}
