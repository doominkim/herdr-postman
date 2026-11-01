package collection

import (
	"encoding/json"
	"io"
	"strings"

	"github.com/darrenburns/posting/v3/internal/model"
)

// GraphQL uses Posting's existing HTTP lowering, so Desktop and the CLI
// see a standard POST request rather than a plan-specific protocol entity.
func nativeWireRequest(req model.Request) model.Request {
	if graphql, ok := req.Payload.(model.GraphQL); ok {
		return graphql.Lower(req)
	}
	return req
}

// Recognize only explicitly GraphQL-negotiating HTTP requests with a lossless
// envelope. Other JSON requests stay in the raw body editor.
func nativeGraphQLEditor(req model.Request) model.Request {
	if req.Method != model.MethodPost || req.Body.Type != model.BodyRaw || req.Body.ContentType != "application/json" {
		return req
	}
	graphqlAccept := false
	for _, header := range req.Headers {
		if header.Enabled && strings.EqualFold(header.Name, "Accept") && strings.Contains(strings.ToLower(header.Value), "application/graphql-response+json") {
			graphqlAccept = true
		}
	}
	if !graphqlAccept {
		return req
	}
	fields, valid := nativeGraphQLFields(req.Body.Raw)
	if !valid {
		return req
	}
	for name := range fields {
		if name != "query" && name != "variables" && name != "operationName" {
			return req
		}
	}
	var graphql model.GraphQL
	if raw, ok := fields["query"]; !ok || string(raw) == "null" || json.Unmarshal(raw, &graphql.Query) != nil {
		return req
	}
	if raw, ok := fields["variables"]; ok {
		var variables map[string]json.RawMessage
		if json.Unmarshal(raw, &variables) != nil || variables == nil {
			return req
		}
		graphql.Variables = string(raw)
	}
	if raw, ok := fields["operationName"]; ok {
		if string(raw) == "null" || json.Unmarshal(raw, &graphql.OperationName) != nil {
			return req
		}
	}
	req.Payload = graphql
	req.Body = model.Body{}
	return req
}

// Duplicate JSON keys cannot be represented by the structured editor without
// silently dropping a value, so retain such documents as raw HTTP bodies.
func nativeGraphQLFields(raw string) (map[string]json.RawMessage, bool) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, false
	}
	fields := map[string]json.RawMessage{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, false
		}
		name, ok := token.(string)
		if !ok {
			return nil, false
		}
		if _, exists := fields[name]; exists {
			return nil, false
		}
		var value json.RawMessage
		if decoder.Decode(&value) != nil {
			return nil, false
		}
		fields[name] = value
	}
	if token, err = decoder.Token(); err != nil || token != json.Delim('}') {
		return nil, false
	}
	if _, err = decoder.Token(); err != io.EOF {
		return nil, false
	}
	return fields, true
}
