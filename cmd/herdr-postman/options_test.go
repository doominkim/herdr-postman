package main

import (
	"bytes"
	"errors"
	"flag"
	"reflect"
	"testing"
)

func TestStartupEnvironmentFlagsPreserveLayerOrder(t *testing.T) {
	var output bytes.Buffer
	opts, err := parseStartupOptions([]string{"--collection", "native collection", "--environment", "base.environment.yaml", "--environment=dev.environment.yaml", "--environment", "base.environment.yaml"}, &output)
	if err != nil {
		t.Fatal(err)
	}
	if opts.root != "native collection" || opts.bin != "postman" || !reflect.DeepEqual(opts.environments, []string{"base.environment.yaml", "dev.environment.yaml", "base.environment.yaml"}) {
		t.Fatal("startup lost environment layering order")
	}
}

func TestStartupOptionsRejectAmbiguousInput(t *testing.T) {
	for _, args := range [][]string{{}, {"--collection", "native", "unexpected"}, {"--collection", "native", "--environment", ""}, {"--collection", "native", "--unknown"}} {
		var output bytes.Buffer
		if _, err := parseStartupOptions(args, &output); err == nil {
			t.Fatalf("accepted invalid flags: %v", args)
		}
	}
	var output bytes.Buffer
	if _, err := parseStartupOptions([]string{"--help"}, &output); !errors.Is(err, flag.ErrHelp) {
		t.Fatal("help is not distinguished from an invalid invocation")
	}
}

func TestCloudStartupRequiresExplicitUnambiguousSource(t *testing.T) {
	for _, args := range [][]string{
		{"--source", "cloud", "--workspace", "w1", "--cloud-collection", "c1"},
		{"--source", "choose", "--collection", "local"},
	} {
		if _, err := parseStartupOptions(args, &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{
		{"--source", "cloud", "--cloud-collection", "c1"},
		{"--source", "cloud", "--workspace", "w1", "--cloud-collection", "c1", "--collection", "local"},
		{"--source", "local", "--collection", "local", "--cloud-collection", "c1"},
		{"--source", "unknown", "--collection", "local"},
	} {
		if _, err := parseStartupOptions(args, &bytes.Buffer{}); err == nil {
			t.Fatal("ambiguous source accepted", args)
		}
	}
}
