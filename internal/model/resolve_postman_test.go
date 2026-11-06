package model

import (
	"reflect"
	"testing"
)

func TestFindPostmanVariables(t *testing.T) {
	refs := FindPostmanVariables("{{base}}/${literal}/{{}}/{{a b}}/{{bad{x}}/{{id}}")
	want := []VariableRef{{Name: "base", Start: 0, End: 8}, {Name: "a b", Start: 25, End: 32}, {Name: "id", Start: 43, End: 49}}
	if !reflect.DeepEqual(refs, want) {
		t.Fatalf("refs = %+v, want %+v", refs, want)
	}
}

func TestResolvePostmanFillsKnownBracesAndLeavesTheRestForPostman(t *testing.T) {
	req := NewRequest()
	req.Method = MethodPost
	req.URL = "{{base_url}}/users/:id?q={{term}}&cost=$5"
	req.PathParams = []KeyValue{{Name: "id", Value: "{{user_id}}", Enabled: true}}
	req.Headers = []KeyValue{{Name: "X-Trace", Value: "{{$guid}}", Enabled: true}, {Name: "X-Env", Value: "${HOME}", Enabled: true}}
	req.Auth = Auth{Type: AuthBearer, Token: "{{token}}"}
	req.Body = Body{Type: BodyRaw, Raw: `{"name": "{{name}}", "later": "{{created_by_script}}"}`, ContentType: "application/json"}
	// Postman substitutes the body whatever Posting's option says.
	req.Options.SubstituteBodyVariables = false
	got := ResolvePostman(req, lookup(map[string]string{
		"base_url": "https://api.test", "user_id": "42", "term": "ada", "token": "t0k", "name": "Ada",
	}))

	// The query stays as written, as Postman sends it, rather than re-encoded.
	if want := "https://api.test/users/42?q=ada&cost=$5"; got.URL != want {
		t.Errorf("URL = %q, want %q", got.URL, want)
	}
	if got.Headers[0].Value != "{{$guid}}" || got.Headers[1].Value != "${HOME}" {
		t.Errorf("headers = %+v; unknown and $ references must stay as written", got.Headers)
	}
	if got.Auth.Token != "t0k" {
		t.Errorf("token = %q", got.Auth.Token)
	}
	if want := `{"name": "Ada", "later": "{{created_by_script}}"}`; got.Body.Raw != want {
		t.Errorf("body = %q, want %q", got.Body.Raw, want)
	}
	if len(got.Query) != 0 {
		t.Errorf("query = %+v, want it merged into the URL", got.Query)
	}
}

func TestResolvePostmanKeepsAnUnresolvedHostAsWritten(t *testing.T) {
	req := NewRequest()
	req.URL = "{{missing}}/items"
	if got := ResolvePostman(req, lookup(nil)).URL; got != "{{missing}}/items" {
		t.Fatalf("URL = %q; Postman resolves it at execution, so no scheme may be guessed", got)
	}
}
