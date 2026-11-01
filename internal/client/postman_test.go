package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"testing"

	"github.com/darrenburns/posting/v3/internal/model"
)

func TestPostmanRequestSendsEditedJSONHeadersAndParameters(t *testing.T) {
	bin, err := exec.LookPath("postman")
	if err != nil {
		t.Skip("Postman CLI not installed")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		if r.Method != "POST" || r.URL.Path != "/users/42" || r.URL.Query().Get("locale") != "ko" || r.URL.Query().Has("disabled") || r.Header.Get("X-Trace") != "yes" || r.Header.Get("X-Disabled") != "" || string(data) != `{"name":"Alice"}` {
			t.Errorf("wire request mismatch: %s %s %v %s", r.Method, r.URL, r.Header, data)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	req := model.NewRequest()
	req.Method = model.MethodPost
	req.URL = server.URL + "/users/:id"
	req.PathParams = []model.KeyValue{{Name: "id", Value: "42", Enabled: true}}
	req.Query = []model.KeyValue{{Name: "locale", Value: "ko", Enabled: true}, {Name: "disabled", Value: "never", Enabled: false}}
	req.Headers = []model.KeyValue{{Name: "X-Trace", Value: "yes", Enabled: true}, {Name: "X-Disabled", Value: "never", Enabled: false}}
	req.Body = model.Body{Type: model.BodyRaw, ContentType: "application/json", Raw: `{"name":"Alice"}`}
	response, err := (&PostmanRequest{Binary: bin}).Send(context.Background(), Call{Request: req})
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 200 {
		t.Fatalf("status %d", response.StatusCode)
	}
}

func TestPostmanRequestReturnsActualResponse(t *testing.T) {
	bin, err := exec.LookPath("postman")
	if err != nil {
		t.Skip("Postman CLI not installed")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Query().Get("id") != "42" {
			t.Errorf("wrong request: %s %s", r.Method, r.URL)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Add("X-Result", "native")
		w.WriteHeader(201)
		w.Write([]byte(`{"id":42}`))
	}))
	defer server.Close()
	req := model.NewRequest()
	req.URL = server.URL + "?id=42"
	sender := &PostmanRequest{Binary: bin}
	response, err := sender.Send(context.Background(), Call{Request: req})
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 201 || string(response.Body) != `{"id":42}` || response.Header("X-Result") != "native" {
		t.Fatalf("response mismatch: %+v", response)
	}
	if response.Elapsed <= 0 || response.Method != model.MethodGet {
		t.Fatal("response metadata missing")
	}
}
