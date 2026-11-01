package ui

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/darrenburns/posting/v3/internal/env"
	"github.com/darrenburns/posting/v3/internal/model"
)

type nativeEnvironmentWatchSource struct {
	env.NativeFiles
	loads atomic.Int32
}

func (s *nativeEnvironmentWatchSource) Load(files []string) (model.Environment, error) {
	s.loads.Add(1)
	return s.NativeFiles.Load(files)
}

func TestNativeEnvironmentWatchReloadsAndStops(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "dev.environment.yaml")
	write := func(body string) {
		t.Helper()
		if err := os.WriteFile(file, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("name: Dev\nvalues:\n  - key: host\n    value: original\n")
	source := &nativeEnvironmentWatchSource{NativeFiles: env.NativeFiles{Roots: []string{dir}}}
	app := New(Config{Environments: source, Environment: []string{file}})
	defer app.Close()
	app.setSessionVars(map[string]string{"override": "retained"})
	updates := make(chan func(), 32)
	app.dispatch = func(fn func()) { updates <- fn }
	app.watchEnvironment(5 * time.Millisecond)
	next := func() {
		t.Helper()
		select {
		case fn := <-updates:
			fn()
		case <-time.After(time.Second):
			t.Fatal("environment change not dispatched")
		}
	}
	write("name: Renamed\nvalues:\n  - key: host\n    value: changed\n")
	next()
	current := app.env.active.Peek()
	if current.Name != "Renamed" || model.Values(current.Variables)["host"] != "changed" {
		t.Fatal("valid file changes not applied")
	}
	write("name: NameOnly\nvalues:\n  - key: host\n    value: changed\n")
	next()
	if app.env.active.Peek().Name != "NameOnly" {
		t.Fatal("name-only edit was ignored")
	}
	write("name: [broken")
	next()
	if model.Values(app.env.active.Peek().Variables)["host"] != "changed" {
		t.Fatal("invalid file replaced valid environment")
	}
	write("name: Recovered\nvalues:\n  - key: host\n    value: recovered\n")
	next()
	if app.env.active.Peek().Name != "Recovered" || app.sessionVars.Peek()["override"] != "retained" {
		t.Fatal("reload lost name or session overrides")
	}
	if err := app.Close(); err != nil {
		t.Fatal(err)
	}
	count := source.loads.Load()
	time.Sleep(20 * time.Millisecond)
	if source.loads.Load() != count {
		t.Fatal("environment watcher still running after close")
	}
}
