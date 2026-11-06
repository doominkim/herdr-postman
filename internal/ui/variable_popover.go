package ui

import (
	"maps"
	"strings"

	t "github.com/darrenburns/terma"
	"github.com/rivo/uniseg"

	"github.com/darrenburns/posting/v3/internal/model"
)

// The variable popover is Postman's variable hover card. Terma reports only
// when the pointer enters or leaves a widget, not where it rests inside a
// text field, so the card follows the text cursor instead: clicking on a
// variable, or moving onto it with the keyboard, shows it below.

const (
	variablePopoverID      = "var-popover"
	variablePopoverEditID  = "var-popover-edit"
	variablePopoverValueID = "var-popover-value"
)

// variableTarget is a variable reference under the cursor of an editor, and
// where its card goes.
type variableTarget struct {
	name   string
	anchor string // the editor's widget ID
	point  t.AnchorPoint
	offset t.Offset
}

type variablePopoverState struct {
	// editing is the variable whose value is being changed, if any. While
	// set, the card stays open even though the editor has lost focus.
	editing t.AnySignal[variableTarget]
	value   *t.TextInputState
}

func newVariablePopoverState() *variablePopoverState {
	return &variablePopoverState{editing: t.NewAnySignal(variableTarget{}), value: t.NewTextInputState("")}
}

// variableAt finds the reference that contains cursor, a grapheme index:
// from its first character up to and including its last. start is the
// grapheme where the reference begins.
func variableAt(graphemes []string, cursor int, native bool) (name string, start int, ok bool) {
	text := strings.Join(graphemes, "")
	refs := model.FindVariables(text)
	if native {
		refs = model.FindPostmanVariables(text)
	}
	index := byteToGrapheme(graphemes, len(text))
	for _, ref := range refs {
		first, end := index[ref.Start], index[ref.End]
		if cursor >= first && cursor < end {
			return ref.Name, first, true
		}
	}
	return "", 0, false
}

// variableTarget is the variable under the cursor of the focused editor of
// the current request: the URL, a table row of params, headers or form
// fields, or the raw body.
func (a *App) variableTarget(ctx t.BuildContext) (variableTarget, bool) {
	focus := ctx.FocusedSignal()
	s := a.session()
	if !focus.IsValid() || s == nil {
		return variableTarget{}, false
	}
	id := focusedIDOf(focus.Get())
	native := a.isNative()
	inInput := func(state *t.TextInputState) (variableTarget, bool) {
		graphemes := state.Content.Get()
		name, start, ok := variableAt(graphemes, state.CursorIndex.Get(), native)
		if !ok {
			return variableTarget{}, false
		}
		// The card starts under the reference. A field scrolled sideways
		// shifts it, which is harmless.
		column := uniseg.StringWidth(strings.Join(graphemes[:start], ""))
		return variableTarget{name: name, anchor: id, point: t.AnchorBottomLeft, offset: t.Offset{X: column}}, true
	}
	switch id {
	case urlInputID:
		return inInput(s.url)
	case "req-body-text":
		name, _, ok := variableAt(s.body.Content.Get(), s.body.CursorIndex.Get(), native)
		if !ok {
			return variableTarget{}, false
		}
		// One cell of padding on the left; the card goes on the next line.
		x, y := s.body.CursorScreenPosition(1, 0)
		return variableTarget{name: name, anchor: id, point: t.AnchorTopLeft, offset: t.Offset{X: x, Y: y + 1}}, true
	}
	for _, editor := range []*kvEditor{s.query, s.headers, s.pathParams, s.form} {
		for _, row := range editor.rows.Get() {
			switch id {
			case editor.inputID(row.id, "key"):
				return inInput(row.key)
			case editor.inputID(row.id, "value"):
				return inInput(row.value)
			}
		}
	}
	return variableTarget{}, false
}

// editableVariable reports whether a value can be set for name. Dynamic
// variables such as {{$guid}} are generated for each request.
func editableVariable(name string) bool { return !strings.HasPrefix(name, "$") }

func (a *App) startVariablePopoverEdit(target variableTarget, value string) {
	p := a.variablePopover
	p.value.SetText(value)
	p.value.CursorEnd()
	p.editing.Set(target)
	t.RequestFocus(variablePopoverValueID)
}

// commitVariableEdit stores the edited value as the session's current value,
// as the Variables dialog does, and returns to the editor.
func (a *App) commitVariablePopoverEdit() {
	p := a.variablePopover
	target := p.editing.Peek()
	if target.name == "" {
		return
	}
	value := p.value.GetText()
	if current, ok := a.variableValuesPeek()[target.name]; !ok || current != value {
		vars := maps.Clone(a.sessionVars.Peek())
		if vars == nil {
			vars = map[string]string{}
		}
		vars[target.name] = value
		a.setSessionVars(vars)
	}
	p.editing.Set(variableTarget{})
	t.RequestFocus(target.anchor)
}

func (a *App) cancelVariablePopoverEdit() {
	p := a.variablePopover
	target := p.editing.Peek()
	p.editing.Set(variableTarget{})
	if target.anchor != "" {
		t.RequestFocus(target.anchor)
	}
}

type variablePopover struct{ app *App }

func (v variablePopover) Build(ctx t.BuildContext) t.Widget {
	a := v.app
	theme := ctx.Theme()
	editing := a.variablePopover.editing.Get()
	target, visible := editing, editing.name != ""
	if !visible && a.postmanUI {
		target, visible = a.variableTarget(ctx)
	}
	config := t.FloatConfig{AnchorID: target.anchor, Anchor: target.point, Offset: target.offset}
	if editing.name != "" {
		// Escape or a click elsewhere abandons the edit.
		config.OnDismiss = a.cancelVariablePopoverEdit
	}
	if !visible {
		return t.Floating{Config: config}
	}

	// Subscribe to the values the card shows.
	a.env.active.Get()
	a.sessionVars.Get()
	var found *model.Variable
	for _, variable := range a.variableList() {
		if variable.Name == target.name {
			found = &variable
			break
		}
	}
	value, source := "", "Not defined"
	if found != nil {
		value, source = found.Value, variableSource(*found)
	}

	hint := source
	if editing.name == "" && editableVariable(target.name) {
		hint += " · click the value to edit"
	} else if editing.name != "" {
		hint = "enter saves · esc cancels"
	}
	var field t.Widget
	switch {
	case editing.name != "":
		field = input{ID: variablePopoverValueID, State: a.variablePopover.value, Placeholder: "value", Width: t.Flex(1), OnSubmit: func(string) { a.commitVariablePopoverEdit() }}
	case !editableVariable(target.name):
		field = t.Text{Content: "Generated for each request", Style: t.Style{ForegroundColor: theme.TextMuted}}
	default:
		shown, color := value, theme.Text
		if found == nil || value == "" {
			shown, color = "Set a value…", theme.TextMuted
		}
		field = t.Text{
			ID:      variablePopoverEditID,
			Content: shown,
			Style:   t.Style{Width: t.Flex(1), ForegroundColor: color, BackgroundColor: theme.Background, Padding: t.EdgeInsetsXY(1, 0)},
			Click:   func(t.MouseEvent) { a.startVariablePopoverEdit(target, value) },
		}
	}
	return t.Floating{
		Visible: true,
		Config:  config,
		Child: t.Column{
			ID: variablePopoverID,
			Style: t.Style{
				Width:           t.Cells(48),
				BackgroundColor: theme.Surface,
				Border:          t.RoundedBorder(theme.Border),
				Padding:         t.EdgeInsetsXY(1, 0),
			},
			Children: []t.Widget{
				t.Text{Content: target.name, Style: t.Style{ForegroundColor: theme.AccentText, Bold: true}},
				field,
				t.Text{Content: hint, Style: t.Style{ForegroundColor: theme.TextMuted}},
			},
		},
	}
}
