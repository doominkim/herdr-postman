package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/darrenburns/posting/v3/internal/ui"
)

func TestCloudEnvironmentSelectionRestoresPerWorkspace(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	bin := filepath.Join(t.TempDir(), "postman")
	os.WriteFile(bin, []byte(`#!/bin/sh
case "$*" in
 'environment list --workspace w1 --json') echo '[{"id":"e1","name":"Development"}]' ;;
 'environment list --workspace w2 --json') echo '[]' ;;
 'environment get e1 --json') echo '{"id":"e1","values":[{"key":"url","value":"https://dev.test","enabled":true}]}' ;;
 *) exit 1 ;;
esac
`), 0700)
	var cfg ui.Config
	if err := configureCloudEnvironments(context.Background(), &cfg, bin, "w1", false); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Environments.Candidates()) != 1 || len(cfg.Environment) != 0 {
		t.Fatal("initial selection")
	}
	cfg.RememberEnvironment([]string{"e1"})
	var restored ui.Config
	if err := configureCloudEnvironments(context.Background(), &restored, bin, "w1", true); err != nil {
		t.Fatal(err)
	}
	if len(restored.Environment) != 1 || restored.Environment[0] != "e1" {
		t.Fatal("selection not restored")
	}
	loaded, err := restored.Environments.Load(restored.Environment)
	if err != nil || len(loaded.Variables) != 1 {
		t.Fatal("restored values unavailable")
	}
	var other ui.Config
	if err := configureCloudEnvironments(context.Background(), &other, bin, "w2", false); err != nil {
		t.Fatal(err)
	}
	if len(other.Environment) != 0 {
		t.Fatal("selection leaked between workspaces")
	}
	restored.RememberEnvironment(nil)
	if err := configureCloudEnvironments(context.Background(), &cfg, bin, "w1", true); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Environment) != 0 {
		t.Fatal("clear was not remembered")
	}
}

func TestDeletedRememberedEnvironmentAllowsChoosingReplacement(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	bin := filepath.Join(t.TempDir(), "postman")
	os.WriteFile(bin, []byte("#!/bin/sh\necho '[]'\n"), 0700)
	var cfg ui.Config
	if err := configureCloudEnvironments(context.Background(), &cfg, bin, "w1", false); err != nil {
		t.Fatal(err)
	}
	cfg.RememberEnvironment([]string{"deleted"})
	if err := configureCloudEnvironments(context.Background(), &cfg, bin, "w1", false); err != nil {
		t.Fatalf("cannot choose a replacement: %v", err)
	}
	if len(cfg.Environment) != 0 || len(cfg.StartupMessages) == 0 {
		t.Fatal("missing environment was not explicitly reported")
	}
}
