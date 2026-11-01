package cloud

import (
	"context"
	"github.com/darrenburns/posting/v3/internal/model"
	"os"
	"path/filepath"
	"testing"
)

func TestStoreSavePersistsToCloudBeforeUpdatingPrivateCopy(t *testing.T) {
	state := filepath.Join(t.TempDir(), "updated")
	t.Setenv("CLOUD_TEST_STATE", state)
	bin := fixtureCLI(t, `case "$*" in
 'collection get c1 --json') printf '%s' '{"$kind":"collection","id":"c1","name":"Fixture","items":[{"$kind":"http-request","id":"r1","name":"Original","method":"GET","url":"https://example.test/old"}]}' ;;
 'collection request get r1 --collection c1 --workspace w1 --json')
 if [ -f "$CLOUD_TEST_STATE" ]; then url=new; else url=old; fi
 printf '{"$kind":"http-request","id":"r1","name":"Original","method":"GET","url":"https://example.test/%s","parent":{"id":"c1"}}' "$url" ;;
 'collection request update r1 --collection c1 --workspace w1 --type http --json --url https://example.test/new') touch "$CLOUD_TEST_STATE"; printf '{}' ;;
 *) exit 3 ;;
esac`)
	store, err := (CLI{Binary: bin}).OpenStore(context.Background(), "w1", "c1")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	coll, problems := store.Load()
	if len(problems) != 0 || len(coll.Requests) != 1 {
		t.Fatal("request not loaded", problems)
	}
	req := coll.Requests[0]
	req.URL = "https://example.test/new"
	if err := store.Save(req); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(state); err != nil {
		t.Fatal("cloud was not updated")
	}
	coll, problems = store.Load()
	if len(problems) != 0 || coll.Requests[0].URL != req.URL {
		t.Fatal("confirmed save not reflected locally")
	}
	if err := store.Create(req); err == nil {
		t.Fatal("create silently wrote only to cache")
	}
	if err := store.Delete(req.File); err == nil {
		t.Fatal("delete silently removed only cache")
	}
}

// Opt-in account check: reads only; never executes or edits a saved request.
func TestLiveCloudStoreRead(t *testing.T) {
	id, workspace := os.Getenv("HERDR_POSTMAN_CLOUD_READ_TEST"), os.Getenv("HERDR_POSTMAN_WORKSPACE_READ_TEST")
	if id == "" || workspace == "" {
		t.Skip("set both read-only account fixture IDs")
	}
	store, err := (CLI{}).OpenStore(context.Background(), workspace, id)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	coll, problems := store.Load()
	if len(problems) != 0 {
		t.Fatal("cloud request materialization failed; details omitted")
	}
	count := 0
	coll.Walk(func(_ *model.Collection, _ model.Request) { count++ })
	if count == 0 {
		t.Fatal("expected existing requests")
	}
	t.Logf("opened %d canonical Cloud requests without execution or writes", count)
}
