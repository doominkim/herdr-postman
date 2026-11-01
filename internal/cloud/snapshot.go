package cloud

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/darrenburns/posting/v3/internal/collection"
	"gopkg.in/yaml.v3"
)

// Snapshot is a private execution copy, never the authoritative cloud store.
// Closing it removes only the temporary directory allocated by Open.
type Snapshot struct {
	Store        *collection.NativeStore
	CollectionID string
	Document     json.RawMessage
	closeOnce    sync.Once
	closeErr     error
}

func (s *Snapshot) Close() error {
	s.closeOnce.Do(func() { s.closeErr = os.RemoveAll(s.Store.Root) })
	return s.closeErr
}

func (c CLI) Open(ctx context.Context, id string) (*Snapshot, error) {
	if id == "" {
		return nil, fmt.Errorf("select a cloud collection first")
	}
	var document json.RawMessage
	if err := c.read(ctx, &document, "collection", "get", id, "--json"); err != nil {
		return nil, err
	}
	return snapshotFromDocument(id, document)
}

func snapshotFromDocument(id string, document json.RawMessage) (*Snapshot, error) {
	var root map[string]any
	if json.Unmarshal(document, &root) != nil || root["$kind"] != "collection" {
		return nil, fmt.Errorf("Postman Cloud returned an invalid collection")
	}
	dir, err := os.MkdirTemp("", "herdr-postman-cloud-")
	if err != nil {
		return nil, err
	}
	snapshot := &Snapshot{Store: collection.NewNativeStore(dir), CollectionID: id, Document: document}
	if err := materializeCollection(dir, root); err != nil {
		snapshot.Close()
		return nil, err
	}
	return snapshot, nil
}

func writeNative(file string, document map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		return err
	}
	data, err := yaml.Marshal(document)
	if err != nil {
		return fmt.Errorf("could not encode cloud document")
	}
	return os.WriteFile(file, data, 0600)
}

// Cloud identifiers are opaque. Hashing prevents filenames supplied by remote
// metadata from escaping the private snapshot, even when names contain slashes.
func itemFile(item map[string]any, index int) string {
	identity, _ := item["id"].(string)
	if identity == "" {
		identity = fmt.Sprintf("anonymous-%d", index)
	}
	return fmt.Sprintf("item-%x", sha256.Sum256([]byte(identity)))
}

func materializeCollection(dir string, folder map[string]any) error {
	items, _ := folder["items"].([]any)
	delete(folder, "items")
	folder["$kind"] = "collection"
	if err := writeNative(filepath.Join(dir, ".resources", "definition.yaml"), folder); err != nil {
		return err
	}
	for i, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("invalid cloud collection item")
		}
		name := itemFile(item, i)
		if item["$kind"] == "folder" || item["$kind"] == "collection" {
			if err := materializeCollection(filepath.Join(dir, name), item); err != nil {
				return err
			}
			continue
		}
		if examples, ok := item["examples"].([]any); ok {
			relative := filepath.Join(".resources", name+".resources", "examples")
			for j, raw := range examples {
				example, ok := raw.(map[string]any)
				if !ok {
					return fmt.Errorf("invalid cloud request example")
				}
				if err := writeNative(filepath.Join(dir, relative, fmt.Sprintf("example-%d.example.yaml", j)), example); err != nil {
					return err
				}
			}
			if len(examples) > 0 {
				item["examples"] = "./" + filepath.ToSlash(relative) + "/"
			} else {
				delete(item, "examples")
			}
		}
		if err := writeNative(filepath.Join(dir, name+collection.NativeSuffix), item); err != nil {
			return err
		}
	}
	return nil
}
