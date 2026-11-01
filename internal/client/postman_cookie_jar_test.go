package client

import (
	"encoding/json"
	"testing"
)

func TestPostmanCookieJarMergesConcurrentDeltas(t *testing.T) {
	var jar postmanCookieJar
	initial := []byte(`{"version":"tough-cookie@fixture","cookies":[{"key":"old","value":"old","domain":"example.test","path":"/"}]}`)
	if err := jar.merge(nil, initial); err != nil {
		t.Fatal(err)
	}
	base := jar.snapshot()
	first := []byte(`{"version":"tough-cookie@fixture","cookies":[{"key":"old","value":"old","domain":"example.test","path":"/"},{"key":"a","value":"one","domain":"example.test","path":"/"}]}`)
	second := []byte(`{"version":"tough-cookie@fixture","cookies":[{"key":"b","value":"two","domain":"example.test","path":"/"}]}`)
	if err := jar.merge(base, first); err != nil {
		t.Fatal(err)
	}
	if err := jar.merge(base, second); err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Cookies []struct {
			Key string `json:"key"`
		}
	}
	json.Unmarshal(jar.snapshot(), &doc)
	keys := map[string]bool{}
	for _, cookie := range doc.Cookies {
		keys[cookie.Key] = true
	}
	if len(keys) != 2 || !keys["a"] || !keys["b"] {
		t.Fatal("concurrent changes lost or deletion ignored")
	}
	before := string(jar.snapshot())
	if jar.merge(nil, []byte("invalid")) == nil {
		t.Fatal("invalid jar accepted")
	}
	if string(jar.snapshot()) != before {
		t.Fatal("invalid jar replaced valid state")
	}
}

func TestPostmanCookieJarReadDoesNotOverwriteConcurrentUpdate(t *testing.T) {
	var jar postmanCookieJar
	initial := []byte(`{"cookies":[{"key":"session","value":"old","domain":"example.test","path":"/","lastAccessed":"2026-01-01T00:00:00Z"}]}`)
	updated := []byte(`{"cookies":[{"key":"session","value":"new","domain":"example.test","path":"/","lastAccessed":"2026-01-01T00:00:01Z"}]}`)
	accessed := []byte(`{"cookies":[{"key":"session","value":"old","domain":"example.test","path":"/","lastAccessed":"2026-01-01T00:00:02Z"}]}`)
	if err := jar.merge(nil, initial); err != nil {
		t.Fatal(err)
	}
	base := jar.snapshot()
	if err := jar.merge(base, updated); err != nil {
		t.Fatal(err)
	}
	if err := jar.merge(base, accessed); err != nil {
		t.Fatal(err)
	}
	_, records, err := parsePostmanJar(jar.snapshot())
	if err != nil {
		t.Fatal(err)
	}
	if records["example.test\x00/\x00session"]["value"] != "new" {
		t.Fatal("read-only run overwrote concurrent cookie update")
	}
}
