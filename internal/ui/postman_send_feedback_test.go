package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	t "github.com/darrenburns/terma"

	"github.com/darrenburns/posting/v3/internal/client"
	"github.com/darrenburns/posting/v3/internal/model"
)

// heldSender answers only once released, or fails when cancelled.
type heldSender struct{ release chan struct{} }

func (h heldSender) Send(ctx context.Context, _ client.Call) (*model.Response, error) {
	select {
	case <-h.release:
		resp := fixedResponse()
		resp.Body = []byte("fresh body")
		return resp, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func settleUntil(p *t.Pilot, cond func() bool) bool {
	for range 100 {
		if cond() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
		p.Advance(0)
	}
	return false
}

func TestPostmanSendShowsProgressInsteadOfTheOldResponse(tt *testing.T) {
	sender := heldSender{release: make(chan struct{})}
	app, s := postmanBodyApp(tt, sender, "old body")
	p := t.NewPilot(tt, app, 150, 45)
	p.Click(sendButtonID)
	if !settleUntil(p, func() bool { return strings.Contains(p.ScreenText(), "Sending request") }) {
		tt.Fatalf("no progress while sending:\n%s", p.ScreenText())
	}
	if strings.Contains(p.ScreenText(), "old body") {
		tt.Fatal("the previous response still shows while sending")
	}
	if got := strings.TrimSpace(p.TextOf(sendButtonID)); !strings.Contains(got, "Cancel") {
		tt.Fatalf("send button while sending = %q", got)
	}
	close(sender.release)
	if !settleUntil(p, func() bool { return strings.Contains(p.ScreenText(), "fresh body") }) {
		tt.Fatalf("new response not shown:\n%s", p.ScreenText())
	}
	if got := strings.TrimSpace(p.TextOf(sendButtonID)); !strings.Contains(got, "Send") || s.phase.Peek() == exchangeSending {
		tt.Fatalf("send button after the response = %q", got)
	}
}

func TestPostmanCancelButtonStopsTheRequest(tt *testing.T) {
	sender := heldSender{release: make(chan struct{})}
	app, s := postmanBodyApp(tt, sender, "old body")
	p := t.NewPilot(tt, app, 150, 45)
	p.Click(sendButtonID)
	if !settleUntil(p, func() bool { return s.phase.Peek() == exchangeSending }) {
		tt.Fatal("request never started")
	}
	p.Click(sendButtonID)
	if !settleUntil(p, func() bool { return s.phase.Peek() != exchangeSending }) {
		tt.Fatal("clicking Cancel did not stop the request")
	}
	if got := strings.TrimSpace(p.TextOf(sendButtonID)); !strings.Contains(got, "Send") {
		tt.Fatalf("button after cancelling = %q", got)
	}
}
