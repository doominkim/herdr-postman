package ui

import (
	"strings"

	t "github.com/darrenburns/terma"
	uv "github.com/darrenburns/ultraviolet"

	"github.com/darrenburns/posting/v3/internal/model"
)

const (
	postmanTitleID  = "postman-title"
	postmanRenameID = "postman-rename"
	requestMenuID   = "request-menu"
)

// requestActions holds the Postman layout's in-place rename of the request
// title and the collection's right-click menu.
type requestActions struct {
	// renaming is the tab whose title is being edited, or nil.
	renaming t.AnySignal[*Session]
	name     *t.TextInputState
	// saving is the tab whose new name is being saved, or nil. A Cloud save
	// goes through the Postman CLI and takes seconds.
	saving t.AnySignal[*Session]
	// pending is the name being saved.
	pending string
	// menuAt is where the right-click menu is open; menu holds its items.
	menuAt t.AnySignal[*t.Offset]
	menu   *t.MenuState
}

func newRequestActions() *requestActions {
	return &requestActions{
		renaming: t.NewAnySignal[*Session](nil),
		name:     t.NewTextInputState(""),
		saving:   t.NewAnySignal[*Session](nil),
		menuAt:   t.NewAnySignal[*t.Offset](nil),
		menu:     t.NewMenuState(nil),
	}
}

func (a *App) startRename(s *Session) {
	r := a.requestActions
	r.name.SetText(strings.TrimSpace(s.name.GetText()))
	r.name.CursorEnd()
	r.renaming.Set(s)
	t.RequestFocus(postmanRenameID)
}

func (a *App) cancelRename() {
	a.requestActions.renaming.Set(nil)
	t.RequestFocus(urlInputID)
}

func (a *App) commitRename() {
	r := a.requestActions
	s := r.renaming.Peek()
	if s == nil {
		return
	}
	r.renaming.Set(nil)
	t.RequestFocus(urlInputID)
	a.renameSession(s, strings.TrimSpace(r.name.GetText()))
}

// renameSession changes only the name of the request in s. A saved request
// is saved straight away under the new name, as Postman does; one with other
// unsaved edits is left alone, since saving would store those too. The save
// runs in the background so a slow Cloud save doesn't freeze the interface.
func (a *App) renameSession(s *Session, name string) {
	if name == "" || name == strings.TrimSpace(s.name.GetText()) {
		return
	}
	file := s.file.Peek()
	if file == "" {
		s.name.SetText(name)
		s.touch()
		return
	}
	if s.dirty.Peek() {
		a.notify("Save or discard the changes to this request before renaming it", toastWarning)
		return
	}
	r := a.requestActions
	if r.saving.Peek() != nil {
		a.notify("Still saving the previous name", toastWarning)
		return
	}
	saved, ok := a.savedRequest(file)
	if !ok {
		a.notify("Couldn't find "+file+" in the collection", toastError)
		return
	}
	renamed := saved.Clone()
	renamed.Name = name
	r.pending = name
	r.saving.Set(s)
	store := a.store
	go func() {
		err := store.Save(renamed)
		t.Dispatch(func() {
			r.saving.Set(nil)
			if err != nil {
				a.notify("Couldn't rename "+file+": "+err.Error(), toastError)
				return
			}
			a.recordRequest(renamed)
			if s.dirty.Peek() {
				// Edited while saving: keep those edits unsaved.
				s.syncing = true
				s.name.ReplaceText(name, s.name.CursorIndex.Peek())
				s.syncing = false
				s.title.Set(renamed.DisplayName())
			} else {
				s.savedAs(renamed)
			}
			a.notify("Renamed to "+name, toastSuccess)
		})
	}()
}

// savedRequest is the collection's copy of the request saved in file.
func (a *App) savedRequest(file string) (model.Request, bool) {
	var found model.Request
	ok := false
	a.collection.Peek().Walk(func(_ *model.Collection, r model.Request) {
		if r.File == file {
			found, ok = r, true
		}
	})
	return found, ok
}

// openRequestMenu shows the right-click menu for req at the pointer.
func (a *App) openRequestMenu(req model.Request, x, y int) {
	r := a.requestActions
	r.menu = t.NewMenuState([]t.MenuItem{
		{Label: "Rename", Action: func() {
			a.openRequest(req)
			if s := a.sessionForFile(req.File); s != nil {
				a.keepSession(s.id)
				a.startRename(s)
			}
		}},
		{Label: "Duplicate", Action: func() { a.duplicateRequests([]model.Request{req}) }},
		{Label: "Delete", Action: func() { a.confirmDeleteRequests([]model.Request{req}) }},
	})
	r.menuAt.Set(&t.Offset{X: x, Y: y})
	t.RequestFocus(requestMenuID)
}

func (a *App) closeRequestMenu() {
	a.requestActions.menuAt.Set(nil)
	t.RequestFocus(treeID)
}

// withRequestMenu opens the menu when row, showing req, is right-clicked.
func (a *App) withRequestMenu(row t.Text, req *model.Request) t.Text {
	if !a.postmanUI || req == nil {
		return row
	}
	target := *req
	click := row.Click
	row.Click = func(e t.MouseEvent) {
		if e.Button == uv.MouseRight {
			a.openRequestMenu(target, e.X, e.Y)
			return
		}
		if click != nil {
			click(e)
		}
	}
	return row
}

type requestMenu struct{ app *App }

func (m requestMenu) Build(t.BuildContext) t.Widget {
	r := m.app.requestActions
	at := r.menuAt.Get()
	if at == nil {
		return t.ShowWhen(false, t.Menu{})
	}
	return t.Menu{
		ID:       requestMenuID,
		State:    r.menu,
		Position: t.FloatPositionAbsolute,
		Offset:   *at,
		// Terma runs OnSelect in place of the item's Action.
		OnSelect: func(item t.MenuItem) {
			r.menuAt.Set(nil)
			item.Action()
		},
		OnDismiss: m.app.closeRequestMenu,
	}
}
