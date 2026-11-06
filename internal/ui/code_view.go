package ui

import (
	t "github.com/darrenburns/terma"

	"github.com/darrenburns/posting/v3/internal/model"
)

const (
	codeResolveID     = "req-code-resolve"
	codeURLID         = "req-code-url"
	codeCommandID     = "req-code-command"
	codeCopyURLID     = "req-code-copy-url"
	codeCopyCommandID = "req-code-copy-command"
)

// codePreview is the request as the Code tab shows it.
type codePreview struct {
	url     string
	command string
	// problem says why the command is missing, or why variables were left in.
	problem string
	// note says what Postman adds that the command doesn't show.
	note string
}

// codePreview is s's request as it would be sent: its URL and the
// exportTool command, with the variables' values when resolve is set. When
// they can't be filled in, both are shown with the variables left in.
func (a *App) codePreview(s *Session, resolve bool) codePreview {
	req := s.Snapshot()
	var preview codePreview
	shown, err := a.exportedRequest(req, resolve)
	if err != nil {
		preview.problem = err.Error() + " — showing the request with variables left in"
		shown = req
	}
	preview.url = shown.URL
	if command, err := a.formatExport(shown); err != nil {
		preview.problem = err.Error()
	} else {
		preview.command = command
	}
	switch req.Auth.Type {
	case model.AuthInherit:
		preview.note = "Authorization is inherited from the collection or folder. If they set any, Postman adds it when sending; it isn't shown here."
	case model.AuthManaged:
		preview.note = "Authorization is managed in Postman. Postman adds it when sending; it isn't shown here."
	}
	return preview
}

// codeArea is the read-only view of command, the one the Code tab shows.
// It is replaced when the command changes, which also returns it to the top.
func (s *Session) codeArea(command string) *t.TextAreaState {
	if s.codeView == nil || s.codeCommand != command {
		s.codeView = t.NewTextAreaState(command)
		s.codeView.ReadOnly.Set(true)
		s.codeScroll = t.NewScrollState()
		s.codeCommand = command
	}
	return s.codeView
}

// codeView is the Code tab: the request's URL and its command, as sent, to
// read and copy before sending.
type codeView struct {
	fillParent
	app     *App
	session *Session
}

func (v codeView) Build(ctx t.BuildContext) t.Widget {
	theme := ctx.Theme()
	a, s := v.app, v.session
	resolve := a.curlDialog.resolve.Get()
	// The preview reads the request and the variables without subscribing;
	// these reads rebuild the tab when either changes.
	s.edits.Get()
	a.resolvedVariables()
	preview := a.codePreview(s, resolve)
	tool := a.exportTool()
	inset := t.EdgeInsetsXY(1, 0)
	heading := func(label string, copyID string, copy func()) t.Widget {
		return t.Row{Style: t.Style{Width: t.Flex(1)}, Children: []t.Widget{
			t.Text{Content: label, Style: t.Style{ForegroundColor: theme.Text, Bold: true}},
			t.Spacer{},
			t.Button{ID: copyID, Label: "Copy", OnPress: copy, Click: func(t.MouseEvent) { copy() }},
		}}
	}
	copyURL := func() {
		t.SetClipboard(t.SystemClipboard, preview.url)
		a.notify("Copied URL", toastSuccess)
	}
	copyCommand := func() {
		t.SetClipboard(t.SystemClipboard, preview.command)
		a.notify("Copied "+tool+" command to clipboard", toastSuccess)
	}
	children := []t.Widget{
		tabStrip{
			ID:       codeResolveID,
			Tabs:     []tabItem{{Key: "resolved", Label: "Values"}, {Key: "variables", Label: "Variables"}},
			Active:   map[bool]string{true: "resolved", false: "variables"}[resolve],
			OnChange: func(value string) { a.curlDialog.resolve.Set(value == "resolved") },
		},
		t.Column{Style: t.Style{Width: t.Flex(1)}, Children: []t.Widget{
			heading("Request URL", codeCopyURLID, copyURL),
			t.Column{Style: t.Style{Width: t.Flex(1), BackgroundColor: theme.Surface, Padding: inset}, Children: []t.Widget{
				t.Text{ID: codeURLID, Content: preview.url, Wrap: t.WrapSoft, Style: t.Style{Width: t.Flex(1), ForegroundColor: theme.Text}},
			}},
		}},
	}
	if preview.command != "" {
		state := s.codeArea(preview.command)
		area := t.TextArea{
			ID:          codeCommandID,
			State:       state,
			ScrollState: s.codeScroll,
			Highlighter: a.bodyHighlighter(theme, "bash", false),
			Style:       t.Style{Width: t.Flex(1), BackgroundColor: theme.Surface, Padding: inset},
			MouseUp:     func(t.MouseEvent) { a.copyDragSelection(state) },
		}
		children = append(children, t.Column{Style: t.Style{Width: t.Flex(1), Height: t.Flex(1)}, Children: []t.Widget{
			heading(tool, codeCopyCommandID, copyCommand),
			scrollingArea(codeCommandID, s.codeScroll, theme.Surface, area),
		}})
	}
	if preview.problem != "" {
		children = append(children, t.Text{Content: preview.problem, Wrap: t.WrapSoft, Style: t.Style{Width: t.Flex(1), ForegroundColor: theme.ErrorText}})
	}
	if preview.note != "" {
		children = append(children, t.Text{Content: preview.note, Wrap: t.WrapSoft, Style: t.Style{Width: t.Flex(1), ForegroundColor: theme.TextMuted}})
	}
	return t.Column{Style: t.Style{Width: t.Flex(1), Height: t.Flex(1), Padding: t.EdgeInsetsTRBL(0, 1, 0, 0)}, Spacing: 1, Children: children}
}
