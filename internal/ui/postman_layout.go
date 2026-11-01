package ui

import (
	"github.com/darrenburns/posting/v3/internal/model"
	t "github.com/darrenburns/terma"
	"strings"
)

func registerPostmanTheme() {
	theme := t.ExtendTheme(t.ThemeNameGalaxy,
		t.WithPrimary(t.Hex("#097bed")), t.WithSecondary(t.Hex("#ff6c37")), t.WithAccent(t.Hex("#ff6c37")),
		t.WithBackground(t.Hex("#212121")), t.WithSurface(t.Hex("#262626")), t.WithSurface2(t.Hex("#303030")),
		t.WithText(t.Hex("#e6e6e6")), t.WithTextMuted(t.Hex("#a6a6a6")), t.WithBorder(t.Hex("#3b3b3b")),
		t.WithSuccess(t.Hex("#73c991")), t.WithWarning(t.Hex("#e5b95c")), t.WithError(t.Hex("#f47067")), t.WithInfo(t.Hex("#75a7ff")))
	theme.ActiveCursor = t.Hex("#ff6c37")
	theme.Selection = t.Hex("#3b3b3b")
	theme.SelectionText = t.Hex("#171717")
	theme.Cursor = t.Hex("#e6e6e6")
	theme.TextOnPrimary = t.Hex("#ffffff")
	theme.FocusRing = t.Hex("#ff6c37")
	theme.Link = t.Hex("#75a7ff")
	theme.Name = "postman-dark"
	t.RegisterTheme(theme.Name, theme)
}

type postmanRequestHeading struct {
	fillWidth
	app     *App
	session *Session
}

func (h postmanRequestHeading) Build(ctx t.BuildContext) t.Widget {
	name := h.session.name.GetText()
	if name == "" {
		name = "Untitled Request"
	}
	folder := ""
	h.app.collection.Get().Walk(func(c *model.Collection, r model.Request) {
		if r.File == h.session.file.Get() {
			folder = c.Name
		}
	})
	if folder != "" {
		name = folder + " / " + name
	}
	if h.session.dirty.Get() {
		name += " •"
	}
	return t.Row{Style: t.Style{Height: t.Cells(1), Width: t.Flex(1), Padding: t.EdgeInsetsXY(1, 0)}, Children: []t.Widget{
		t.Text{Content: name, Style: t.Style{Width: t.Flex(1), ForegroundColor: ctx.Theme().Text}},
		t.Button{ID: "postman-save", Label: "Save", DisableFocus: true, OnPress: h.app.saveRequest},
	}}
}

type postmanFooter struct{ app *App }

func (f postmanFooter) Build(ctx t.BuildContext) t.Widget {
	return t.Row{Style: t.Style{Width: t.Flex(1), Height: t.Cells(1), Padding: t.EdgeInsetsXY(1, 0), BackgroundColor: ctx.Theme().Surface}, Children: []t.Widget{
		t.Text{Content: f.app.appName + " " + f.app.version, Style: t.Style{ForegroundColor: ctx.Theme().TextMuted}},
		t.Spacer{},
		t.Button{ID: "postman-variables", Label: "Variables", DisableFocus: true, OnPress: f.app.openVariables},
		t.Button{ID: "postman-layout", Label: "Layout", DisableFocus: true, OnPress: f.app.toggleLayout},
		t.Button{ID: "postman-commands", Label: "Commands", DisableFocus: true, OnPress: f.app.openPalette},
		t.Button{ID: "postman-question", Label: "Question", DisableFocus: true, OnPress: func() { f.app.openLink("Question", "https://github.com/doominkim/herdr-postman/issues") }},
	}}
}

// postmanPanel puts editor tabs at the left and response metadata at the right.
func postmanPanel(theme t.ThemeData, title string, tabs *tabStrip, info []t.Text, content t.Widget) t.Widget {
	heading := []t.Widget{}
	if tabs != nil {
		heading = append(heading, *tabs)
	} else {
		heading = append(heading, t.Text{Content: title, Style: t.Style{ForegroundColor: theme.TextMuted}})
	}
	if tabs == nil {
		heading = append(heading, t.Spacer{})
	}
	for _, part := range info {
		heading = append(heading, part)
	}
	return t.Column{Style: t.Style{Width: t.Flex(1), Height: t.Flex(1)}, Children: []t.Widget{
		t.Row{Style: t.Style{Width: t.Flex(1), Height: t.Cells(1), BackgroundColor: theme.Surface}, Spacing: 1, Children: heading}, content,
	}}
}

func postmanMethodColor(theme t.ThemeData, m model.Method) t.Color {
	switch m {
	case model.MethodGet:
		return theme.SuccessText
	case model.MethodPost:
		return theme.WarningText
	case model.MethodPut, model.MethodPatch:
		return theme.InfoText
	case model.MethodDelete:
		return theme.ErrorText
	default:
		return theme.TextMuted
	}
}
func isPostmanTheme(theme t.ThemeData) bool { return strings.HasPrefix(theme.Name, "postman-") }
