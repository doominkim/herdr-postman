// Package cloud accesses account resources through the authenticated Postman CLI.
// Credentials remain owned by the CLI; they are never read into application config.
package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

type Resource struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type CLI struct{ Binary string }

func (c CLI) read(ctx context.Context, target any, args ...string) error {
	return c.run(ctx, target, nil, args...)
}

func (c CLI) run(ctx context.Context, target any, stdin []byte, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	bin := c.Binary
	if bin == "" {
		bin = "postman"
	}
	command := exec.CommandContext(ctx, bin, args...)
	command.Stdin = bytes.NewReader(stdin)
	var output bytes.Buffer
	command.Stdout = &output
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("Postman Cloud command failed; check CLI login and workspace access")
	}
	if err := json.Unmarshal(output.Bytes(), target); err != nil {
		return fmt.Errorf("Postman Cloud returned an invalid JSON response")
	}
	return nil
}

func (c CLI) Workspaces(ctx context.Context) ([]Resource, error) {
	var resources []Resource
	cursor := ""
	seen := map[string]bool{}
	for {
		var page struct {
			Workspaces []Resource `json:"workspaces"`
			Cursor     string     `json:"cursor"`
		}
		args := []string{"workspace", "list", "--json"}
		if cursor != "" {
			args = append(args, "--cursor", cursor)
		}
		if err := c.read(ctx, &page, args...); err != nil {
			return nil, err
		}
		if err := validateResources(page.Workspaces); err != nil {
			return nil, err
		}
		resources = append(resources, page.Workspaces...)
		if page.Cursor == "" {
			return resources, nil
		}
		if seen[page.Cursor] {
			return nil, fmt.Errorf("Postman Cloud repeated a workspace page cursor")
		}
		seen[page.Cursor] = true
		cursor = page.Cursor
	}
}

func (c CLI) Collections(ctx context.Context, workspace string) ([]Resource, error) {
	if workspace == "" {
		return nil, fmt.Errorf("select a cloud workspace first")
	}
	var resources []Resource
	if err := c.read(ctx, &resources, "collection", "list", "--workspace", workspace, "--json"); err != nil {
		return nil, err
	}
	if err := validateResources(resources); err != nil {
		return nil, err
	}
	return resources, nil
}

func validateResources(resources []Resource) error {
	if resources == nil {
		return fmt.Errorf("Postman Cloud returned an invalid resource list")
	}
	for _, resource := range resources {
		if resource.ID == "" || resource.Name == "" {
			return fmt.Errorf("Postman Cloud returned a resource without an ID or name")
		}
	}
	return nil
}
