package cloud

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/darrenburns/posting/v3/internal/client"
)

type cachedCollection struct {
	ID   string `json:"id"`
	Hash string `json:"hash"`
}
type cacheManifest struct {
	Version     int                `json:"version"`
	Workspace   string             `json:"workspace"`
	Collections []cachedCollection `json:"collections"`
}

func cacheWorkspaceDir(root, id string) string {
	return filepath.Join(root, fmt.Sprintf("%x", sha256.Sum256([]byte(id))))
}
func writeCacheFile(file string, data []byte) error {
	if old, err := os.ReadFile(file); err == nil && bytes.Equal(old, data) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(file), ".cache-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), file)
}

// SaveWorkspaceCache commits a manifest only after all content-addressed
// collection documents exist. A failed refresh cannot replace the prior view.
func SaveWorkspaceCache(root, id string, w *Workspace) error {
	dir := cacheWorkspaceDir(root, id)
	manifest := cacheManifest{Version: 1, Workspace: id, Collections: []cachedCollection{}}
	for _, store := range w.stores {
		var doc any
		if err := json.Unmarshal(store.snapshot.Document, &doc); err != nil {
			return err
		}
		data, err := json.Marshal(doc)
		if err != nil {
			return err
		}
		hash := fmt.Sprintf("%x", sha256.Sum256(data))
		if err := writeCacheFile(filepath.Join(dir, hash+".json"), data); err != nil {
			return err
		}
		manifest.Collections = append(manifest.Collections, cachedCollection{ID: store.snapshot.CollectionID, Hash: hash})
	}
	sort.Slice(manifest.Collections, func(i, j int) bool { return manifest.Collections[i].ID < manifest.Collections[j].ID })
	data, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	return writeCacheFile(filepath.Join(dir, "manifest.json"), data)
}
func (c CLI) OpenCachedWorkspace(root, id, name string) (*Workspace, error) {
	dir := cacheWorkspaceDir(root, id)
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return nil, err
	}
	var manifest cacheManifest
	if json.Unmarshal(data, &manifest) != nil || manifest.Version != 1 || manifest.Workspace != id {
		return nil, fmt.Errorf("invalid workspace cache")
	}
	w := &Workspace{name: name, stores: map[string]*Store{}, senders: map[string]*client.PostmanCollection{}}
	fail := func(err error) (*Workspace, error) { w.Close(); return nil, err }
	for _, entry := range manifest.Collections {
		if len(entry.Hash) != 64 {
			return fail(fmt.Errorf("invalid cache hash"))
		}
		for _, ch := range entry.Hash {
			if !(ch >= '0' && ch <= '9' || ch >= 'a' && ch <= 'f') {
				return fail(fmt.Errorf("invalid cache hash"))
			}
		}
		doc, err := os.ReadFile(filepath.Join(dir, entry.Hash+".json"))
		if err != nil {
			return fail(err)
		}
		if fmt.Sprintf("%x", sha256.Sum256(doc)) != entry.Hash {
			return fail(fmt.Errorf("cache integrity check failed"))
		}
		snapshot, err := snapshotFromDocument(entry.ID, doc)
		if err != nil {
			return fail(err)
		}
		store, err := c.storeFromSnapshot(context.Background(), id, snapshot, false)
		if err != nil {
			return fail(err)
		}
		prefix := fmt.Sprintf("collection-%x", sha256.Sum256([]byte(entry.ID)))
		w.stores[prefix] = store
		w.senders[prefix] = &client.PostmanCollection{Binary: c.Binary, Root: store.Root(), Render: store.Render}
	}
	return w, nil
}

func HasWorkspaceCache(root, id string) bool {
	info, err := os.Stat(filepath.Join(cacheWorkspaceDir(root, id), "manifest.json"))
	return err == nil && !info.IsDir()
}
