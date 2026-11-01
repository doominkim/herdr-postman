package ui

import "github.com/darrenburns/posting/v3/internal/collection"

// requestSuffix lets the editor use the active store's native file format.
func (a *App) requestSuffix() string {
	if store, ok := a.store.(interface{ RequestSuffix() string }); ok {
		return store.RequestSuffix()
	}
	return collection.FileSuffix
}

func (a *App) isNative() bool { return a.requestSuffix() == collection.NativeSuffix }
