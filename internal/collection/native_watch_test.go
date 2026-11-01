package collection

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNativeSignatureDetectsGUIChanges(t *testing.T) {
	dir := t.TempDir()
	store := NewNativeStore(dir)
	initial := store.Signature()
	file := filepath.Join(dir, "get.request.yaml")
	write := func(p, body string) {
		t.Helper()
		if err := os.WriteFile(p, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(file, "$kind: http-request\nurl: https://example.test/a\n")
	created := store.Signature()
	if created == initial {
		t.Fatal("new request ignored")
	}
	info, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	write(file, "$kind: http-request\nurl: https://example.test/b\n")
	if err := os.Chtimes(file, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	edited := store.Signature()
	if edited == created {
		t.Fatal("same-size same-time edit ignored")
	}
	if err := os.Mkdir(filepath.Join(dir, ".resources"), 0700); err != nil {
		t.Fatal(err)
	}
	write(filepath.Join(dir, ".resources", "definition.yaml"), "$kind: collection\nname: Changed\n")
	metadata := store.Signature()
	if metadata == edited {
		t.Fatal("collection metadata edit ignored")
	}
	write(filepath.Join(dir, "notes.txt"), "not part of Postman")
	if store.Signature() != metadata {
		t.Fatal("unrelated file changes signature")
	}
	if err := os.Rename(file, filepath.Join(dir, "renamed.request.yaml")); err != nil {
		t.Fatal(err)
	}
	renamed := store.Signature()
	if renamed == metadata {
		t.Fatal("rename ignored")
	}
	if err := os.Remove(filepath.Join(dir, "renamed.request.yaml")); err != nil {
		t.Fatal(err)
	}
	if store.Signature() == renamed {
		t.Fatal("deletion ignored")
	}
}
