package collection

import (
	"github.com/darrenburns/posting/v3/internal/model"
	"os"
	"path/filepath"
	"testing"
)

func TestNativeInheritedVariablesFollowFolderAndDisabledState(t *testing.T) {
	dir := t.TempDir()
	write := func(file, body string) {
		t.Helper()
		full := filepath.Join(dir, file)
		os.MkdirAll(filepath.Dir(full), 0700)
		if err := os.WriteFile(full, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(".resources/definition.yaml", "$kind: collection\nvariables:\n  - key: root_only\n    value: root\n  - key: shared\n    value: root\n  - key: ignored\n    value: hidden\n    disabled: true\n")
	write("folder/.resources/definition.yaml", "$kind: collection\nvariables:\n  - key: shared\n    value: folder\n")
	write("folder/get.request.yaml", "$kind: http-request\nmethod: GET\nurl: '{{shared}}'\n")
	store := NewNativeStore(dir)
	_, problems := store.Load()
	if len(problems) > 0 {
		t.Fatal(problems)
	}
	root := model.Values(store.Variables(""))
	nested := model.Values(store.Variables("folder/get.request.yaml"))
	if root["shared"] != "root" || nested["shared"] != "root" || nested["root_only"] != "root" {
		t.Fatal("wrong native variable scope")
	}
	if _, ok := nested["ignored"]; ok {
		t.Fatal("disabled variable included")
	}
	write(".resources/definition.yaml", "$kind: collection\nvariables:\n  - key: shared\n    value: changed\n")
	store.Load()
	if model.Values(store.Variables("folder/get.request.yaml"))["shared"] != "changed" {
		t.Fatal("metadata refresh lost")
	}
	write(".resources/definition.yaml", "variables: [broken")
	_, problems = store.Load()
	if len(problems) == 0 {
		t.Fatal("invalid metadata ignored")
	}
	if model.Values(store.Variables("folder/get.request.yaml"))["shared"] != "changed" {
		t.Fatal("invalid metadata replaced last valid variables")
	}
}
