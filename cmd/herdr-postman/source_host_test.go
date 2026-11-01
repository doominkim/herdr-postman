package main

import (
	"context"
	"fmt"
	"github.com/darrenburns/posting/v3/internal/model"
	"github.com/darrenburns/posting/v3/internal/ui"
	t "github.com/darrenburns/terma"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSourceHostRendersSelectBoxesInsideEditor(tst *testing.T) {
	host := newSourceHost(startupOptions{}, ui.Config{Collection: &model.Collection{Name: "Local fixture"}})
	defer host.Close()
	widget := host.Build(t.BuildContext{})
	_ = widget
	if mode, ok := host.mode.Value(); !ok || mode != "local" {
		tst.Fatal("local selection missing")
	}
	host.listWorkspaces = func(context.Context) ([]sourceWorkspace, error) {
		return []sourceWorkspace{{ID: "w1", Name: "Personal"}}, nil
	}
	// Synchronous completion keeps this test independent of a running event loop.
	host.launch = func(work func()) { work() }
	host.dispatch = func(work func()) { work() }
	host.selectMode("cloud")
	if len(host.workspaces.Peek()) != 1 {
		tst.Fatal("workspace options not loaded")
	}
	if host.app != nil && host.appMode == "cloud" {
		tst.Fatal("Cloud editor opened before workspace selection")
	}
	host.loadSource = func(context.Context, string, string) (ui.Config, func() error, error) {
		return ui.Config{Collection: &model.Collection{Name: "Personal", Children: []*model.Collection{{Name: "A"}, {Name: "B"}}}}, func() error { return nil }, nil
	}
	host.selectWorkspace("w1")
	if host.appMode != "cloud" || host.appWorkspace != "w1" {
		tst.Fatal("workspace did not open")
	}
	host.selectMode("local")
	if host.appMode != "local" {
		tst.Fatal("cannot return to local inside app")
	}
}

func TestSourceHostFailedWorkspaceLoadPreservesLocalEditor(tst *testing.T) {
	host := newSourceHost(startupOptions{}, ui.Config{Collection: &model.Collection{Name: "Local"}})
	defer host.Close()
	original := host.app
	host.launch = func(fn func()) { fn() }
	host.dispatch = func(fn func()) { fn() }
	host.loadSource = func(context.Context, string, string) (ui.Config, func() error, error) {
		return ui.Config{}, nil, fmt.Errorf("fixture load failed")
	}
	host.mode.SetValue("cloud")
	host.selectWorkspace("w1")
	mode, _ := host.mode.Value()
	if host.app != original || mode != "local" || host.busy.Peek() || host.message.Peek() == "" {
		tst.Fatal("failed transition replaced editor or left controls stuck")
	}
}

func TestSourceHostWithoutLocalPathUsesPrivateDefault(tst *testing.T) {
	base := tst.TempDir()
	tst.Setenv("XDG_DATA_HOME", base)
	root, err := defaultLocalCollection()
	if err != nil {
		tst.Fatal(err)
	}
	if !strings.HasPrefix(root, base+string(os.PathSeparator)) {
		tst.Fatal("default escaped data directory")
	}
	file := filepath.Join(root, ".resources", "definition.yaml")
	os.WriteFile(file, []byte("$kind: collection\nname: Existing\n"), 0600)
	if _, err := defaultLocalCollection(); err != nil {
		tst.Fatal(err)
	}
	data, _ := os.ReadFile(file)
	if !strings.Contains(string(data), "Existing") {
		tst.Fatal("default overwrote existing collection")
	}
}

func TestSourceHostCloseCancelsLoadingAndCleansLateResult(tst *testing.T) {
	host := newSourceHost(startupOptions{}, ui.Config{})
	started := make(chan struct{})
	var cleaned atomic.Bool
	host.dispatch = func(func()) {} // Event loop has already stopped.
	host.loadSource = func(ctx context.Context, _ string, _ string) (ui.Config, func() error, error) {
		close(started)
		<-ctx.Done()
		return ui.Config{}, func() error { cleaned.Store(true); return nil }, ctx.Err()
	}
	host.load("cloud", "w1")
	<-started
	if err := host.Close(); err != nil {
		tst.Fatal(err)
	}
	if !cleaned.Load() {
		tst.Fatal("late Cloud result leaked after shutdown")
	}
}

func TestWorkspaceLoadingHasDeadlineAndCanBeCancelled(tst *testing.T) {
	host := newSourceHost(startupOptions{}, ui.Config{})
	defer host.Close()
	var work func()
	host.launch = func(fn func()) { work = fn }
	host.dispatch = func(fn func()) { fn() }
	host.loadSource = func(ctx context.Context, _ string, _ string) (ui.Config, func() error, error) {
		if _, ok := ctx.Deadline(); !ok {
			tst.Error("workspace loading has no overall deadline")
		}
		if ctx.Err() != context.Canceled {
			tst.Error("Escape did not cancel workspace loading")
		}
		return ui.Config{}, nil, ctx.Err()
	}
	host.load("cloud", "w1")
	host.cancelLoading()
	work()
	if host.busy.Peek() {
		tst.Fatal("cancel left loading screen active")
	}
}

func TestSourceHostRestoresLastWorkspaceAndRemembersOnlySuccessfulLoads(tst *testing.T) {
	host := newSourceHost(startupOptions{root: tst.TempDir()}, ui.Config{})
	defer host.Close()
	host.launch = func(fn func()) { fn() }
	host.dispatch = func(fn func()) { fn() }
	host.listWorkspaces = func(context.Context) ([]sourceWorkspace, error) {
		return []sourceWorkspace{{ID: "w1", Name: "Personal"}}, nil
	}
	var remembered []sourceSelection
	host.remember = func(s sourceSelection) error { remembered = append(remembered, s); return nil }
	host.loadSource = func(context.Context, string, string) (ui.Config, func() error, error) { return ui.Config{}, nil, nil }
	host.restoreSource(sourceSelection{Mode: "cloud", Workspace: "w1", LocalRoot: host.opts.root})
	if host.appMode != "cloud" || host.appWorkspace != "w1" || len(remembered) != 1 || remembered[0].Workspace != "w1" {
		tst.Fatal("last workspace was not restored and remembered")
	}
	host.loadSource = func(context.Context, string, string) (ui.Config, func() error, error) {
		return ui.Config{}, nil, fmt.Errorf("access denied")
	}
	host.selectWorkspace("w2")
	if len(remembered) != 1 || host.appWorkspace != "w1" {
		tst.Fatal("failed selection replaced last successful workspace")
	}
}

func TestSourceHostMissingRememberedWorkspaceFallsBackToSelection(tst *testing.T) {
	host := newSourceHost(startupOptions{root: tst.TempDir()}, ui.Config{})
	defer host.Close()
	host.launch = func(fn func()) { fn() }
	host.dispatch = func(fn func()) { fn() }
	host.listWorkspaces = func(context.Context) ([]sourceWorkspace, error) {
		return []sourceWorkspace{{ID: "other", Name: "Other"}}, nil
	}
	host.loadSource = func(context.Context, string, string) (ui.Config, func() error, error) {
		tst.Fatal("missing workspace should not be loaded")
		return ui.Config{}, nil, nil
	}
	host.restoreSource(sourceSelection{Mode: "cloud", Workspace: "missing", LocalRoot: host.opts.root})
	if host.busy.Peek() || host.message.Peek() == "" {
		tst.Fatal("missing workspace left startup stuck")
	}
	if _, selected := host.workspace.Value(); selected {
		tst.Fatal("missing workspace remains selected")
	}
}

func TestCloudRefreshKeyKeepsEditorAndHandlesFailure(tst *testing.T) {
	host := newSourceHost(startupOptions{}, ui.Config{Collection: &model.Collection{Name: "Before"}})
	defer host.Close()
	host.appMode, host.appWorkspace = "cloud", "w1"
	host.mode.SetValue("cloud")
	host.workspace.SetValue("w1")
	host.launch = func(fn func()) { fn() }
	host.dispatch = func(fn func()) { fn() }
	original := host.app
	loads := 0
	host.loadSource = func(ctx context.Context, mode, id string) (ui.Config, func() error, error) {
		loads++
		if mode != "cloud" || id != "w1" {
			tst.Fatal("wrong refresh target")
		}
		if _, ok := ctx.Deadline(); !ok {
			tst.Fatal("refresh has no timeout")
		}
		if loads == 2 {
			return ui.Config{}, nil, fmt.Errorf("fixture refresh failed")
		}
		return ui.Config{Collection: &model.Collection{Name: "Refreshed"}}, nil, nil
	}
	var refresh func()
	for _, key := range host.Keybinds() {
		if key.Key == "ctrl+r" {
			refresh = key.Action
		}
	}
	if refresh == nil {
		tst.Fatal("Ctrl+R refresh not available")
	}
	refresh()
	if loads != 1 || host.app != original || host.busy.Peek() {
		tst.Fatal("refresh recreated editor or stayed busy")
	}
	refresh()
	if loads != 2 || host.app != original || !strings.Contains(host.message.Peek(), "fixture refresh failed") {
		tst.Fatal("failed refresh lost editor or error")
	}
	host.busy.Set(true)
	refresh()
	if loads != 2 {
		tst.Fatal("concurrent refresh started")
	}
	host.busy.Set(false)
}

func TestCloudRefreshFromEditorKeyboard(tst *testing.T) {
	host := newSourceHost(startupOptions{}, ui.Config{PostmanUI: true, Collection: &model.Collection{Name: "Before"}})
	defer host.Close()
	host.appMode, host.appWorkspace = "cloud", "w1"
	host.mode.SetValue("cloud")
	host.workspace.SetValue("w1")
	host.workspaces.Set([]sourceWorkspace{{ID: "w1", Name: "Personal"}})
	host.launch = func(fn func()) { fn() }
	host.dispatch = func(fn func()) { fn() }
	loads := 0
	host.loadSource = func(context.Context, string, string) (ui.Config, func() error, error) {
		loads++
		return ui.Config{Collection: &model.Collection{Name: "After", Children: []*model.Collection{{Name: "Imported collection", Path: "imported"}}}}, nil, nil
	}
	p := t.NewPilot(tst, host, 160, 45)
	if !strings.Contains(p.ScreenText(), "Refresh (Ctrl+R)") {
		tst.Fatal("refresh button missing")
	}
	host.app.FocusEditor()
	p.Advance(0)
	p.Press("ctrl+r")
	if loads != 1 || !strings.Contains(p.ScreenText(), "Imported collection") {
		tst.Fatal("editor shortcut did not refresh visible collection")
	}
	t.RequestFocus("source-refresh")
	p.Advance(0)
	p.Press("enter")
	if loads != 2 {
		tst.Fatal("refresh button did not reload")
	}
}

func TestPostmanLoadingScreenAndReturnToEditor(tst *testing.T) {
	host := newSourceHost(startupOptions{}, ui.Config{PostmanUI: true, Collection: &model.Collection{Name: "Saved collection"}})
	defer host.Close()
	host.busy.Set(true)
	p := t.NewPilot(tst, host, 100, 32)
	screen := p.ScreenText()
	for _, want := range []string{"herdr-postman", "Unofficial terminal client.", "Not affiliated with or endorsed by Postman.", "Esc to cancel", "Loading"} {
		if !strings.Contains(screen, want) {
			tst.Fatalf("loading screen missing %q", want)
		}
	}
	if strings.Contains(screen, "Source") || strings.Contains(screen, "Question") {
		tst.Fatal("loading screen exposes editor")
	}
	host.busy.Set(false)
	p.Advance(0)
	if strings.Contains(p.ScreenText(), "Unofficial terminal client.") || !strings.Contains(p.ScreenText(), "Source") || !strings.Contains(p.ScreenText(), "Question") {
		tst.Fatal("loading screen did not return to editor")
	}
}

func TestPostmanLoadingScreenFitsSmallTerminalAndCancels(tst *testing.T) {
	host := newSourceHost(startupOptions{}, ui.Config{})
	defer host.Close()
	host.busy.Set(true)
	cancelled := false
	host.loadCancel = func() { cancelled = true }
	p := t.NewPilot(tst, host, 60, 24)
	for _, want := range []string{"herdr-postman", "Not affiliated with or endorsed by Postman.", "Esc to cancel"} {
		if !strings.Contains(p.ScreenText(), want) {
			tst.Fatalf("small terminal hides %q", want)
		}
	}
	p.Press("esc")
	if !cancelled {
		tst.Fatal("loading screen cannot be cancelled")
	}
}

func TestLoadingLogoUsesCenteredFixedSizeBraille(tst *testing.T) {
	var first []string
	host := newSourceHost(startupOptions{}, ui.Config{})
	defer host.Close()
	host.busy.Set(true)
	p := t.NewPilot(tst, host, 60, 32)
	for _, width := range []int{60, 100, 180} {
		p.Resize(width, 32)
		var logo []string
		for _, line := range strings.Split(p.ScreenText(), "\n") {
			hasDots := strings.ContainsFunc(line, func(r rune) bool { return r >= '\u2801' && r <= '\u28ff' })
			if !hasDots {
				continue
			}
			row := []rune(line)
			left, right := 0, len(row)
			for left < right && (row[left] == ' ' || row[left] == '\u2800') {
				left++
			}
			for right > left && (row[right-1] == ' ' || row[right-1] == '\u2800') {
				right--
			}
			if diff := left - (len(row) - right); diff < -2 || diff > 2 {
				tst.Fatalf("logo is off center at width %d", width)
			}
			logo = append(logo, string(row[left:right]))
		}
		if len(logo) != 16 {
			tst.Fatalf("want 16 braille rows, got %d", len(logo))
		}
		if first == nil {
			first = logo
		} else if strings.Join(first, "\n") != strings.Join(logo, "\n") {
			tst.Fatal("resizing changed logo artwork")
		}
	}
}

func TestLoadingLogoRotatesWithoutMovingTextAndStops(tst *testing.T) {
	host := newSourceHost(startupOptions{}, ui.Config{})
	defer host.Close()
	host.busy.Set(true)
	p := t.NewPilot(tst, host, 100, 32)
	before := p.ScreenText()
	p.Advance(160 * time.Millisecond)
	after := p.ScreenText()
	if before == after {
		tst.Fatal("loading logo does not animate")
	}
	a, b := strings.Split(before, "\n"), strings.Split(after, "\n")
	for i, line := range a {
		if strings.Contains(line, "herdr-postman") || strings.Contains(line, "Postman") || strings.Contains(line, "Unofficial") || strings.Contains(line, "Esc to cancel") {
			if b[i] != line {
				tst.Fatal("rotation moves loading text")
			}
		}
	}
	host.busy.Set(false)
	p.Advance(0)
	if host.logo.IsRunning() {
		tst.Fatal("animation continues after loading")
	}
	host.busy.Set(true)
	p.Advance(0)
	if !host.logo.IsRunning() {
		tst.Fatal("animation does not restart on next load")
	}
	host.Close()
	if host.logo.IsRunning() {
		tst.Fatal("animation continues after shutdown")
	}
}

func TestCachedCloudShowsEditorBeforeBackgroundRefresh(tst *testing.T) {
	host := newSourceHost(startupOptions{}, ui.Config{})
	defer host.Close()
	host.hasCachedSource = func(string) bool { return true }
	var pending []func()
	host.launch = func(fn func()) { pending = append(pending, fn) }
	host.dispatch = func(fn func()) { fn() }
	host.loadCached = func(context.Context, string) (ui.Config, func() error, error) {
		return ui.Config{Collection: &model.Collection{Name: "Cache", Children: []*model.Collection{{Name: "Cached requests", Path: "cached"}}}}, nil, nil
	}
	host.loadSource = func(context.Context, string, string) (ui.Config, func() error, error) {
		return ui.Config{}, nil, fmt.Errorf("offline fixture")
	}
	host.restoreSource(sourceSelection{Mode: "cloud", Workspace: "w1"})
	if len(pending) != 1 {
		tst.Fatal("cache open not scheduled")
	}
	work := pending[0]
	pending = pending[1:]
	work()
	if host.appMode != "cloud" || len(pending) != 1 {
		tst.Fatal("cache did not open before network refresh")
	}
	original := host.app
	p := t.NewPilot(tst, host, 120, 32)
	if !strings.Contains(p.ScreenText(), "Cached requests") || strings.Contains(p.ScreenText(), "Unofficial terminal client.") {
		tst.Fatal("background refresh hides cached editor")
	}
	pending[0]()
	p.Advance(0)
	if host.app != original || host.busy.Peek() || !strings.Contains(host.message.Peek(), "offline fixture") {
		tst.Fatal("offline refresh discarded cached editor")
	}
}
