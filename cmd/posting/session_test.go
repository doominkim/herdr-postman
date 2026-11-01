package main

import "testing"

func TestSessionVariablesSurviveARestart(t *testing.T) {
	for _, name := range []string{"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_RUNTIME_DIR"} {
		t.Setenv(name, t.TempDir())
	}
	collection := t.TempDir()
	first, err := appConfig(options{collection: collection})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.SessionVariables) != 0 {
		t.Fatalf("a fresh collection has no session variables: %v", first.SessionVariables)
	}
	first.RememberSession(map[string]string{"API_TOKEN": "captured"})

	second, err := appConfig(options{collection: collection})
	if err != nil {
		t.Fatal(err)
	}
	if got := second.SessionVariables["API_TOKEN"]; got != "captured" {
		t.Fatalf("API_TOKEN after a restart = %q, want the captured value", got)
	}
	other, err := appConfig(options{collection: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if len(other.SessionVariables) != 0 {
		t.Fatalf("another collection shouldn't see these variables: %v", other.SessionVariables)
	}
}
