package main

import (
	"context"
	"strings"

	"github.com/darrenburns/posting/v3/internal/client"
	"github.com/darrenburns/posting/v3/internal/cloud"
	"github.com/darrenburns/posting/v3/internal/history"
	"github.com/darrenburns/posting/v3/internal/paths"
	"github.com/darrenburns/posting/v3/internal/ui"
)

// Remote display names must not be interpreted as terminal escape sequences.
func safeDisplay(value string) string {
	return strings.Map(func(r rune) rune {
		if r < 32 || (r >= 127 && r <= 159) {
			return ' '
		}
		return r
	}, value)
}

func cloudAppConfig(opts startupOptions) (ui.Config, func() error, error) {
	store, err := (cloud.CLI{Binary: opts.bin}).OpenStore(context.Background(), opts.workspace, opts.cloudCollection)
	if err != nil {
		return ui.Config{}, nil, err
	}
	cfg, err := appConfig(store.Root(), opts.bin)
	if err != nil {
		store.Close()
		return ui.Config{}, nil, err
	}
	cfg.AppName = "herdr-postman [Cloud]"
	cfg.Store = store
	cfg.Collection, _ = store.Load()
	cfg.Sender = &client.PostmanCollection{Binary: opts.bin, Root: store.Root(), Render: store.Render}
	cfg.Watch = nil
	cfg.Reload = nil
	if err := configureCloudEnvironments(context.Background(), &cfg, opts.bin, opts.workspace, false); err != nil {
		store.Close()
		return ui.Config{}, nil, err
	}
	if cfg.Settings != nil && cfg.Settings.History.Enabled {
		cfg.History = history.ForCollection(paths.HistoryDir(), "cloud:"+opts.workspace+":"+opts.cloudCollection)
	}
	return cfg, store.Close, nil
}
