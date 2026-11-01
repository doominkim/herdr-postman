package env

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeEnvironmentDiscoveryAndLayering(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		t.Helper()
		file := filepath.Join(dir, name)
		if err := os.WriteFile(file, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		return file
	}
	first := write("dev.environment.yaml", "name: Development\nvalues:\n  - key: base_url\n    value: https://dev.test\n    enabled: true\n  - key: disabled\n    value: hidden\n    enabled: false\n  - key: empty\n    value: ''\n")
	second := write("override.environment.yaml", "name: Override\nvalues:\n  - key: base_url\n    value: https://override.test\n")
	write("ignored.yaml", "name: ignored\nvalues: []\n")
	source := NativeFiles{Roots: []string{dir}}
	if len(source.Candidates()) != 2 {
		t.Fatal("discovery included unrelated files")
	}
	loaded, err := source.Load([]string{first, second})
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Name != "Development + Override" || len(loaded.Variables) != 2 {
		t.Fatal("environment layers or disabled values incorrect")
	}
	values := map[string]string{}
	for _, v := range loaded.Variables {
		values[v.Name] = v.Value
	}
	if values["base_url"] != "https://override.test" {
		t.Fatal("later environment did not override")
	}
	if _, ok := values["empty"]; !ok {
		t.Fatal("empty value lost")
	}
}

func TestNativeEnvironmentRejectsInvalidSchemaWithoutEchoingValues(t *testing.T) {
	for _, body := range []string{"name: X\nvalues:\n  - key: token\n    value: 12345\n", "name: X\nvalues:\n  - key: token\n    value: secret-fixture\n    enabled: invalid\n", "name: X\n"} {
		file := filepath.Join(t.TempDir(), "invalid.environment.yaml")
		os.WriteFile(file, []byte(body), 0600)
		_, err := (NativeFiles{}).Load([]string{file})
		if err == nil {
			t.Fatal("invalid environment accepted")
		}
		if strings.Contains(err.Error(), "secret-fixture") || strings.Contains(err.Error(), "12345") {
			t.Fatal("error leaked a value")
		}
	}
}
