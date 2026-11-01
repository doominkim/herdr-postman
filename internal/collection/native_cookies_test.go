package collection

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/darrenburns/posting/v3/internal/model"
	"gopkg.in/yaml.v3"
)

func TestNativeCookieOptionRoundTrip(t *testing.T) {
	for _, attach := range []bool{false, true} {
		t.Run(map[bool]string{false: "disabled", true: "enabled"}[attach], func(t *testing.T) {
			dir := t.TempDir()
			store := NewNativeStore(dir)
			req := model.NewRequest()
			req.File, req.Name, req.URL = "cookies.request.yaml", "Cookies", "https://example.test"
			req.Options.AttachCookies = attach
			if err := store.Create(req); err != nil {
				t.Fatal(err)
			}
			coll, problems := store.Load()
			if len(problems) != 0 || coll.Requests[0].Options.AttachCookies != attach {
				t.Fatal("created cookie option lost", problems)
			}
			loaded := coll.Requests[0]
			loaded.Options.AttachCookies = !attach
			if err := store.Save(loaded); err != nil {
				t.Fatal(err)
			}
			coll, problems = NewNativeStore(dir).Load()
			if len(problems) != 0 || coll.Requests[0].Options.AttachCookies == attach {
				t.Fatal("saved cookie option lost", problems)
			}
			data, err := os.ReadFile(filepath.Join(dir, req.File))
			if err != nil {
				t.Fatal(err)
			}
			var doc map[string]any
			if err := yaml.Unmarshal(data, &doc); err != nil {
				t.Fatal(err)
			}
			settings := doc["settings"].(map[string]any)
			if settings["disableCookies"] != attach || settings["strictSSL"] != true || settings["followRedirects"] != true {
				t.Fatal("wrong Native settings")
			}
		})
	}
}
