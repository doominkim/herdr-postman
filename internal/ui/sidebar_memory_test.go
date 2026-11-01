package ui

import (
	"github.com/darrenburns/posting/v3/internal/model"
	t "github.com/darrenburns/terma"
	"reflect"
	"testing"
)

func TestSidebarRestoresCollapsedFoldersByIdentityNotPosition(tt *testing.T) {
	a := &model.Collection{Name: "A", Path: "a", Children: []*model.Collection{{Name: "Nested", Path: "a/nested"}}}
	b := &model.Collection{Name: "B", Path: "b"}
	app := New(Config{Collection: &model.Collection{Children: []*model.Collection{a, b}}})
	defer app.Close()
	t.RenderToBuffer(app, 130, 40)
	app.tree.Collapse([]int{0})
	app.tree.Collapse([]int{0, 0})
	saved := app.CollapsedFolders()
	if !reflect.DeepEqual(saved, []string{"a", "a/nested"}) {
		tt.Fatal("folder state not captured", saved)
	}
	reopened := New(Config{Collection: &model.Collection{Children: []*model.Collection{b, a}}})
	defer reopened.Close()
	reopened.RestoreCollapsedFolders(append(saved, "deleted"))
	t.RenderToBuffer(reopened, 130, 40)
	if reopened.tree.IsCollapsed([]int{0}) || !reopened.tree.IsCollapsed([]int{1}) || !reopened.tree.IsCollapsed([]int{1, 0}) {
		tt.Fatal("folder state followed position instead of identity")
	}
	if !reflect.DeepEqual(reopened.CollapsedFolders(), saved) {
		tt.Fatal("deleted folder retained")
	}
}
