package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sync"

	"github.com/darrenburns/posting/v3/internal/collection"
	"github.com/darrenburns/posting/v3/internal/model"
	"gopkg.in/yaml.v3"
)

// Store commits edits to Cloud before updating its private execution copy.
// It deliberately does not embed NativeStore: unsupported mutations must never
// accidentally succeed against the cache alone.
type Store struct {
	snapshot  *Snapshot
	cli       CLI
	workspace string
	baseline  map[string]json.RawMessage
	canonical map[string]bool
	mu        sync.Mutex
}

func (c CLI) OpenStore(ctx context.Context, workspace, id string) (*Store, error) {
	return c.openStore(ctx, workspace, id, true)
}

func (c CLI) openStore(ctx context.Context, workspace, id string, hydrateRequests bool) (*Store, error) {
	if workspace == "" {
		return nil, fmt.Errorf("select a cloud workspace first")
	}
	snapshot, err := c.Open(ctx, id)
	if err != nil {
		return nil, err
	}
	return c.storeFromSnapshot(ctx, workspace, snapshot, hydrateRequests)
}

func (c CLI) storeFromSnapshot(ctx context.Context, workspace string, snapshot *Snapshot, hydrateRequests bool) (*Store, error) {
	var err error
	store := &Store{snapshot: snapshot, cli: c, workspace: workspace, baseline: map[string]json.RawMessage{}, canonical: map[string]bool{}}
	var document map[string]any
	if err = json.Unmarshal(snapshot.Document, &document); err != nil {
		snapshot.Close()
		return nil, err
	}
	// Fetch canonical request documents before presenting editable data. Request
	// endpoints include metadata absent from collection listings.
	var hydrate func(map[string]any) error
	hydrate = func(folder map[string]any) error {
		items, _ := folder["items"].([]any)
		for i, raw := range items {
			item, ok := raw.(map[string]any)
			if !ok {
				return fmt.Errorf("invalid cloud item")
			}
			if item["$kind"] == "folder" || item["$kind"] == "collection" {
				if err := hydrate(item); err != nil {
					return err
				}
				continue
			}
			if item["$kind"] != "http-request" {
				continue
			}
			requestID, _ := item["id"].(string)
			if !hydrateRequests {
				data, err := json.Marshal(item)
				if err != nil {
					return err
				}
				store.baseline[requestID] = data
				continue
			}
			target := Target{Workspace: workspace, Collection: snapshot.CollectionID, Request: requestID}
			data, err := c.Request(ctx, target)
			if err != nil {
				return err
			}
			store.baseline[requestID] = data
			store.canonical[requestID] = true
			var canonical map[string]any
			if err := json.Unmarshal(data, &canonical); err != nil {
				return err
			}
			delete(canonical, "parent")
			items[i] = canonical
		}
		return nil
	}
	if err = hydrate(document); err == nil {
		err = materializeCollection(snapshot.Store.Root, document)
	}
	if err != nil {
		snapshot.Close()
		return nil, err
	}
	snapshot.Store.Load()
	return store, nil
}

func (s *Store) Close() error          { return s.snapshot.Close() }
func (s *Store) Root() string          { return s.snapshot.Store.Root }
func (s *Store) RequestSuffix() string { return collection.NativeSuffix }
func (s *Store) Load() (*model.Collection, []collection.LoadError) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshot.Store.Load()
}
func (s *Store) Render(req model.Request) ([]byte, error) { return s.snapshot.Store.Render(req) }
func (s *Store) Create(model.Request) error {
	return fmt.Errorf("create Cloud requests in Postman GUI for now")
}
func (s *Store) Delete(string) error {
	return fmt.Errorf("delete Cloud requests in Postman GUI for now")
}
func (s *Store) Save(req model.Request) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rendered, err := s.snapshot.Store.Render(req)
	if err != nil {
		return err
	}
	var edited map[string]any
	if err := yaml.Unmarshal(rendered, &edited); err != nil {
		return err
	}
	id, _ := edited["id"].(string)
	baseline, ok := s.baseline[id]
	if !ok {
		return fmt.Errorf("request has no Cloud identity; reopen the collection")
	}
	if !s.canonical[id] {
		current, err := s.cli.Request(context.Background(), Target{Workspace: s.workspace, Collection: s.snapshot.CollectionID, Request: id})
		if err != nil {
			return err
		}
		original, _ := requestObject(baseline)
		latest, _ := requestObject(current)
		// The request endpoint adds parent and examples metadata that may be
		// omitted by collection get. Preserve it, but never accept changes to
		// the document fields the user actually opened.
		delete(latest, "parent")
		delete(original, "parent")
		if _, present := original["examples"]; !present {
			delete(latest, "examples")
		}
		if !reflect.DeepEqual(original, latest) {
			return fmt.Errorf("request changed in Postman Cloud; reopen the workspace before saving")
		}
		baseline = current
		s.baseline[id] = current
		s.canonical[id] = true
	}
	before, err := requestObject(baseline)
	if err != nil {
		return err
	}
	// Compare native documents to find actual edits. Native examples are paths;
	// the unchanged cloud examples and parent metadata stay in the baseline.
	root, err := os.OpenRoot(s.Root())
	if err != nil {
		return err
	}
	defer root.Close()
	original, err := root.ReadFile(req.File)
	if err != nil {
		return err
	}
	var native map[string]any
	if err := yaml.Unmarshal(original, &native); err != nil {
		return err
	}
	for key, value := range edited {
		if !reflect.DeepEqual(native[key], value) {
			before[key] = value
		}
	}
	for key := range native {
		if _, ok := edited[key]; !ok {
			delete(before, key)
		}
	}
	draft, err := json.Marshal(before)
	if err != nil {
		return err
	}
	saved, err := s.cli.SaveRequest(context.Background(), Target{Workspace: s.workspace, Collection: s.snapshot.CollectionID, Request: id}, baseline, draft)
	if err != nil {
		return err
	}
	s.baseline[id] = saved
	if err := s.snapshot.Store.Save(req); err != nil {
		return fmt.Errorf("Cloud saved, but local copy could not update; reopen collection: %w", err)
	}
	return nil
}
