package ui

import (
	"fmt"
	"github.com/darrenburns/posting/v3/internal/model"
	t "github.com/darrenburns/terma"
)

// CanSwitchSource protects every tab before the host replaces this editor.
func (a *App) CanSwitchSource() error {
	for _, s := range a.sessions.Peek() {
		if s.dirty.Peek() {
			return fmt.Errorf("save or close unsaved tabs before switching source")
		}
		if s.phase.Peek() == exchangeSending || s.streaming.Peek() == streamOpen {
			return fmt.Errorf("stop running requests before switching source")
		}
	}
	return nil
}

// FocusEditor returns keyboard control to the editor after a source switch.
func (a *App) FocusEditor() { t.RequestFocus(urlInputID) }

// CanRefreshSource allows drafts but protects active requests from a store swap.
func (a *App) CanRefreshSource() error {
	for _, s := range a.sessions.Peek() {
		if s.phase.Peek() == exchangeSending || s.streaming.Peek() == streamOpen {
			return fmt.Errorf("stop running requests before refreshing")
		}
	}
	return nil
}

// RefreshSource installs a fresh snapshot of the same source without replacing
// the editor. Dirty tabs retain their original revision for conflict detection.
func (a *App) RefreshSource(cfg Config) error {
	if err := a.CanRefreshSource(); err != nil {
		return err
	}
	if cfg.Collection == nil {
		return fmt.Errorf("refresh returned no collection")
	}
	var environment model.Environment
	if cfg.Environments != nil {
		files := a.env.active.Peek().Files
		if len(files) > 0 {
			var err error
			environment, err = cfg.Environments.Load(files)
			if err != nil {
				return fmt.Errorf("could not refresh selected environment; keeping previous snapshot: %w", err)
			}
		}
	}
	collapsed := a.CollapsedFolders()
	if cfg.Store != nil {
		a.store = cfg.Store
	}
	if cfg.Sender != nil {
		a.sender = cfg.Sender
	}
	if cfg.Environments != nil {
		a.env.source = cfg.Environments
		a.env.remember = cfg.RememberEnvironment
		a.setEnvironment(environment)
		if a.overlay.Peek() == "variables" {
			a.refreshVariableRows()
		}
	}
	a.watcher = cfg.Reload
	a.replaceCollection(cfg.Collection)
	a.RestoreCollapsedFolders(collapsed)
	return nil
}
