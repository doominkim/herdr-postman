package cloud

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCloudEnvironmentsListLazilyAndKeepValuesOutOfCache(t *testing.T) {
	cli := CLI{Binary: fixtureCLI(t, `case "$*" in
 'environment list --workspace w1 --json') printf '%s' '[{"id":"123-e1","name":"Development"}]' ;;
 'environment get 123-e1 --json') printf '%s' '{"id":"e1","owner":"123","name":"Development","values":[{"key":"url","value":"https://example.test","enabled":true},{"key":"secret","value":"fixture-private-value","enabled":true},{"key":"off","value":"ignored","enabled":false},{"key":"empty","value":"","enabled":true}]}' ;;
 *) exit 1 ;;
 esac`)}
	root := t.TempDir()
	source, err := cli.OpenEnvironments(context.Background(), root, "w1", false)
	if err != nil {
		t.Fatal(err)
	}
	ids := source.Candidates()
	if len(ids) != 1 || ids[0][0] != "123-e1" {
		t.Fatalf("candidates: %v", ids)
	}
	name, err := source.Describe(ids[0])
	if err != nil || name != "Development" {
		t.Fatalf("name: %q %v", name, err)
	}
	env, err := source.Load(ids[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(env.Variables) != 3 || env.Variables[0].Name != "url" || env.Variables[2].Value != "" {
		t.Fatal("enabled variables were not preserved")
	}
	if _, err := source.Load([]string{"foreign"}); err == nil {
		t.Fatal("accepted environment outside workspace")
	}
	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			data, _ := os.ReadFile(path)
			if strings.Contains(string(data), "fixture-private-value") {
				t.Fatal("persisted values")
			}
		}
		return err
	})
	offline, err := (CLI{Binary: "/nonexistent"}).OpenEnvironments(context.Background(), root, "w1", true)
	if err != nil {
		t.Fatal(err)
	}
	if name, err := offline.Describe(ids[0]); err != nil || name != "Development" {
		t.Fatal("cached metadata unavailable")
	}
	if _, err := offline.Load(ids[0]); err == nil {
		t.Fatal("values unexpectedly cached on disk")
	}
}
