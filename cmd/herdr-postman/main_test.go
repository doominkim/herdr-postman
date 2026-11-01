package main

import (
	"fmt"
	"github.com/darrenburns/posting/v3/internal/paths"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darrenburns/posting/v3/internal/client"
	"github.com/darrenburns/posting/v3/internal/collection"
)

func TestStartupUsesNativeStorageAndPostmanEngine(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := os.WriteFile(filepath.Join(dir, "health.request.yaml"), []byte("$kind: http-request\nmethod: GET\nurl: https://example.test/health\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := appConfig(dir, "postman")
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.PostmanUI {
		t.Fatal("Postman workbench layout not enabled")
	}
	if _, ok := cfg.Store.(*collection.NativeStore); !ok {
		t.Fatalf("unexpected store %T", cfg.Store)
	}
	if any(cfg.Reload) != any(cfg.Store) {
		t.Fatal("manual reload does not use native store")
	}
	if any(cfg.Watch) != any(cfg.Store) {
		t.Fatal("native file watching is not enabled")
	}
	if _, ok := cfg.Sender.(*client.PostmanCollection); !ok {
		t.Fatalf("unexpected engine %T", cfg.Sender)
	}
	if len(cfg.Collection.Requests) != 1 || cfg.Collection.Requests[0].File != "health.request.yaml" {
		t.Fatal("native collection not loaded")
	}
}

func TestStartupCanDisableWatchingWithoutDisablingReload(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configFile, []byte("watch_collection_files: false\nhistory:\n  enabled: false\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HERDR_POSTMAN_CONFIG_FILE", configFile)
	cfg, err := appConfig(dir, "postman")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Watch != nil {
		t.Fatal("watching enabled despite explicit setting")
	}
	if cfg.Reload == nil {
		t.Fatal("manual reload disabled with polling")
	}
}

func TestStartupDiscoversPostmanEnvironments(t *testing.T) {
	root := t.TempDir()
	collectionDir := filepath.Join(root, "postman", "collections", "sample")
	environmentDir := filepath.Join(root, "postman", "environments")
	os.MkdirAll(collectionDir, 0700)
	os.MkdirAll(environmentDir, 0700)
	file := filepath.Join(environmentDir, "dev.environment.yaml")
	os.WriteFile(file, []byte("name: Dev\nvalues: []\n"), 0600)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	cfg, err := appConfig(collectionDir, "postman", file)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Environments == nil || len(cfg.Environments.Candidates()) != 1 {
		t.Fatal("native environments not discovered")
	}
	if len(cfg.Environment) != 1 || cfg.Environment[0] != file {
		t.Fatal("startup environment not selected")
	}
}

func TestNativeStartupRemembersEnvironmentSelectionOnly(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	first := filepath.Join(dir, "first.environment.yaml")
	second := filepath.Join(dir, "second.environment.yaml")
	for _, file := range []string{first, second} {
		if err := os.WriteFile(file, []byte("name: Fixture\nvalues:\n  - key: test\n    value: fixture-value-not-to-persist\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := appConfig(dir, "postman")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RememberEnvironment == nil {
		t.Fatal("selection persistence not wired")
	}
	cfg.RememberEnvironment([]string{first})
	cfg, err = appConfig(dir, "postman")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Environment) != 1 || cfg.Environment[0] != first {
		t.Fatal("selection not restored")
	}
	cfg, err = appConfig(dir, "postman", second)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Environment[0] != second {
		t.Fatal("remembered selection overrode explicit flag")
	}
	data, err := os.ReadFile(paths.EnvironmentMemory())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "fixture-value-not-to-persist") {
		t.Fatal("variable value persisted in selection memory")
	}
	cfg.RememberEnvironment(nil)
	cfg, err = appConfig(dir, "postman")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Environment) != 0 {
		t.Fatal("cleared environment restored unexpectedly")
	}
}

func TestInvalidRememberedEnvironmentDoesNotPreventStartup(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	file := filepath.Join(dir, "broken.environment.yaml")
	os.WriteFile(file, []byte("name: [broken"), 0600)
	cfg, err := appConfig(dir, "postman")
	if err != nil {
		t.Fatal(err)
	}
	cfg.RememberEnvironment([]string{file})
	cfg, err = appConfig(dir, "postman")
	if err != nil {
		t.Fatal("remembered invalid file blocked startup")
	}
	if len(cfg.Environment) != 0 || len(cfg.StartupMessages) == 0 {
		t.Fatal("invalid remembered selection not reported")
	}
	if _, err = appConfig(dir, "postman", file); err == nil {
		t.Fatal("explicit invalid environment accepted")
	}
}

func TestStartupLoadsUserThemesAndPresentationSettings(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	themesDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(themesDir, "fixture.yaml"), []byte("name: native_fixture\nprimary: '#abcdef'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(themesDir, "invalid.yaml"), []byte("name: invalid\nprimary: not-a-colour\n"), 0600); err != nil {
		t.Fatal(err)
	}
	configFile := filepath.Join(t.TempDir(), "config.yaml")
	writeConfig := func(load, font bool) {
		t.Helper()
		data := fmt.Sprintf("theme_directory: %q\nload_user_themes: %t\nnerd_fonts: %t\n", themesDir, load, font)
		if err := os.WriteFile(configFile, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HERDR_POSTMAN_CONFIG_FILE", configFile)
	writeConfig(true, true)
	cfg, err := appConfig(t.TempDir(), "postman")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.UserThemes) != 1 || cfg.UserThemes[0].Name != "native_fixture" {
		t.Fatal("user theme not loaded")
	}
	if !cfg.NerdFonts || cfg.OpenURL == nil {
		t.Fatal("icons or link opener not connected")
	}
	warned := false
	for _, message := range cfg.StartupMessages {
		if strings.Contains(message, "theme") {
			warned = true
		}
	}
	if !warned {
		t.Fatal("invalid theme was not reported")
	}
	writeConfig(false, false)
	cfg, err = appConfig(t.TempDir(), "postman")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.UserThemes) != 0 || cfg.NerdFonts {
		t.Fatal("disabled theme/font setting ignored")
	}
	for _, message := range cfg.StartupMessages {
		if strings.Contains(message, "invalid.yaml") {
			t.Fatal("disabled theme loading still read invalid theme")
		}
	}
}
