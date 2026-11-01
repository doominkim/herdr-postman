package ui

import (
	t "github.com/darrenburns/terma"
	"regexp"
	"strings"
	"testing"
)

func TestSeparateApplicationNameIsRendered(tt *testing.T) {
	app := New(Config{AppName: "herdr-postman", Version: "dev"})
	defer app.Close()
	buffer := t.RenderToBuffer(app, 130, 36)
	svg := t.BufferToSVG(buffer, 130, 36, t.DefaultSVGOptions())
	var text strings.Builder
	for _, match := range regexp.MustCompile(`<text[^>]*>([^<]*)</text>`).FindAllStringSubmatch(svg, -1) {
		text.WriteString(match[1])
	}
	if !strings.Contains(text.String(), "herdr-postman") {
		tt.Fatal("separate application name missing")
	}
}
