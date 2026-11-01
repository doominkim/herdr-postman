package collection

import (
	"github.com/darrenburns/posting/v3/internal/model"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeAuthLoadsAndEditsCredentials(t *testing.T) {
	for _, scheme := range []string{"basic", "digest", "bearer"} {
		t.Run(scheme, func(t *testing.T) {
			dir := t.TempDir()
			file := filepath.Join(dir, "get.request.yaml")
			input := "$kind: http-request\nmethod: GET\nurl: https://example.test\nauth:\n  type: " + scheme + "\n  credentials:\n    - key: username\n      value: '{{user}}'\n    - key: password\n      value: '{{pass}}'\n    - key: token\n      value: '{{old_token}}'\n    - key: realm\n      value: keep-realm\n"
			if err := os.WriteFile(file, []byte(input), 0600); err != nil {
				t.Fatal(err)
			}
			store := NewNativeStore(dir)
			coll, problems := store.Load()
			if len(problems) > 0 {
				t.Fatal(problems)
			}
			req := coll.Requests[0]
			if scheme == "bearer" {
				if req.Auth.Type != model.AuthBearer || req.Auth.Token != "{{old_token}}" {
					t.Fatal("bearer not loaded")
				}
				req.Auth.Token = "{{new_token}}"
			} else {
				if string(req.Auth.Type) != scheme || req.Auth.Username != "{{user}}" {
					t.Fatal("credentials not loaded")
				}
				req.Auth.Username = "{{new_user}}"
			}
			if err := store.Save(req); err != nil {
				t.Fatal(err)
			}
			data, _ := os.ReadFile(file)
			if !strings.Contains(string(data), "keep-realm") || !strings.Contains(string(data), "{{new_") {
				t.Fatal("credential edit lost metadata")
			}
		})
	}
}

func TestNativeAuthDistinguishesInheritedNoneAndManaged(t *testing.T) {
	for _, tc := range []struct {
		yaml string
		want model.AuthType
	}{{"", model.AuthInherit}, {"auth:\n  type: noauth\n", model.AuthNone}, {"auth:\n  type: oauth2\n  credentials: []\n", model.AuthManaged}} {
		t.Run(string(tc.want), func(t *testing.T) {
			dir := t.TempDir()
			file := filepath.Join(dir, "get.request.yaml")
			os.WriteFile(file, []byte("$kind: http-request\nmethod: GET\nurl: https://example.test\n"+tc.yaml), 0600)
			store := NewNativeStore(dir)
			coll, _ := store.Load()
			req := coll.Requests[0]
			if req.Auth.Type != tc.want {
				t.Fatalf("got %s", req.Auth.Type)
			}
			req.Auth = model.Auth{Type: model.AuthNone}
			if err := store.Save(req); err != nil {
				t.Fatal(err)
			}
			data, _ := os.ReadFile(file)
			if !strings.Contains(string(data), "type: noauth") || strings.Contains(string(data), "credentials:") {
				t.Fatal("explicit noauth not written")
			}
			req.Auth = model.Auth{Type: model.AuthInherit}
			if err := store.Save(req); err != nil {
				t.Fatal(err)
			}
			data, _ = os.ReadFile(file)
			if strings.Contains(string(data), "auth:") {
				t.Fatal("inherit should omit request auth")
			}
		})
	}
}
