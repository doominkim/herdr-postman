package cloud

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/darrenburns/posting/v3/internal/client"
	"github.com/darrenburns/posting/v3/internal/collection"
	"github.com/darrenburns/posting/v3/internal/model"
)

// Workspace routes each opaque file prefix to its authoritative Cloud collection.
type Workspace struct {
	name    string
	stores  map[string]*Store
	senders map[string]*client.PostmanCollection
}

func (c CLI) OpenWorkspace(ctx context.Context, id, name string) (*Workspace, error) {
	resources, err := c.Collections(ctx, id)
	if err != nil {
		return nil, err
	}
	ws := &Workspace{name: name, stores: map[string]*Store{}, senders: map[string]*client.PostmanCollection{}}
	// Bound CLI processes and network requests while fetching independent collections.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stores := make([]*Store, len(resources))
	failures := make([]error, len(resources))
	slots := make(chan struct{}, 3)
	var pending sync.WaitGroup
	for i, resource := range resources {
		pending.Add(1)
		go func(i int, resource Resource) {
			defer pending.Done()
			select {
			case slots <- struct{}{}:
			case <-ctx.Done():
				failures[i] = ctx.Err()
				return
			}
			defer func() { <-slots }()
			stores[i], failures[i] = c.openStore(ctx, id, resource.ID, false)
			if failures[i] != nil {
				cancel()
			}
		}(i, resource)
	}
	pending.Wait()
	for _, err := range failures {
		if err != nil {
			for _, store := range stores {
				if store != nil {
					store.Close()
				}
			}
			return nil, err
		}
	}
	for i, resource := range resources {
		store := stores[i]
		prefix := fmt.Sprintf("collection-%x", sha256.Sum256([]byte(resource.ID)))
		ws.stores[prefix] = store
		ws.senders[prefix] = &client.PostmanCollection{Binary: c.Binary, Root: store.Root(), Render: store.Render}
	}
	return ws, nil
}
func (w *Workspace) Close() error {
	var errs []error
	for _, store := range w.stores {
		errs = append(errs, store.Close())
	}
	return errors.Join(errs...)
}
func (w *Workspace) RequestSuffix() string { return collection.NativeSuffix }
func (w *Workspace) Load() (*model.Collection, []collection.LoadError) {
	root := &model.Collection{Name: w.name}
	var problems []collection.LoadError
	for prefix, store := range w.stores {
		coll, errs := store.Load()
		problems = append(problems, errs...)
		var qualify func(*model.Collection)
		qualify = func(c *model.Collection) {
			if c.Path == "" {
				c.Path = prefix
			} else {
				c.Path = prefix + "/" + c.Path
			}
			for i := range c.Requests {
				c.Requests[i].File = prefix + "/" + c.Requests[i].File
			}
			for _, child := range c.Children {
				qualify(child)
			}
		}
		qualify(coll)
		root.Children = append(root.Children, coll)
	}
	root.Sort()
	return root, problems
}
func (w *Workspace) route(req model.Request) (*Store, string, model.Request, error) {
	prefix, file, ok := strings.Cut(req.File, "/")
	store := w.stores[prefix]
	if !ok || store == nil {
		return nil, "", req, fmt.Errorf("select a saved request from a Cloud collection")
	}
	req.File = file
	return store, prefix, req, nil
}
func (w *Workspace) Save(req model.Request) error {
	store, _, local, err := w.route(req)
	if err != nil {
		return err
	}
	return store.Save(local)
}
func (w *Workspace) Render(req model.Request) ([]byte, error) {
	store, _, local, err := w.route(req)
	if err != nil {
		return nil, err
	}
	return store.Render(local)
}
func (w *Workspace) Create(model.Request) error {
	return fmt.Errorf("create Cloud requests in Postman GUI for now")
}
func (w *Workspace) Delete(string) error {
	return fmt.Errorf("delete Cloud requests in Postman GUI for now")
}
func (w *Workspace) Send(ctx context.Context, call client.Call) (*model.Response, error) {
	_, prefix, local, err := w.route(call.Request)
	if err != nil {
		return nil, err
	}
	call.Request = local
	return w.senders[prefix].Send(ctx, call)
}
func (w *Workspace) Variables(file string) []model.Variable {
	store, _, local, err := w.route(model.Request{File: file})
	if err != nil {
		return nil
	}
	return store.snapshot.Store.Variables(local.File)
}
