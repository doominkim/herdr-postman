package ui

import (
	"github.com/darrenburns/posting/v3/internal/model"
	"sort"
)

// CollapsedFolders uses stable collection-relative identities, never row indices.
func (a *App) CollapsedFolders() []string {
	result := []string{}
	collapsed := a.tree.Collapsed.Peek()
	a.walkFolders(func(folder *model.Collection) {
		if collapsed["f:"+folder.Path] {
			result = append(result, folder.Path)
		}
	})
	sort.Strings(result)
	return result
}
func (a *App) RestoreCollapsedFolders(paths []string) {
	saved := map[string]bool{}
	for _, path := range paths {
		saved[path] = true
	}
	collapsed := map[string]bool{}
	a.walkFolders(func(folder *model.Collection) {
		if saved[folder.Path] {
			collapsed["f:"+folder.Path] = true
		}
	})
	a.tree.Collapsed.Set(collapsed)
}
func (a *App) walkFolders(fn func(*model.Collection)) {
	var walk func(*model.Collection)
	walk = func(parent *model.Collection) {
		for _, folder := range parent.Children {
			fn(folder)
			walk(folder)
		}
	}
	walk(a.collection.Peek())
}
