package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWorkspaceCacheReopensOfflineAndReusesUnchangedContent(t *testing.T) {
	dir := t.TempDir()
	bin := fixtureCLI(t, `case "$*" in
 'collection list --workspace w1 --json') printf '%s' '[{"id":"c1","name":"Fixture"}]' ;;
 'collection get c1 --json') printf '%s' '{"$kind":"collection","id":"c1","name":"Fixture","items":[{"$kind":"http-request","id":"r1","name":"Saved request","method":"GET","url":"https://example.test"}]}' ;;
 *) exit 3 ;;
esac`)
	cli := CLI{Binary: bin}
	ws, err := cli.OpenWorkspace(context.Background(), "w1", "Workspace")
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	if err := SaveWorkspaceCache(dir, "w1", ws); err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*", "*.json"))
	if len(files) == 0 {
		t.Fatal("no cache files")
	}
	before := map[string]int64{}
	for _, f := range files {
		s, _ := os.Stat(f)
		before[f] = s.ModTime().UnixNano()
		if s.Mode().Perm() != 0600 {
			t.Fatal("cache is not private")
		}
	}
	if err := SaveWorkspaceCache(dir, "w1", ws); err != nil {
		t.Fatal(err)
	}
	for f, mtime := range before {
		s, _ := os.Stat(f)
		if s.ModTime().UnixNano() != mtime {
			t.Fatal("unchanged content rewritten")
		}
	}
	offline, err := (CLI{Binary: "/nonexistent/postman"}).OpenCachedWorkspace(dir, "w1", "Workspace")
	if err != nil {
		t.Fatal(err)
	}
	defer offline.Close()
	root, problems := offline.Load()
	if len(problems) != 0 || len(root.Children) != 1 || len(root.Children[0].Requests) != 1 {
		t.Fatal("cached requests missing")
	}
	if _, err := cli.OpenCachedWorkspace(dir, "different-workspace", "Other"); err == nil {
		t.Fatal("workspace cache leaked across identity")
	}
}

func TestInvalidRefreshDoesNotReplaceCommittedCache(t *testing.T) {
	dir := t.TempDir()
	snapshot, err := snapshotFromDocument("c1", json.RawMessage(`{"$kind":"collection","id":"c1","name":"Old","items":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	store, err := (CLI{}).storeFromSnapshot(context.Background(), "w1", snapshot, false)
	if err != nil {
		t.Fatal(err)
	}
	w := &Workspace{stores: map[string]*Store{"c1": store}}
	defer w.Close()
	if err := SaveWorkspaceCache(dir, "w1", w); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(cacheWorkspaceDir(dir, "w1"), "manifest.json")
	before, _ := os.ReadFile(manifestPath)
	store.snapshot.Document = json.RawMessage(`invalid`)
	if err := SaveWorkspaceCache(dir, "w1", w); err == nil {
		t.Fatal("invalid refresh accepted")
	}
	after, _ := os.ReadFile(manifestPath)
	if !bytes.Equal(before, after) {
		t.Fatal("failed refresh changed committed cache")
	}
	var manifest cacheManifest
	json.Unmarshal(before, &manifest)
	os.WriteFile(filepath.Join(cacheWorkspaceDir(dir, "w1"), manifest.Collections[0].Hash+".json"), []byte(`{}`), 0600)
	if _, err := (CLI{}).OpenCachedWorkspace(dir, "w1", "Workspace"); err == nil {
		t.Fatal("corrupt cached content accepted")
	}
}

func TestWorkspaceDownloadsAtMostThreeCollectionsConcurrently(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLOUD_FIXTURE_DIR", dir)
	bin := fixtureCLI(t, `case "$*" in
 'collection list --workspace w1 --json') printf '%s' '[{"id":"c1","name":"1"},{"id":"c2","name":"2"},{"id":"c3","name":"3"},{"id":"c4","name":"4"}]' ;;
 'collection get '* )
 touch "$CLOUD_FIXTURE_DIR/$3.started"
 while [ ! -f "$CLOUD_FIXTURE_DIR/release" ]; do sleep 0.02; done
 printf '{"$kind":"collection","id":"%s","name":"Fixture","items":[]}' "$3" ;;
 *) exit 3 ;;
esac`)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		w, err := (CLI{Binary: bin}).OpenWorkspace(ctx, "w1", "Fixture")
		if w != nil {
			w.Close()
		}
		done <- err
	}()
	deadline := time.Now().Add(2 * time.Second)
	count := 0
	for time.Now().Before(deadline) {
		files, _ := filepath.Glob(filepath.Join(dir, "*.started"))
		count = len(files)
		if count >= 3 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	os.WriteFile(filepath.Join(dir, "release"), nil, 0600)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("want three concurrent requests before release, got %d", count)
	}
}
