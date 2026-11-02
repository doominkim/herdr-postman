package ui

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	t "github.com/darrenburns/terma"
	uv "github.com/darrenburns/ultraviolet"

	"github.com/darrenburns/posting/v3/internal/client"
	"github.com/darrenburns/posting/v3/internal/model"
)

type countingSender struct{ calls atomic.Int32 }

func (c *countingSender) Send(context.Context, client.Call) (*model.Response, error) {
	c.calls.Add(1)
	return fixedResponse(), nil
}

// postmanBodyApp is the Postman workbench showing a plain-text response.
func postmanBodyApp(tt *testing.T, sender client.Sender, body string) (*App, *Session) {
	tt.Helper()
	app := New(Config{AppName: "herdr-postman", PostmanUI: true, Collection: model.SampleCollection(), Sender: sender})
	tt.Cleanup(func() { app.Close() })
	app.openRequest(sampleRequest(tt, "List users"))
	s := app.current()
	resp := fixedResponse()
	resp.Headers = []model.Header{{Name: "Content-Type", Value: "text/plain"}}
	resp.Body = []byte(body)
	s.showResponse(resp, nil)
	s.phase.Set(exchangeDone)
	return app, s
}

func focusByTab(tt *testing.T, p *t.Pilot, id string) {
	tt.Helper()
	for range 100 {
		if p.FocusedID() == id {
			return
		}
		p.Press("tab")
	}
	tt.Fatalf("couldn't reach %s with tab", id)
}

func waitCalls(p *t.Pilot, sender *countingSender, want int32) bool {
	for range 50 {
		if sender.calls.Load() >= want {
			return true
		}
		time.Sleep(5 * time.Millisecond)
		p.Advance(0)
	}
	return false
}

func TestPostmanCtrlEnterSendsFromEveryPanel(tt *testing.T) {
	for _, id := range []string{urlInputID, "resp-body", "side-tree", requestTabsID, responseTabsID} {
		tt.Run(id, func(tt *testing.T) {
			sender := &countingSender{}
			app, _ := postmanBodyApp(tt, sender, "hello")
			p := t.NewPilot(tt, app, 150, 45)
			focusByTab(tt, p, id)
			p.Press("ctrl+j")
			if waitCalls(p, sender, 1) {
				tt.Fatal("ctrl+j still sends the request")
			}
			p.Press("ctrl+enter")
			if !waitCalls(p, sender, 1) {
				tt.Fatalf("ctrl+enter did not send from %s", id)
			}
		})
	}
}

func TestPostmanFailedResponseHintsCtrlEnter(tt *testing.T) {
	app, s := postmanBodyApp(tt, &countingSender{}, "hello")
	s.err.Set(context.DeadlineExceeded)
	s.phase.Set(exchangeFailed)
	p := t.NewPilot(tt, app, 150, 45)
	screen := p.ScreenText()
	if !strings.Contains(screen, "ctrl+enter") || strings.Contains(screen, "ctrl+j") {
		tt.Fatalf("failure hint should name ctrl+enter only:\n%s", screen)
	}
}

// bodyCell is the screen cell of column col on the first line of the body.
func bodyCell(tt *testing.T, p *t.Pilot, col int) (int, int) {
	tt.Helper()
	b, ok := p.Bounds("resp-body")
	if !ok {
		tt.Fatal("response body not on screen")
	}
	return b.X + 1 + col, b.Y // one cell of horizontal padding
}

func dragBody(tt *testing.T, p *t.Pilot, from, to int) {
	tt.Helper()
	x0, y := bodyCell(tt, p, from)
	x1, _ := bodyCell(tt, p, to)
	p.MouseDown(x0, y, uv.MouseLeft, 0)
	p.MouseMove(x1, y)
	p.MouseUp(x1, y, uv.MouseLeft, 0)
}

func TestPostmanResponseDragCopiesSelection(tt *testing.T) {
	app, _ := postmanBodyApp(tt, &countingSender{}, "hello world\nsecond line")
	p := t.NewPilot(tt, app, 150, 45)
	x, y := bodyCell(tt, p, 2)
	p.MouseDown(x, y, uv.MouseLeft, 0)
	p.MouseUp(x, y, uv.MouseLeft, 0)
	if got := p.Clipboard(); got != "" {
		tt.Fatalf("a click without dragging copied %q", got)
	}
	dragBody(tt, p, 0, 5)
	if got := p.Clipboard(); got != "hello" {
		tt.Fatalf("drag copied %q, want %q", got, "hello")
	}
}

func TestPostmanResponseCopyAllIgnoresSelection(tt *testing.T) {
	body := "hello world\nsecond line"
	app, _ := postmanBodyApp(tt, &countingSender{}, body)
	p := t.NewPilot(tt, app, 150, 45)
	dragBody(tt, p, 0, 5)
	p.Click("resp-copy-all")
	if got := p.Clipboard(); got != body {
		tt.Fatalf("copy all copied %q, want the whole body %q", got, body)
	}
}

func TestPostmanRequestBodyDragCopiesSelection(tt *testing.T) {
	app, s := postmanBodyApp(tt, &countingSender{}, "")
	s.requestTab.Set("body")
	s.bodyType.Set(model.BodyRaw)
	s.body.SetText("{\"name\": 1}")
	p := t.NewPilot(tt, app, 150, 45)
	b, ok := p.Bounds("req-body-text")
	if !ok {
		tt.Fatalf("request body editor not on screen:\n%s", p.ScreenText())
	}
	p.MouseDown(b.X+1, b.Y, uv.MouseLeft, 0)
	p.MouseMove(b.X+8, b.Y)
	p.MouseUp(b.X+8, b.Y, uv.MouseLeft, 0)
	if got := p.Clipboard(); got != "{\"name\"" {
		tt.Fatalf("drag copied %q", got)
	}
}
