package paths

import (
	"path/filepath"
	"testing"
)

// A separate application must never overwrite Posting's configuration or state.
func TestHerdrPostmanUsesIndependentStorage(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(root, "run"))
	t.Setenv("POSTING_CONFIG_FILE", filepath.Join(root, "posting-private.yaml"))
	t.Setenv("HERDR_POSTMAN_CONFIG_FILE", "")
	for name, pair := range map[string][2]string{
		"config":      {ConfigDir(), filepath.Join(root, "config", "herdr-postman")},
		"data":        {DataDir(), filepath.Join(root, "data", "herdr-postman")},
		"socket":      {SocketDir(), filepath.Join(root, "run", "herdr-postman")},
		"config file": {ConfigFile(), filepath.Join(root, "config", "herdr-postman", "config.yaml")},
	} {
		if pair[0] != pair[1] {
			t.Errorf("%s: got %q, want %q", name, pair[0], pair[1])
		}
	}
}
