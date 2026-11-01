package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/darrenburns/posting/v3/internal/cloud"
	"github.com/darrenburns/posting/v3/internal/history"
	"github.com/darrenburns/posting/v3/internal/paths"
	"github.com/darrenburns/posting/v3/internal/ui"
	t "github.com/darrenburns/terma"
)

type sourceWorkspace = cloud.Resource

type sourceHost struct {
	opts                  startupOptions
	mode, workspace       *t.SelectState[string]
	workspaces            t.AnySignal[[]sourceWorkspace]
	busy                  t.Signal[bool]
	background            t.Signal[bool]
	hasCachedSource       func(string) bool
	loadCached            func(context.Context, string) (ui.Config, func() error, error)
	logo                  *t.SpinnerState
	message               t.Signal[string]
	app                   *ui.App
	appMode, appWorkspace string
	ctx                   context.Context
	cancel                context.CancelFunc
	loadCancel            context.CancelFunc
	launch, dispatch      func(func())
	listWorkspaces        func(context.Context) ([]sourceWorkspace, error)
	loadSource            func(context.Context, string, string) (ui.Config, func() error, error)
	mu                    sync.Mutex
	closed                bool
	closers               []func() error
	wg                    sync.WaitGroup
	remoteClose           func() error
	remember              func(sourceSelection) error
	restoreWorkspace      string
	onStart               func()
	startOnce             sync.Once
	sidebarStateDir       string
}

func newSourceHost(opts startupOptions, cfg ui.Config) *sourceHost {
	ctx, cancel := context.WithCancel(context.Background())
	h := &sourceHost{opts: opts, ctx: ctx, cancel: cancel, mode: t.NewSelectState[string](), workspace: t.NewSelectState[string](), workspaces: t.NewAnySignal([]sourceWorkspace(nil)), busy: t.NewSignal(false), message: t.NewSignal(""), app: ui.New(cfg), appMode: "local", dispatch: t.Dispatch}
	h.logo = t.NewSpinnerState(t.SpinnerStyle{Frames: postmanRotationFrames, FrameTime: 80 * time.Millisecond})
	h.mode.SetValue("local")
	h.launch = func(fn func()) { h.wg.Add(1); go func() { defer h.wg.Done(); fn() }() }
	h.listWorkspaces = (cloud.CLI{Binary: opts.bin}).Workspaces
	h.hasCachedSource = func(id string) bool {
		return cloud.HasWorkspaceCache(filepath.Join(paths.DataDir(), "cloud-cache"), id)
	}
	h.background = t.NewSignal(false)
	h.loadSource = h.configForSource
	h.loadCached = func(ctx context.Context, id string) (ui.Config, func() error, error) {
		return h.configForSourceWithCache(ctx, "cloud", id, true)
	}
	return h
}
func (h *sourceHost) configForSource(ctx context.Context, mode, id string) (ui.Config, func() error, error) {
	return h.configForSourceWithCache(ctx, mode, id, false)
}
func (h *sourceHost) configForSourceWithCache(ctx context.Context, mode, id string, cached bool) (ui.Config, func() error, error) {
	cfg, err := appConfig(h.opts.root, h.opts.bin, h.opts.environments...)
	if err != nil {
		return cfg, nil, err
	}
	cfg.AppName = "herdr-postman [Local]"
	if mode == "local" {
		return cfg, nil, nil
	}
	name := "Postman Cloud"
	// Workspace display name is cosmetic; the stable ID owns all routing.
	cli := cloud.CLI{Binary: h.opts.bin}
	cacheDir := filepath.Join(paths.DataDir(), "cloud-cache")
	var ws *cloud.Workspace
	if cached {
		ws, err = cli.OpenCachedWorkspace(cacheDir, id, name)
	} else {
		ws, err = cli.OpenWorkspace(ctx, id, name)
	}
	if err != nil {
		return ui.Config{}, nil, err
	}
	if !cached {
		if cacheErr := cloud.SaveWorkspaceCache(cacheDir, id, ws); cacheErr != nil {
			ws.Close()
			return ui.Config{}, nil, fmt.Errorf("could not save Cloud cache: %w", cacheErr)
		}
	}
	cfg.AppName = "herdr-postman [Cloud]"
	cfg.Collection, _ = ws.Load()
	cfg.Store = ws
	cfg.Sender = ws
	cfg.Watch = nil
	cfg.Reload = nil
	if err := configureCloudEnvironments(ctx, &cfg, h.opts.bin, id, cached); err != nil {
		ws.Close()
		return ui.Config{}, nil, err
	}
	cfg.History = nil
	if cfg.Settings != nil && cfg.Settings.History.Enabled {
		cfg.History = history.ForCollection(paths.HistoryDir(), "cloud-workspace:"+id)
	}
	return cfg, ws.Close, nil
}
func (h *sourceHost) allowed() bool {
	if h.busy.Peek() {
		return false
	}
	if h.app != nil {
		if err := h.app.CanSwitchSource(); err != nil {
			h.message.Set(err.Error())
			return false
		}
	}
	return true
}
func (h *sourceHost) restoreSelection() {
	h.mode.SetValue(h.appMode)
	if h.appWorkspace == "" {
		h.workspace.Clear()
	} else {
		h.workspace.SetValue(h.appWorkspace)
	}
}
func (h *sourceHost) selectMode(mode string) {
	if !h.allowed() {
		h.restoreSelection()
		return
	}
	h.mode.SetValue(mode)
	h.message.Set("")
	if mode == "local" {
		if h.appMode == "local" {
			h.rememberCurrent()
			return
		}
		h.load("local", "")
		return
	}
	if h.appMode == "cloud" {
		h.workspace.SetValue(h.appWorkspace)
		return
	}
	h.workspace.Clear()
	h.busy.Set(true)
	ctx, cancel := context.WithTimeout(h.ctx, time.Minute)
	h.loadCancel = cancel
	h.launch(func() {
		defer cancel()
		items, err := h.listWorkspaces(ctx)
		h.dispatch(func() {
			target := h.restoreWorkspace
			h.restoreWorkspace = ""
			if h.isClosed() {
				return
			}
			h.busy.Set(false)
			if err != nil {
				h.message.Set(err.Error())
				h.restoreSelection()
				return
			}
			h.workspaces.Set(items)
			if target != "" {
				for _, item := range items {
					if item.ID == target {
						h.selectWorkspace(target)
						return
					}
				}
				h.message.Set("Saved workspace is unavailable; choose a workspace")
			}
			t.RequestFocus("source-workspace")
			if len(items) == 0 {
				h.message.Set("No Cloud workspaces available")
			}
		})
	})
}
func (h *sourceHost) selectWorkspace(id string) {
	if !h.allowed() {
		h.restoreSelection()
		return
	}
	h.workspace.SetValue(id)
	h.load("cloud", id)
}
func (h *sourceHost) refresh() {
	if h.busy.Peek() || h.appMode != "cloud" {
		return
	}
	if err := h.app.CanRefreshSource(); err != nil {
		h.message.Set(err.Error())
		return
	}
	h.loadIntoEditor(h.appMode, h.appWorkspace, true)
}
func (h *sourceHost) load(mode, id string) { h.loadIntoEditor(mode, id, false) }
func (h *sourceHost) loadIntoEditor(mode, id string, refresh bool) {
	restoreList := h.restoreWorkspace != ""
	h.background.Set(refresh)
	h.busy.Set(true)
	h.message.Set("")
	if refresh {
		h.message.Set("Checking Cloud updates… cached requests remain available (Esc to cancel)")
	}
	ctx, cancel := context.WithTimeout(h.ctx, time.Minute)
	h.loadCancel = cancel
	h.launch(func() {
		defer cancel()
		var cfg ui.Config
		var closeStore func() error
		var err error
		cached := false
		if mode == "cloud" && !refresh && h.loadCached != nil {
			cfg, closeStore, err = h.loadCached(ctx, id)
			cached = err == nil
		}
		if !cached {
			cfg, closeStore, err = h.loadSource(ctx, mode, id)
		}
		var workspaceItems []sourceWorkspace
		if !cached && err == nil && mode == "cloud" && restoreList {
			workspaceItems, _ = h.listWorkspaces(ctx)
		}
		h.mu.Lock()
		if h.closed {
			h.mu.Unlock()
			if closeStore != nil {
				closeStore()
			}
			return
		}
		if closeStore != nil {
			h.closers = append(h.closers, closeStore)
		}
		h.mu.Unlock()
		h.dispatch(func() {
			if h.isClosed() {
				return
			}
			h.busy.Set(false)
			if err != nil {
				h.message.Set(err.Error())
				h.restoreSelection()
				return
			}
			if workspaceItems != nil {
				h.workspaces.Set(workspaceItems)
				h.restoreWorkspace = ""
			}
			if refresh {
				if err := h.app.RefreshSource(cfg); err != nil {
					h.message.Set(err.Error())
					return
				}
				h.saveSidebar()
				h.message.Set("Cloud refreshed; unsaved tabs kept")
				h.app.FocusEditor()
				return
			}
			if h.remoteClose != nil {
				h.remoteClose()
				h.remoteClose = nil
			}
			if h.app != nil {
				h.saveSidebar()
				if err := h.app.Close(); err != nil {
					h.message.Set(err.Error())
				}
			}
			h.app = ui.New(cfg)
			h.appMode = mode
			h.appWorkspace = id
			h.restoreSidebar()
			h.rememberCurrent()
			h.attachRemote(cfg)
			h.app.FocusEditor()
			if cached {
				h.loadIntoEditor(mode, id, true)
			}
		})
	})
}
func (h *sourceHost) attachRemote(cfg ui.Config) {
	// Cloud has its own routing identity and does not expose a local-file socket.
	if h.appMode != "local" {
		return
	}
	server, err := listenForRemote(cfg)
	if err != nil {
		h.message.Set(err.Error())
		return
	}
	if server != nil {
		server.Serve(h.app.Remote())
		h.remoteClose = server.Close
	}
}
func (h *sourceHost) isClosed() bool { h.mu.Lock(); defer h.mu.Unlock(); return h.closed }
func (h *sourceHost) Close() error {
	h.logo.Stop()
	h.mu.Lock()
	h.closed = true
	h.mu.Unlock()
	h.cancel()
	h.wg.Wait()
	var errs []error
	if h.remoteClose != nil {
		errs = append(errs, h.remoteClose())
	}
	if h.app != nil {
		h.saveSidebar()
		errs = append(errs, h.app.Close())
	}
	for _, closeStore := range h.closers {
		errs = append(errs, closeStore())
	}
	return errors.Join(errs...)
}
func (h *sourceHost) cancelLoading() {
	if h.busy.Peek() && h.loadCancel != nil {
		h.loadCancel()
	}
}

func (h *sourceHost) Keybinds() []t.Keybind {
	if h.busy.Peek() {
		return []t.Keybind{{Key: "esc", Name: "Cancel loading", Action: h.cancelLoading}}
	}
	binds := []t.Keybind{{Key: "alt+s", Name: "Source", Action: func() { t.RequestFocus("source-mode") }}}
	if h.appMode == "cloud" {
		binds = append(binds, t.Keybind{Key: "ctrl+r", Name: "Refresh", Action: h.refresh})
	}
	return binds
}
func (h *sourceHost) Build(ctx t.BuildContext) t.Widget {
	h.startOnce.Do(func() {
		if h.onStart != nil {
			h.dispatch(h.onStart)
		}
	})
	mode, _ := h.mode.Value()
	workspace, _ := h.workspace.Value()
	busy := h.busy.Get()
	if busy && !h.background.Get() {
		status := "Loading Postman resources…"
		if mode == "cloud" && workspace == "" {
			status = "Loading workspaces…"
		}
		if mode == "cloud" && workspace != "" {
			status = "Loading workspace collections…"
		}
		if !h.logo.IsRunning() {
			h.logo.Start()
		}
		return postmanLoadingScreen(ctx, status, h.logo.Frame())
	}
	h.logo.Stop()
	controls := []t.Widget{t.Text{Content: " herdr-postman ", Style: t.Style{ForegroundColor: ctx.Theme().Secondary}}, t.Text{Content: " Source "}, t.SelectBox[string]{ID: "source-mode", State: h.mode, Options: []t.SelectOption[string]{{Label: "Local", Value: "local"}, {Label: "Cloud", Value: "cloud"}}, Disabled: busy, Style: t.Style{Width: t.Cells(14)}, OnChange: h.selectMode}}
	if mode == "cloud" {
		options := []t.SelectOption[string]{}
		for _, ws := range h.workspaces.Get() {
			options = append(options, t.SelectOption[string]{Label: safeDisplay(ws.Name), Value: ws.ID})
		}
		controls = append(controls, t.Text{Content: " Workspace "}, t.SelectBox[string]{ID: "source-workspace", State: h.workspace, Options: options, Placeholder: "Choose workspace", Searchable: true, Disabled: busy, Style: t.Style{Width: t.Cells(38)}, OnChange: h.selectWorkspace})
	}
	if h.appMode == "cloud" && !busy {
		controls = append(controls, t.Button{ID: "source-refresh", Label: "Refresh (Ctrl+R)", OnPress: h.refresh})
	}
	var body t.Widget = t.Text{Content: "Choose a workspace above to browse its collections."}
	if h.app != nil && mode == h.appMode && (mode == "local" || workspace == h.appWorkspace) {
		body = h.app
	}
	return t.Column{Style: t.Style{Width: t.Flex(1), Height: t.Flex(1)}, Children: []t.Widget{t.Row{Style: t.Style{Height: t.Cells(1), BackgroundColor: ctx.Theme().Surface}, Children: controls}, t.Text{Content: h.message.Get()}, t.Column{Style: t.Style{Width: t.Flex(1), Height: t.Flex(1)}, Children: []t.Widget{body}}}}
}

func (h *sourceHost) rememberCurrent() {
	if h.remember == nil {
		return
	}
	if err := h.remember(sourceSelection{Mode: h.appMode, Workspace: h.appWorkspace, LocalRoot: h.opts.root}); err != nil {
		h.message.Set("Opened source, but could not remember selection: " + err.Error())
	}
}
func (h *sourceHost) restoreSource(saved sourceSelection) {
	if saved.Mode == "cloud" {
		h.restoreWorkspace = saved.Workspace
		if !h.hasCachedSource(saved.Workspace) {
			h.selectMode("cloud")
			return
		}
		h.mode.SetValue("cloud")
		h.workspace.SetValue(saved.Workspace)
		h.workspaces.Set([]sourceWorkspace{{ID: saved.Workspace, Name: "Saved workspace"}})
		h.load("cloud", saved.Workspace)
	}
}

func runSourceHost(opts startupOptions) int {
	memoryFile := filepath.Join(paths.DataDir(), "source.json")
	var saved sourceSelection
	warning := ""
	if opts.source == "choose" {
		saved = readSourceSelection(memoryFile)
		if saved.Mode != "" {
			if info, err := os.Stat(saved.LocalRoot); err == nil && info.IsDir() {
				opts.root = saved.LocalRoot
				opts.environments = nil
			} else {
				warning = "Saved local collection is unavailable; using the launch directory"
			}
		}
	}
	if opts.root == "" {
		root, err := defaultLocalCollection()
		if err != nil {
			fmt.Println(err)
			return 1
		}
		opts.root = root
	}
	cfg, err := appConfig(opts.root, opts.bin, opts.environments...)
	if err != nil {
		fmt.Println(err)
		return 1
	}
	cfg.AppName = "herdr-postman [Local]"
	if canonical, err := filepath.EvalSymlinks(opts.root); err == nil {
		opts.root, _ = filepath.Abs(canonical)
	}
	host := newSourceHost(opts, cfg)
	host.sidebarStateDir = filepath.Join(paths.DataDir(), "sidebar")
	host.restoreSidebar()
	host.remember = func(s sourceSelection) error { return saveSourceSelection(memoryFile, s) }
	host.message.Set(warning)
	if saved.Mode == "cloud" {
		host.onStart = func() { host.restoreSource(saved) }
	} else if saved.Mode == "" || warning == "" {
		host.rememberCurrent()
	}
	host.attachRemote(cfg)
	defer quitOnSignal(func() { t.Dispatch(t.Quit) })()
	err = errors.Join(t.Run(host), host.Close())
	if err != nil {
		fmt.Println(err)
		return 1
	}
	return 0
}

func (h *sourceHost) currentSelection() sourceSelection {
	return sourceSelection{Mode: h.appMode, Workspace: h.appWorkspace, LocalRoot: h.opts.root}
}
func (h *sourceHost) saveSidebar() {
	if h.sidebarStateDir == "" || h.app == nil {
		return
	}
	if err := saveSidebarState(h.sidebarStateDir, h.currentSelection(), h.app.CollapsedFolders()); err != nil {
		h.message.Set("Could not remember sidebar: " + err.Error())
	}
}
func (h *sourceHost) restoreSidebar() {
	if h.sidebarStateDir != "" && h.app != nil {
		h.app.RestoreCollapsedFolders(readSidebarState(h.sidebarStateDir, h.currentSelection()))
	}
}
