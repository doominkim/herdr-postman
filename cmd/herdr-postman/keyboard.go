package main

import "os"

// enableModifierKeys asks Terma to turn on the Kitty keyboard protocol, so the
// terminal reports modified keys unambiguously: ctrl+enter is not read as
// enter, ctrl+h not as backspace, ctrl+shift+p not as ctrl+p. Without it the
// shortcuts behave differently depending on which panel has focus. Terma
// leaves the protocol off by default; an explicit TERMA_ENABLE_KITTY_KEYBOARD
// or TERMA_DISABLE_KITTY_KEYBOARD setting still decides.
func enableModifierKeys() {
	if _, set := os.LookupEnv("TERMA_ENABLE_KITTY_KEYBOARD"); set {
		return
	}
	if _, set := os.LookupEnv("TERMA_DISABLE_KITTY_KEYBOARD"); set {
		return
	}
	os.Setenv("TERMA_ENABLE_KITTY_KEYBOARD", "1")
}
