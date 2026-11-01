package main

import (
	"context"
	"github.com/darrenburns/posting/v3/internal/ui"
	"os"
	"path/filepath"
	"testing"
)

func TestCloudStartupUsesCloudStoreAndDisablesLocalWatch(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	bin := filepath.Join(t.TempDir(), "postman")
	os.WriteFile(bin, []byte(`#!/bin/sh
case "$*" in
 'environment list --workspace w1 --json') echo '[]' ;;
 'collection get c1 --json') printf '%s' '{"$kind":"collection","id":"c1","name":"Fixture","items":[]}' ;;
 *) exit 1 ;;
esac
`), 0700)
	cfg, closeStore, err := cloudAppConfig(startupOptions{source: "cloud", workspace: "w1", cloudCollection: "c1", bin: bin})
	if err != nil {
		t.Fatal(err)
	}
	defer closeStore()
	if len(cfg.StartupMessages) != 0 {
		t.Fatalf("successful Cloud open produced errors: %v", cfg.StartupMessages)
	}
	if cfg.AppName != "herdr-postman [Cloud]" || cfg.Watch != nil || cfg.Reload != nil {
		t.Fatal("cloud source is presented as local")
	}
	if err := cfg.Store.Delete("unknown.request.yaml"); err == nil {
		t.Fatal("cloud store silently permits cache deletion")
	}
}

func TestSuccessfulCloudWorkspaceDoesNotShowErrorToast(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	bin := filepath.Join(t.TempDir(), "postman")
	if err := os.WriteFile(bin, []byte(`#!/bin/sh
case "$*" in
 'environment list --workspace w1 --json') echo '[]' ;;
 'collection list --workspace w1 --json') printf '%s' '[{"id":"c1","name":"Fixture"}]' ;;
 'collection get c1 --json') printf '%s' '{"$kind":"collection","id":"c1","name":"Fixture","items":[]}' ;;
 *) exit 1 ;;
esac
`), 0700); err != nil {
		t.Fatal(err)
	}
	host := newSourceHost(startupOptions{root: t.TempDir(), bin: bin}, ui.Config{})
	defer host.Close()
	cfg, closeStore, err := host.configForSource(context.Background(), "cloud", "w1")
	if err != nil {
		t.Fatal(err)
	}
	defer closeStore()
	if len(cfg.StartupMessages) != 0 {
		t.Fatalf("successful workspace open produced errors: %v", cfg.StartupMessages)
	}
}
