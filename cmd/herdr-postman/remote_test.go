package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darrenburns/posting/v3/internal/collection"
	"github.com/darrenburns/posting/v3/internal/config"
	"github.com/darrenburns/posting/v3/internal/model"
	"github.com/darrenburns/posting/v3/internal/remote"
	"github.com/darrenburns/posting/v3/internal/ui"
)

func TestNativeRemoteSocketAndCommandDiscovery(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "hpm-remote-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	t.Setenv("XDG_RUNTIME_DIR", dir)
	settings := config.Defaults()
	cfg := ui.Config{Store: collection.NewNativeStore(t.TempDir()), Settings: &settings, Version: "fixture"}
	server, err := listenForRemote(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	server.Serve(remote.HandlerFunc(func(context.Context, remote.Request) (any, error) { return nil, nil }))
	var info remote.Info
	if err := remote.Call(context.Background(), server.Path(), remote.Request{Command: remote.CommandInfo}, &info); err != nil {
		t.Fatal(err)
	}
	if info.Collection != cfg.Store.(*collection.NativeStore).Root || info.Version != "fixture" {
		t.Fatal("native collection missing from instance discovery")
	}
	if filepath.Dir(server.Path()) != filepath.Join(dir, "herdr-postman") {
		t.Fatal("socket not isolated")
	}
	var out, stderr bytes.Buffer
	if code := remoteCommand([]string{"list", "--json"}, strings.NewReader(""), &out, &stderr); code != 0 {
		t.Fatal(stderr.String())
	}
	if !strings.Contains(out.String(), info.Collection) {
		t.Fatal("remote CLI did not discover native app")
	}
	settings.RemoteControl.Enabled = false
	disabled, err := listenForRemote(cfg)
	if err != nil || disabled != nil {
		t.Fatal("disabled remote control opened a socket")
	}
}

func TestRemotePrintSeparatesRunFailureFromResponse(t *testing.T) {
	response := remote.NewResponse(&model.Response{StatusCode: 200, Body: []byte("fixture body"), PostmanRun: &model.PostmanRun{ExitCode: 1, Output: "fixture assertion failed"}}, model.Status{Code: "200", Text: "OK"})
	exchange := remote.Exchange{Outcome: remote.OutcomeDone, Response: response}
	var out, stderr bytes.Buffer
	if code := printExchange(exchange, false, &out, &stderr); code != 0 {
		t.Fatal("received HTTP response changed remote exit contract")
	}
	if !strings.Contains(out.String(), "200 OK") || !strings.Contains(out.String(), "fixture body") {
		t.Fatal("HTTP response missing")
	}
	if !strings.Contains(stderr.String(), "Postman run failed (exit 1)") || !strings.Contains(stderr.String(), "fixture assertion failed") {
		t.Fatal("run failure hidden from text output")
	}
	out.Reset()
	stderr.Reset()
	if code := printExchange(exchange, true, &out, &stderr); code != 0 {
		t.Fatal("JSON response changed remote exit contract")
	}
	if out.Len() != 0 || stderr.Len() != 0 {
		t.Fatal("JSON status handling added output after serialization")
	}
}
