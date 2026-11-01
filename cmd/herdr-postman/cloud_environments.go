package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/darrenburns/posting/v3/internal/cloud"
	"github.com/darrenburns/posting/v3/internal/paths"
	"github.com/darrenburns/posting/v3/internal/ui"
)

func configureCloudEnvironments(ctx context.Context, cfg *ui.Config, bin, workspace string, cached bool) error {
	source, err := (cloud.CLI{Binary: bin}).OpenEnvironments(ctx, filepath.Join(paths.DataDir(), "cloud-cache"), workspace, cached)
	if err != nil {
		return fmt.Errorf("could not load Cloud environments: %w", err)
	}
	file := filepath.Join(paths.DataDir(), "cloud-environments", fmt.Sprintf("%x.json", sha256.Sum256([]byte(workspace))))
	var selected []string
	if data, err := os.ReadFile(file); err == nil {
		_ = json.Unmarshal(data, &selected)
	}
	if len(selected) > 0 {
		if _, err := source.Describe(selected); err != nil {
			cfg.StartupMessages = append(cfg.StartupMessages, "Saved Cloud environment is no longer available. Choose an environment before sending requests.")
			selected = nil
		}
	}
	if len(selected) > 0 {
		if _, err := source.LoadContext(ctx, selected); err != nil {
			return fmt.Errorf("could not restore selected Cloud environment: %w", err)
		}
	}
	cfg.Environments = source
	cfg.Environment = selected
	cfg.WatchEnvironment = false
	cfg.RememberEnvironment = func(ids []string) { _ = writePrivateJSON(file, ids) }
	return nil
}
