// herdr-postman shares Postman Native Git files with Desktop Local View.
package main

import (
	"errors"
	"flag"
	"fmt"
	"github.com/darrenburns/posting/v3/internal/remote"
	"os"
	"path/filepath"
	"time"

	"github.com/darrenburns/posting/v3/internal/client"
	"github.com/darrenburns/posting/v3/internal/collection"
	"github.com/darrenburns/posting/v3/internal/config"
	"github.com/darrenburns/posting/v3/internal/env"
	"github.com/darrenburns/posting/v3/internal/history"
	"github.com/darrenburns/posting/v3/internal/paths"
	"github.com/darrenburns/posting/v3/internal/themes"
	"github.com/darrenburns/posting/v3/internal/ui"
	"github.com/darrenburns/terma"
)

func main() { os.Exit(run()) }

func run() int {
	if len(os.Args) > 1 && os.Args[1] == "remote" {
		return remoteCommand(os.Args[2:], os.Stdin, os.Stdout, os.Stderr)
	}
	opts, err := parseStartupOptions(os.Args[1:], os.Stderr)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	enableModifierKeys()
	if opts.source == "choose" || opts.source == "local" {
		return runSourceHost(opts)
	}
	return runSource(opts)
}

func runSource(opts startupOptions) int {
	var cfg ui.Config
	var err error
	if opts.source == "cloud" {
		var closeStore func() error
		cfg, closeStore, err = cloudAppConfig(opts)
		if closeStore != nil {
			defer closeStore()
		}
	} else {
		cfg, err = appConfig(opts.root, opts.bin, opts.environments...)
		cfg.AppName = "herdr-postman [Local]"
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	server, remoteErr := listenForRemote(cfg)
	if remoteErr != nil {
		cfg.StartupMessages = append(cfg.StartupMessages, "herdr-postman remote unavailable: "+remoteErr.Error())
	}
	app := ui.New(cfg)
	if server != nil {
		server.Serve(app.Remote())
		defer server.Close()
		defer closeOnSignal(server)()
	}
	err = errors.Join(terma.Run(app), app.Close())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func appConfig(root, bin string, environmentFiles ...string) (ui.Config, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return ui.Config{}, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return ui.Config{}, err
	}
	info, err := os.Stat(root)
	if err != nil {
		return ui.Config{}, err
	}
	if !info.IsDir() {
		return ui.Config{}, fmt.Errorf("collection must be a directory")
	}
	environmentRoot := filepath.Join(root, "environments")
	if filepath.Base(filepath.Dir(root)) == "collections" {
		environmentRoot = filepath.Join(filepath.Dir(filepath.Dir(root)), "environments")
	}
	source := env.NativeFiles{Roots: []string{environmentRoot}}
	memory := env.Memory{File: paths.EnvironmentMemory()}
	explicitEnvironment := len(environmentFiles) > 0
	environmentWarning := ""
	if len(environmentFiles) == 0 {
		environmentFiles = memory.Recall(root)
	}
	for i, file := range environmentFiles {
		absolute, err := filepath.Abs(file)
		if err != nil {
			return ui.Config{}, err
		}
		environmentFiles[i] = absolute
	}
	if _, err := source.Load(environmentFiles); err != nil {
		if explicitEnvironment {
			return ui.Config{}, err
		}
		environmentFiles = nil
		environmentWarning = "Remembered environment could not be loaded; select a valid environment"
	}
	store := collection.NewNativeStore(root)
	coll, problems := store.Load()
	settings, messages := config.Load()
	if environmentWarning != "" {
		messages = append(messages, environmentWarning)
	}
	for _, problem := range problems {
		messages = append(messages, problem.Error())
	}
	var userThemes []themes.Theme
	if settings.LoadUserThemes {
		var problems []error
		userThemes, problems = themes.LoadDir(settings.ThemeDirectory)
		for _, problem := range problems {
			messages = append(messages, "Couldn't load theme "+problem.Error())
		}
	}
	cfg := ui.Config{
		AppName:             "herdr-postman",
		PostmanUI:           true,
		OpenURL:             terma.OpenURL,
		NerdFonts:           settings.UseNerdFonts(os.LookupEnv),
		UserThemes:          userThemes,
		Version:             "dev",
		Settings:            &settings,
		Collection:          coll,
		Store:               store,
		Reload:              store,
		Environments:        source,
		Environment:         environmentFiles,
		WatchEnvironment:    settings.WatchEnvFiles,
		RememberEnvironment: func(files []string) { _ = memory.Remember(root, files) },
		LocalValues:         env.LocalValues{Dir: filepath.Join(paths.DataDir(), "local-values")},
		Sender:              &client.PostmanCollection{Binary: bin, Root: root, Render: store.Render},
		StartupMessages:     messages,
	}
	if settings.WatchCollectionFiles {
		cfg.Watch = store
	}
	if settings.History.Enabled {
		cfg.History = history.ForCollection(paths.HistoryDir(), root)
	}
	return cfg, nil
}

func listenForRemote(cfg ui.Config) (*remote.Server, error) {
	if cfg.Settings == nil || !cfg.Settings.RemoteControl.Enabled {
		return nil, nil
	}
	store, ok := cfg.Store.(*collection.NativeStore)
	if !ok {
		return nil, fmt.Errorf("remote control requires a native collection store")
	}
	cwd, _ := os.Getwd()
	return remote.Listen(paths.SocketDir(), remote.Info{PID: os.Getpid(), Version: cfg.Version, Collection: store.Root, Cwd: cwd, Started: time.Now()})
}
