package cloud

import (
	"context"
	"fmt"
	"github.com/darrenburns/posting/v3/internal/model"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWorkspaceShowsAllCollectionsAndRoutesFiles(t *testing.T) {
	bin := fixtureCLI(t, `case "$*" in
 'collection list --workspace w1 --json') printf '%s' '[{"id":"c1","name":"First"},{"id":"c2","name":"Second"}]' ;;
 'collection get c1 --json'|'collection get c2 --json')
 if [ "$3" = c1 ]; then label=First; suffix=first; else label=Second; suffix=second; fi
 printf '{"$kind":"collection","id":"%s","name":"Collection","items":[{"$kind":"http-request","id":"r1","name":"%s request","method":"GET","url":"https://example.test/%s"}]}' "$3" "$label" "$suffix" ;;
 'collection request get r1 --collection c1 --workspace w1 --json') printf '%s' '{"$kind":"http-request","id":"r1","name":"First request","method":"GET","url":"https://example.test/first"}' ;;
 'collection request get r1 --collection c2 --workspace w1 --json') printf '%s' '{"$kind":"http-request","id":"r1","name":"Second request","method":"GET","url":"https://example.test/second"}' ;;
 *) exit 3 ;;
esac`)
	ws, err := (CLI{Binary: bin}).OpenWorkspace(context.Background(), "w1", "Workspace")
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	coll, problems := ws.Load()
	if len(problems) != 0 || len(coll.Children) != 2 {
		t.Fatal("workspace did not include both collections")
	}
	files := map[string]bool{}
	coll.Walk(func(_ *model.Collection, req model.Request) {
		if files[req.File] {
			t.Fatal("collection identities collide")
		}
		files[req.File] = true
		data, err := ws.Render(req)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), req.URL) {
			t.Fatal("request routed to wrong collection")
		}
		if err := ws.Save(req); err != nil {
			t.Fatal(err)
		}
	})
	if len(files) != 2 {
		t.Fatal("missing requests")
	}
}

func TestWorkspaceOpensWithoutFetchingEveryRequestIndividually(t *testing.T) {
	bin := fixtureCLI(t, `case "$*" in
 'collection list --workspace w1 --json') printf '%s' '[{"id":"c1","name":"Collection"}]' ;;
 'collection get c1 --json') printf '%s' '{"$kind":"collection","id":"c1","name":"Collection","items":[{"$kind":"http-request","id":"r1","name":"Request","method":"GET","url":"https://example.test"}]}' ;;
 *) exit 3 ;;
esac`)
	ws, err := (CLI{Binary: bin}).OpenWorkspace(context.Background(), "w1", "Workspace")
	if err != nil {
		t.Fatal("opening workspace unnecessarily waits for individual request reads:", err)
	}
	defer ws.Close()
	coll, problems := ws.Load()
	if len(problems) != 0 || len(coll.Children) != 1 || len(coll.Children[0].Requests) != 1 {
		t.Fatal("collection response did not populate sidebar")
	}
}

func TestLiveCloudWorkspaceRead(t *testing.T) {
	id := os.Getenv("HERDR_POSTMAN_WORKSPACE_READ_TEST")
	if id == "" {
		t.Skip("opt-in read-only workspace check")
	}
	started := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	ws, err := (CLI{}).OpenWorkspace(ctx, id, "Workspace")
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	coll, problems := ws.Load()
	if len(problems) != 0 {
		t.Fatal("workspace has unsupported documents; details omitted")
	}
	count := 0
	coll.Walk(func(_ *model.Collection, _ model.Request) { count++ })
	t.Logf("loaded %d collections and %d requests in %s; no writes or request execution", len(coll.Children), count, time.Since(started).Round(time.Millisecond))
}

func TestWorkspaceDeferredRequestReadPreservesMetadataAndRejectsConflict(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		t.Run(fmt.Sprint(conflict), func(t *testing.T) {
			marker := filepath.Join(t.TempDir(), "saved")
			t.Setenv("CLOUD_LAZY_MARKER", marker)
			current := "old"
			if conflict {
				current = "gui"
			}
			t.Setenv("CLOUD_LAZY_CURRENT", current)
			bin := fixtureCLI(t, `case "$*" in
 'collection list --workspace w1 --json') printf '%s' '[{"id":"c1","name":"Collection"}]' ;;
 'collection get c1 --json') printf '%s' '{"$kind":"collection","id":"c1","name":"Collection","items":[{"$kind":"http-request","id":"r1","method":"GET","url":"https://example.test/old"}]}' ;;
 'collection request get r1 --collection c1 --workspace w1 --json')
 if [ -f "$CLOUD_LAZY_MARKER" ]; then url=new;else url="$CLOUD_LAZY_CURRENT";fi
 printf '{"$kind":"http-request","id":"r1","method":"GET","url":"https://example.test/%s","parent":{"id":"c1"},"examples":[]}' "$url" ;;
 'collection request update r1 --collection c1 --workspace w1 --type http --json --url https://example.test/new') touch "$CLOUD_LAZY_MARKER";printf '{}' ;;
 *) exit 3 ;;
esac`)
			ws, err := (CLI{Binary: bin}).OpenWorkspace(context.Background(), "w1", "Workspace")
			if err != nil {
				t.Fatal(err)
			}
			defer ws.Close()
			coll, _ := ws.Load()
			req := coll.Children[0].Requests[0]
			req.URL = "https://example.test/new"
			err = ws.Save(req)
			_, statErr := os.Stat(marker)
			if conflict {
				if err == nil || !os.IsNotExist(statErr) {
					t.Fatal("GUI edit was overwritten")
				}
			} else if err != nil || statErr != nil {
				t.Fatal("matching snapshot could not save with endpoint-only metadata", err)
			}
		})
	}
}
