package main

import (
	"os"
	"testing"
)

func TestModifierKeysEnabledUnlessTheEnvironmentDecides(t *testing.T) {
	for _, tc := range []struct {
		name, preset, value string
		want             string
		wantSet          bool
	}{
		{name: "default", want: "1", wantSet: true},
		{name: "explicit opt-out", preset: "TERMA_DISABLE_KITTY_KEYBOARD", value: "1"},
		{name: "explicit choice", preset: "TERMA_ENABLE_KITTY_KEYBOARD", value: "0", want: "0", wantSet: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("TERMA_ENABLE_KITTY_KEYBOARD", "")
			t.Setenv("TERMA_DISABLE_KITTY_KEYBOARD", "")
			os.Unsetenv("TERMA_ENABLE_KITTY_KEYBOARD")
			os.Unsetenv("TERMA_DISABLE_KITTY_KEYBOARD")
			if tc.preset != "" {
				t.Setenv(tc.preset, tc.value)
			}
			enableModifierKeys()
			got, set := os.LookupEnv("TERMA_ENABLE_KITTY_KEYBOARD")
			if set != tc.wantSet || got != tc.want {
				t.Fatalf("TERMA_ENABLE_KITTY_KEYBOARD = %q (set %v), want %q (set %v)", got, set, tc.want, tc.wantSet)
			}
		})
	}
}
