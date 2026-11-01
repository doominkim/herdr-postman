package collection

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestNativeDuplicateHasIndependentItemID(t *testing.T) {
	dir := t.TempDir()
	source := "$kind: http-request\nid: original-id\nname: Original\nmethod: GET\nurl: https://example.test\n"
	if err := os.WriteFile(filepath.Join(dir, "original.request.yaml"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	store := NewNativeStore(dir)
	coll, problems := store.Load()
	if len(problems) != 0 {
		t.Fatal(problems)
	}
	req := coll.Requests[0]
	req.Name = "Copy"
	copy, err := store.Duplicate(req, "copy.request.yaml")
	if err != nil {
		t.Fatal(err)
	}
	data, err := store.Render(copy)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := yaml.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	id, _ := document["id"].(string)
	if id == "" || id == "original-id" {
		t.Fatal("copy must have its own stable item ID")
	}
	copy.URL = "https://example.test/edited"
	if err := store.Save(copy); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(filepath.Join(dir, copy.File))
	if err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	if document["id"] != id {
		t.Fatal("editing copy changed its ID")
	}
	data, err = os.ReadFile(filepath.Join(dir, "original.request.yaml"))
	if err != nil || string(data) != source {
		t.Fatal("copy changed original identity")
	}
}

func TestNativeDuplicateOwnsExamples(t *testing.T) {
	dir := t.TempDir()
	examples := filepath.Join(dir, ".resources/get.resources/examples")
	if err := os.MkdirAll(examples, 0700); err != nil {
		t.Fatal(err)
	}
	original := []byte("$kind: http-example\nname: fixture\nresponse:\n  statusCode: 200\n  statusText: OK\n  body:\n    type: text\n    content: fixture\n")
	if err := os.WriteFile(filepath.Join(examples, "ok.example.yaml"), original, 0600); err != nil {
		t.Fatal(err)
	}
	source := "$kind: http-request\nname: Get\nmethod: GET\nurl: https://example.test\nexamples: ./.resources/get.resources/examples/\n"
	if err := os.WriteFile(filepath.Join(dir, "get.request.yaml"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	store := NewNativeStore(dir)
	coll, problems := store.Load()
	if len(problems) > 0 {
		t.Fatal(problems)
	}
	req := coll.Requests[0]
	req.Name = "Get (copy)"
	dup, err := store.Duplicate(req, "get-copy.request.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Duplicate(req, "get-copy.request.yaml"); !os.IsExist(err) {
		t.Fatalf("expected copy collision, got %v", err)
	}
	resources, err := os.ReadDir(filepath.Join(dir, ".resources"))
	if err != nil || len(resources) != 2 {
		t.Fatal("collision leaked a resource directory", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, dup.File))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	ref, ok := doc["examples"].(string)
	if !ok || strings.Contains(ref, "/get.resources/") {
		t.Fatal("copy still shares original examples")
	}
	if bin, err := exec.LookPath("postman"); err == nil {
		canonical, err := filepath.EvalSymlinks(dir)
		if err != nil {
			t.Fatal(err)
		}
		command := exec.Command(bin, "collection", "lint", canonical)
		if out, err := command.CombinedOutput(); err != nil {
			t.Fatalf("CLI rejected copy: %v %s", err, out)
		}
		command = exec.Command(bin, "collection", "request", "get", "Get (copy)", "--collection", canonical, "--json")
		command.Dir = canonical
		if out, err := command.CombinedOutput(); err != nil || !strings.Contains(string(out), "example.test") {
			t.Fatalf("CLI could not read copy: %v %s", err, out)
		}
	}
	copied := filepath.Join(dir, filepath.FromSlash(ref), "ok.example.yaml")
	got, err := os.ReadFile(copied)
	if err != nil || string(got) != string(original) {
		t.Fatal("example bytes not preserved", err)
	}
	if err := os.WriteFile(copied, []byte("copy edit"), 0600); err != nil {
		t.Fatal(err)
	}
	got, _ = os.ReadFile(filepath.Join(examples, "ok.example.yaml"))
	if string(got) != string(original) {
		t.Fatal("copy edit changed source")
	}
	dup.URL = "https://example.test/new"
	if err := store.Save(dup); err != nil {
		t.Fatal(err)
	}
}

func TestNativeDuplicateFailureLeavesNoCopy(t *testing.T) {
	for _, scenario := range []string{"missing-examples", "symlink-examples", "recursive-examples", "changed-source", "existing-copy"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			source := "$kind: http-request\nname: Get\nmethod: GET\nurl: https://example.test\n"
			switch scenario {
			case "missing-examples":
				source += "examples: ./missing/\n"
			case "symlink-examples":
				if err := os.Symlink(t.TempDir(), filepath.Join(dir, "linked")); err != nil {
					t.Fatal(err)
				}
				source += "examples: ./linked/\n"
			case "recursive-examples":
				source += "examples: ./\n"
			}
			file := filepath.Join(dir, "get.request.yaml")
			if err := os.WriteFile(file, []byte(source), 0600); err != nil {
				t.Fatal(err)
			}
			store := NewNativeStore(dir)
			coll, problems := store.Load()
			if len(problems) > 0 {
				t.Fatal(problems)
			}
			if scenario == "changed-source" {
				if err := os.WriteFile(file, []byte(source+"description: GUI change\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			destination := filepath.Join(dir, "get-copy.request.yaml")
			if scenario == "existing-copy" {
				if err := os.WriteFile(destination, []byte("existing"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := store.Duplicate(coll.Requests[0], "get-copy.request.yaml"); err == nil {
				t.Fatal("invalid copy accepted")
			}
			got, err := os.ReadFile(destination)
			if scenario == "existing-copy" {
				if err != nil || string(got) != "existing" {
					t.Fatal("existing copy overwritten")
				}
			} else if !os.IsNotExist(err) {
				t.Fatal("failed duplicate created file")
			}
			entries, _ := os.ReadDir(filepath.Join(dir, ".resources"))
			if len(entries) > 0 {
				t.Fatal("failed copy left resources")
			}
		})
	}
}
