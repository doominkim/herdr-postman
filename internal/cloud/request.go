package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
)

// Target is an explicit cloud identity; local paths are never accepted implicitly.
type Target struct{ Workspace, Collection, Request string }

func (target Target) valid() bool {
	return target.Workspace != "" && target.Collection != "" && target.Request != ""
}

func (c CLI) Request(ctx context.Context, target Target) (json.RawMessage, error) {
	if !target.valid() {
		return nil, fmt.Errorf("select a cloud workspace, collection and request")
	}
	var result json.RawMessage
	if err := c.read(ctx, &result, "collection", "request", "get", target.Request, "--collection", target.Collection, "--workspace", target.Workspace, "--json"); err != nil {
		return nil, err
	}
	item, err := requestObject(result)
	if err != nil {
		return nil, err
	}
	if item["id"] != target.Request {
		return nil, fmt.Errorf("Postman Cloud returned a different request identity")
	}
	return result, nil
}

func requestObject(data json.RawMessage) (map[string]any, error) {
	var item map[string]any
	if json.Unmarshal(data, &item) != nil || item == nil || item["$kind"] != "http-request" {
		return nil, fmt.Errorf("expected a Postman Cloud HTTP request")
	}
	return item, nil
}

// SaveRequest performs a preflight conflict check, patches only edited fields,
// then reads the canonical result. This is not an atomic compare-and-swap: the
// CLI exposes no conditional-write option, so simultaneous edits can still race.
func (c CLI) SaveRequest(ctx context.Context, target Target, baseline, draft json.RawMessage) (json.RawMessage, error) {
	if !target.valid() {
		return nil, fmt.Errorf("select a cloud workspace, collection and request")
	}
	before, err := requestObject(baseline)
	if err != nil {
		return nil, err
	}
	after, err := requestObject(draft)
	if err != nil {
		return nil, err
	}
	if before["id"] != target.Request || after["id"] != target.Request {
		return nil, fmt.Errorf("cloud request identity cannot be changed")
	}
	keys := map[string]bool{}
	for key := range before {
		keys[key] = true
	}
	for key := range after {
		keys[key] = true
	}
	var changed []string
	for key := range keys {
		beforeValue, beforeExists := before[key]
		afterValue, afterExists := after[key]
		if beforeExists != afterExists || !reflect.DeepEqual(beforeValue, afterValue) {
			changed = append(changed, key)
		}
	}
	sort.Strings(changed)
	args := []string{"collection", "request", "update", target.Request, "--collection", target.Collection, "--workspace", target.Workspace, "--type", "http", "--json"}
	flags := map[string]string{"name": "--rename", "description": "--description", "url": "--url", "method": "--method"}
	var stdin []byte
	for _, key := range changed {
		if key == "body" {
			body, ok := after[key].(map[string]any)
			if !ok || len(body) != 2 || body["type"] != "json" {
				return nil, fmt.Errorf("cloud body editing currently supports plain JSON bodies only; no changes were sent")
			}
			content, ok := body["content"].(string)
			if !ok {
				return nil, fmt.Errorf("cloud JSON body content must be a string; no changes were sent")
			}
			stdin = []byte(content)
			args = append(args, "--body", "json:-")
			continue
		}
		flag, ok := flags[key]
		if !ok {
			return nil, fmt.Errorf("cloud editing of %s is not supported yet; no changes were sent", key)
		}
		value, ok := after[key].(string)
		if !ok {
			return nil, fmt.Errorf("cloud %s must be a string", key)
		}
		args = append(args, flag, value)
	}
	current, err := c.Request(ctx, target)
	if err != nil {
		return nil, err
	}
	latest, err := requestObject(current)
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(before, latest) {
		return nil, fmt.Errorf("request changed in Postman Cloud; reload before saving")
	}
	if len(changed) == 0 {
		return current, nil
	}
	var result json.RawMessage
	if err := c.run(ctx, &result, stdin, args...); err != nil {
		return nil, fmt.Errorf("cloud update did not complete reliably; reload before retrying: %w", err)
	}
	saved, err := c.Request(ctx, target)
	if err != nil {
		return nil, fmt.Errorf("cloud update completed but read-back failed; reload before further edits")
	}
	confirmed, err := requestObject(saved)
	if err != nil {
		return nil, err
	}
	for _, key := range changed {
		if !reflect.DeepEqual(after[key], confirmed[key]) {
			return nil, fmt.Errorf("cloud update returned different values; reload to inspect the saved request")
		}
	}
	return saved, nil
}
