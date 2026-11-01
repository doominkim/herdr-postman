package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darrenburns/posting/v3/internal/collection"
	"github.com/darrenburns/posting/v3/internal/model"
)

func TestPostmanSavedCookieSettingControlsJarAndRedirects(t *testing.T) {
	bin, err := exec.LookPath("postman")
	if err != nil {
		t.Skip("Postman CLI not installed")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/seed":
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "fixture", Path: "/"})
		case "/redirect":
			http.SetCookie(w, &http.Cookie{Name: "redirect", Value: "fixture", Path: "/"})
			http.Redirect(w, r, "/check", http.StatusFound)
			return
		}
		w.Write([]byte(r.Header.Get("Cookie")))
	}))
	defer server.Close()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "cookies.request.yaml"), []byte("$kind: http-request\nname: Cookies\nmethod: GET\nurl: "+server.URL+"/redirect\nsettings:\n  disableCookies: true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	store := collection.NewNativeStore(dir)
	coll, problems := store.Load()
	if len(problems) != 0 {
		t.Fatal(problems)
	}
	runner := &PostmanCollection{Binary: bin, Root: dir, Render: store.Render}
	seed := model.NewRequest()
	seed.URL = server.URL + "/seed"
	if _, err := runner.Send(context.Background(), Call{Request: seed}); err != nil {
		t.Fatal(err)
	}
	req := coll.Requests[0]
	resp, err := runner.Send(context.Background(), Call{Request: req})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 || len(resp.Body) != 0 {
		t.Fatal("disabled Native cookie setting attached cookies on redirect")
	}
	req.Options.AttachCookies = true
	req.URL = server.URL + "/check"
	if err := store.Save(req); err != nil {
		t.Fatal(err)
	}
	coll, problems = store.Load()
	if len(problems) != 0 {
		t.Fatal(problems)
	}
	resp, err = runner.Send(context.Background(), Call{Request: coll.Requests[0]})
	if err != nil {
		t.Fatal(err)
	}
	if string(resp.Body) != "session=fixture" {
		t.Fatalf("saved enable option did not restore jar: %q", resp.Body)
	}
}

func TestPostmanResponseCookiesPreserveAttributes(t *testing.T) {
	bin, err := exec.LookPath("postman")
	if err != nil {
		t.Skip("Postman CLI not installed")
	}
	expires := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "session", Value: "fixture", Domain: "example.test", Path: "/api", Secure: true, HttpOnly: true, Expires: expires})
		http.SetCookie(w, &http.Cookie{Name: "preference", Value: "compact", Path: "/"})
		w.Write([]byte("cookies"))
	}))
	defer server.Close()
	for _, mode := range []string{"collection", "request"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			store := collection.NewNativeStore(dir)
			var sender Sender = &PostmanCollection{Binary: bin, Root: dir, Render: store.Render}
			if mode == "request" {
				sender = &PostmanRequest{Binary: bin}
			}
			req := model.NewRequest()
			req.URL = server.URL
			resp, err := sender.Send(context.Background(), Call{Request: req})
			if err != nil {
				t.Fatal(err)
			}
			if len(resp.Cookies) != 2 {
				t.Fatalf("got %d cookies", len(resp.Cookies))
			}
			first := resp.Cookies[0]
			if first.Name != "session" || first.Value != "fixture" || first.Domain != "example.test" || first.Path != "/api" || !first.Secure || !first.HTTPOnly || first.Expires != expires.Format(time.RFC1123) {
				t.Fatal("cookie attributes lost")
			}
			if resp.Cookies[1].Name != "preference" || resp.Cookies[1].Secure || resp.Cookies[1].HTTPOnly || resp.Cookies[1].Expires != "" {
				t.Fatal("session cookie attributes changed")
			}
			count := 0
			for _, header := range resp.Headers {
				if strings.EqualFold(header.Name, "Set-Cookie") {
					count++
				}
			}
			if count != 2 {
				t.Fatal("raw Set-Cookie headers lost")
			}
		})
	}
}

func TestPostmanCollectionReusesCookiesWithinAppOnly(t *testing.T) {
	bin, err := exec.LookPath("postman")
	if err != nil {
		t.Skip("Postman CLI not installed")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/set":
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "fixture", Path: "/"})
		case "/check":
			cookie, err := r.Cookie("session")
			if err != nil || cookie.Value != "fixture" {
				w.WriteHeader(401)
				return
			}
		case "/empty":
			if _, err := r.Cookie("session"); err == nil {
				w.WriteHeader(400)
				return
			}
		}
		w.Write([]byte("ok"))
	}))
	defer server.Close()
	dir := t.TempDir()
	store := collection.NewNativeStore(dir)
	runner := &PostmanCollection{Binary: bin, Root: dir, Render: store.Render}
	send := func(sender *PostmanCollection, path string, attach bool) int {
		t.Helper()
		req := model.NewRequest()
		req.URL = server.URL + path
		req.Options.AttachCookies = attach
		resp, err := sender.Send(context.Background(), Call{Request: req})
		if err != nil {
			t.Fatal(err)
		}
		return resp.StatusCode
	}
	if send(runner, "/set", true) != 200 || send(runner, "/check", true) != 200 {
		t.Fatal("cookie not reused")
	}
	if send(runner, "/empty", false) != 200 {
		t.Fatal("disabled cookie attachment ignored")
	}
	fresh := &PostmanCollection{Binary: bin, Root: dir, Render: store.Render}
	if send(fresh, "/empty", true) != 200 {
		t.Fatal("cookie leaked into another app instance")
	}
}
