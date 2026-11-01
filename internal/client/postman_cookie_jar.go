package client

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"sync"
)

// postmanCookieJar retains Postman's own serialized cookie records in memory.
// Runs merge only their changes against their starting snapshot, so independent
// concurrent requests do not replace each other's newly received cookies.
type postmanCookieJar struct {
	mu       sync.Mutex
	metadata map[string]any
	records  map[string]map[string]any
}

func parsePostmanJar(data []byte) (map[string]any, map[string]map[string]any, error) {
	doc := map[string]any{}
	records := map[string]map[string]any{}
	if len(data) == 0 {
		return doc, records, nil
	}
	if json.Unmarshal(data, &doc) != nil {
		return nil, nil, fmt.Errorf("invalid Postman cookie jar")
	}
	values, ok := doc["cookies"].([]any)
	if !ok {
		return nil, nil, fmt.Errorf("Postman cookie jar has no cookie list")
	}
	for _, value := range values {
		cookie, ok := value.(map[string]any)
		if !ok {
			return nil, nil, fmt.Errorf("invalid Postman cookie record")
		}
		key, keyOK := cookie["key"].(string)
		domain, domainOK := cookie["domain"].(string)
		path, pathOK := cookie["path"].(string)
		if !keyOK || !domainOK || !pathOK {
			return nil, nil, fmt.Errorf("invalid Postman cookie identity")
		}
		records[domain+"\x00"+path+"\x00"+key] = cookie
	}
	delete(doc, "cookies")
	return doc, records, nil
}

func (j *postmanCookieJar) snapshot() []byte {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.metadata == nil {
		return nil
	}
	doc := map[string]any{}
	for key, value := range j.metadata {
		doc[key] = value
	}
	keys := make([]string, 0, len(j.records))
	for key := range j.records {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	values := make([]map[string]any, 0, len(keys))
	for _, key := range keys {
		values = append(values, j.records[key])
	}
	doc["cookies"] = values
	data, _ := json.Marshal(doc)
	return data
}

func (j *postmanCookieJar) merge(base, after []byte) error {
	_, before, err := parsePostmanJar(base)
	if err != nil {
		return err
	}
	metadata, next, err := parsePostmanJar(after)
	if err != nil {
		return err
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.records == nil {
		j.records = map[string]map[string]any{}
	}
	for key := range before {
		if _, exists := next[key]; !exists {
			delete(j.records, key)
		}
	}
	for key, value := range next {
		if !samePostmanCookie(before[key], value) {
			j.records[key] = value
		}
	}
	j.metadata = metadata
	return nil
}

// Reading a cookie updates lastAccessed, but must not overwrite a concurrent
// response that changed its value or attributes.
func samePostmanCookie(a, b map[string]any) bool {
	withoutAccess := func(record map[string]any) map[string]any {
		if record == nil {
			return nil
		}
		copy := make(map[string]any, len(record))
		for key, value := range record {
			if key != "lastAccessed" {
				copy[key] = value
			}
		}
		return copy
	}
	return reflect.DeepEqual(withoutAccess(a), withoutAccess(b))
}
