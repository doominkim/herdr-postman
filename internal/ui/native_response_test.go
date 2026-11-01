package ui

import (
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/darrenburns/posting/v3/internal/client"
	"github.com/darrenburns/posting/v3/internal/collection"
	t "github.com/darrenburns/terma"
)

func TestNativeCLICookiesReachResponseTable(tt *testing.T) {
	bin, err := exec.LookPath("postman")
	if err != nil {
		tt.Skip("Postman CLI not installed")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "fixture_cookie", Value: "cookie-value", HttpOnly: true, Secure: true, Path: "/api"})
		w.Write([]byte("response body"))
	}))
	defer server.Close()
	dir := tt.TempDir()
	store := collection.NewNativeStore(dir)
	app := New(Config{Store: store, Sender: &client.PostmanCollection{Binary: bin, Root: dir, Render: store.Render}})
	defer app.Close()
	s := app.current()
	s.url.SetText(server.URL)
	s.urlEdited()
	updates := make(chan func(), 1)
	s.dispatch = func(fn func()) { updates <- fn }
	app.send()
	select {
	case fn := <-updates:
		fn()
	case <-time.After(15 * time.Second):
		s.Cancel()
		tt.Fatal("CLI send did not finish")
	}
	if s.err.Peek() != nil {
		tt.Fatal(s.err.Peek())
	}
	rows := s.responseCookies.Rows.Peek()
	if len(rows) != 1 || rows[0].Name != "fixture_cookie" || !rows[0].HTTPOnly || !rows[0].Secure {
		tt.Fatal("actual CLI cookies not populated in UI")
	}
	s.responseTab.Set("cookies")
	buf := t.RenderToBuffer(responseCookies{app: app, session: s}, 100, 15)
	svg := t.BufferToSVG(buf, 100, 15, t.DefaultSVGOptions())
	for _, text := range []string{"fixture_cookie", "cookie-value", "HttpOnly", "Secure"} {
		if !strings.Contains(svg, text) {
			tt.Fatalf("cookie table does not render %s", text)
		}
	}
}
