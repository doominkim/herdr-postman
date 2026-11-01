package ui

import (
	"bytes"
	"html"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/darrenburns/posting/v3/internal/client"
	"github.com/darrenburns/posting/v3/internal/collection"
	"github.com/darrenburns/posting/v3/internal/history"
	t "github.com/darrenburns/terma"
)

func TestNativeCLIHistoryRestoresRunAndResponse(tst *testing.T) {
	bin, err := exec.LookPath("postman")
	if err != nil {
		tst.Skip("Postman CLI not installed")
	}
	body := []byte{0, 255, 128, 65}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "fixture", Value: "history", HttpOnly: true})
		w.Write(body)
	}))
	defer server.Close()
	dir, historyDir := tst.TempDir(), tst.TempDir()
	source := "$kind: http-request\nname: History fixture\nmethod: GET\nurl: " + server.URL + "\nscripts:\n  - type: afterResponse\n    language: text/javascript\n    code: pm.test('history fixture assertion', () => { pm.expect(1).to.eql(2); });\n"
	if err := os.WriteFile(filepath.Join(dir, "history.request.yaml"), []byte(source), 0600); err != nil {
		tst.Fatal(err)
	}
	store := collection.NewNativeStore(dir)
	coll, problems := store.Load()
	if len(problems) > 0 {
		tst.Fatal(problems)
	}
	app := New(Config{Store: store, Collection: coll, History: history.ForCollection(historyDir, dir), Sender: &client.PostmanCollection{Binary: bin, Root: dir, Render: store.Render}})
	defer app.Close()
	// Persistence completions need not be rendered before Close drains the queue.
	app.dispatch = func(fn func()) {}
	app.openRequest(coll.Requests[0])
	session := app.current()
	updates := make(chan func(), 1)
	session.dispatch = func(fn func()) { updates <- fn }
	app.send()
	select {
	case fn := <-updates:
		fn()
	case <-time.After(15 * time.Second):
		session.Cancel()
		tst.Fatal("CLI send timed out")
	}
	if err := session.err.Peek(); err != nil {
		tst.Fatal(err)
	}
	if err := app.Close(); err != nil {
		tst.Fatal(err)
	}
	restoredStore := collection.NewNativeStore(dir)
	restoredCollection, problems := restoredStore.Load()
	if len(problems) > 0 {
		tst.Fatal(problems)
	}
	reopened := New(Config{Store: restoredStore, Collection: restoredCollection, History: history.ForCollection(historyDir, dir)})
	defer reopened.Close()
	entries := reopened.history.Peek()
	if len(entries) != 1 {
		tst.Fatal("sent exchange missing after restart")
	}
	reopened.openHistory(entries[0])
	s := reopened.current()
	resp := s.response.Peek()
	if resp == nil || resp.StatusCode != 200 || !bytes.Equal(resp.Body, body) || resp.PostmanRun == nil || resp.PostmanRun.ExitCode == 0 {
		tst.Fatal("reopened response lost HTTP bytes or failed run result")
	}
	if strings.Contains(resp.PostmanRun.Output, "Could not read response data") && !strings.HasPrefix(resp.PostmanRun.Output, "Internal response capture succeeded.") {
		tst.Fatal("internal reporter warning not distinguished from user API failure")
	}
	if len(resp.Cookies) != 1 || resp.Cookies[0].Name != "fixture" || !resp.Cookies[0].HTTPOnly {
		tst.Fatal("restored cookie lost")
	}
	s.responseTab.Set("run")
	buf := t.RenderToBuffer(responseRun{session: s, response: resp}, 120, 80)
	svg := t.BufferToSVG(buf, 120, 80, t.DefaultSVGOptions())
	var visible strings.Builder
	for _, node := range regexp.MustCompile(`<text[^>]*>(.*?)</text>`).FindAllStringSubmatch(svg, -1) {
		visible.WriteString(html.UnescapeString(node[1]))
	}
	if !strings.Contains(visible.String(), "historyfixtureassertion") {
		tst.Fatal("restored run output not rendered")
	}
	sourceAfter, _ := os.ReadFile(filepath.Join(dir, "history.request.yaml"))
	if string(sourceAfter) != source {
		tst.Fatal("history path changed source collection")
	}
}
