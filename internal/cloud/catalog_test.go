package cloud

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func fixtureCLI(t *testing.T, script string) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "postman")
	if err := os.WriteFile(file, []byte("#!/bin/sh\nset -eu\n"+script), 0700); err != nil {
		t.Fatal(err)
	}
	return file
}

func TestCatalogListsAllWorkspacePagesThenCollections(t *testing.T) {
	bin := fixtureCLI(t, `case "$*" in
  'workspace list --json') printf '%s' '{"workspaces":[{"id":"personal","name":"My Workspace"}],"cursor":"page-2"}' ;;
  'workspace list --json --cursor page-2') printf '%s' '{"workspaces":[{"id":"team","name":"Team"}],"cursor":null}' ;;
  'collection list --workspace personal --json') printf '%s' '[{"id":"owner-c1","name":"Existing API","type":"collection"}]' ;;
  *) exit 3 ;;
esac
`)
	catalog := CLI{Binary: bin}
	workspaces, err := catalog.Workspaces(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(workspaces, []Resource{{ID: "personal", Name: "My Workspace"}, {ID: "team", Name: "Team"}}) {
		t.Fatalf("workspaces: %#v", workspaces)
	}
	collections, err := catalog.Collections(context.Background(), "personal")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(collections, []Resource{{ID: "owner-c1", Name: "Existing API"}}) {
		t.Fatalf("collections: %#v", collections)
	}
}

func TestCatalogDoesNotExposeRawCLIErrors(t *testing.T) {
	bin := fixtureCLI(t, "printf 'fixture-secret-value' >&2\nexit 1\n")
	_, err := (CLI{Binary: bin}).Workspaces(context.Background())
	if err == nil || strings.Contains(err.Error(), "fixture-secret-value") {
		t.Fatalf("unsafe error: %v", err)
	}
}

func TestCatalogRejectsMalformedResponseInsteadOfShowingAnEmptyAccount(t *testing.T) {
	for _, response := range []string{`{"message":"login needed"}`, `{"workspaces":[{"name":"Missing ID"}]}`} {
		bin := fixtureCLI(t, "printf '%s' '"+response+"'\n")
		if _, err := (CLI{Binary: bin}).Workspaces(context.Background()); err == nil {
			t.Fatal("malformed account shown as an empty or valid workspace list")
		}
	}
}
