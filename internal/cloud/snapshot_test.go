package cloud

import (
	"context"
	"github.com/darrenburns/posting/v3/internal/client"
	"github.com/darrenburns/posting/v3/internal/model"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// Opt-in and read-only: never run a real account's requests or update its data.
func TestLiveCloudCollectionRead(t *testing.T) {
	id := os.Getenv("HERDR_POSTMAN_CLOUD_READ_TEST")
	if id == "" {
		t.Skip("set HERDR_POSTMAN_CLOUD_READ_TEST for an authorized read-only check")
	}
	snapshot, err := (CLI{}).Open(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	defer snapshot.Close()
	coll, problems := snapshot.Store.Load()
	if len(problems) > 0 {
		t.Fatal("cloud snapshot has unsupported or invalid documents; content omitted")
	}
	count := 0
	coll.Walk(func(_ *model.Collection, _ model.Request) { count++ })
	if count == 0 {
		t.Fatal("expected existing cloud requests")
	}
	if err := exec.Command("postman", "collection", "lint", snapshot.Store.Root).Run(); err != nil {
		t.Fatal("CLI lint rejected cloud snapshot; content omitted")
	}
	t.Logf("read and linted %d cloud requests without execution or modification", count)
}

func TestOpenCloudCollectionRetainsNativeRequestAndFolderContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, ok := r.BasicAuth()
		if !ok || user != "fixture" || password != "fixture-only" || r.Header.Get("X-Note") != "fixture" {
			t.Error("inherited cloud auth or header lost")
		}
		w.Write([]byte("cloud fixture response"))
	}))
	defer server.Close()
	bin := fixtureCLI(t, strings.ReplaceAll(`test "$*" = 'collection get owner-c1 --json'
cat <<'JSON'
{"id":"c1","$kind":"collection","name":"Existing API","items":[{"id":"folder-1","$kind":"folder","name":"GUI folder","auth":{"type":"basic","credentials":[{"key":"username","value":"fixture"},{"key":"password","value":"fixture-only"}]},"items":[{"id":"req-1","$kind":"http-request","name":"Get item","method":"GET","url":"https://example.test/item","headers":[{"key":"X-Note","value":"fixture","description":"GUI note"}],"scripts":[{"type":"afterResponse","language":"text/javascript","code":"pm.test('ok',()=>{});"}],"examples":[{"id":"example-1","$kind":"http-example","name":"OK","response":{"statusCode":200,"body":{"type":"json","content":"{}"}}}]}]}]}
JSON
`, "https://example.test", server.URL))
	snapshot, err := (CLI{Binary: bin}).Open(context.Background(), "owner-c1")
	if err != nil {
		t.Fatal(err)
	}
	defer snapshot.Close()
	coll, problems := snapshot.Store.Load()
	if len(problems) != 0 || len(coll.Children) != 1 || len(coll.Children[0].Requests) != 1 {
		t.Fatal("cloud tree not loaded", problems)
	}
	if coll.Name != "Existing API" || coll.Children[0].Name != "GUI folder" {
		t.Fatal("cloud display names replaced by cache paths")
	}
	req := coll.Children[0].Requests[0]
	if req.Name != "Get item" || req.URL != server.URL+"/item" {
		t.Fatal("cloud request changed")
	}
	data, err := snapshot.Store.Render(req)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "GUI note") || !strings.Contains(string(data), "pm.test") {
		t.Fatal("GUI-managed fields lost")
	}
	if postman, err := exec.LookPath("postman"); err == nil {
		response, err := (&client.PostmanCollection{Binary: postman, Root: snapshot.Store.Root, Render: snapshot.Store.Render}).Send(context.Background(), client.Call{Request: req})
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != 200 || string(response.Body) != "cloud fixture response" || response.PostmanRun == nil || response.PostmanRun.ExitCode != 0 {
			t.Fatal("cloud snapshot could not execute through CLI")
		}
	}
	info, err := os.Stat(snapshot.Store.Root)
	if err != nil || info.Mode().Perm() != 0700 {
		t.Fatal("snapshot is not private")
	}
	root := snapshot.Store.Root
	if err := snapshot.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("private snapshot remained after close")
	}
}
