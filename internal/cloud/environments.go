package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/darrenburns/posting/v3/internal/model"
)

// Environments caches only identities on disk. Selected values live in memory.
type Environments struct {
	cli       CLI
	resources []Resource
	mu        sync.Mutex
	loaded    map[string]model.Environment
}

func (c CLI) OpenEnvironments(ctx context.Context, root, workspace string, cached bool) (*Environments, error) {
	if workspace == "" {
		return nil, fmt.Errorf("select a cloud workspace first")
	}
	file := filepath.Join(cacheWorkspaceDir(root, workspace), "environments.json")
	var resources []Resource
	if cached {
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		if err = json.Unmarshal(data, &resources); err != nil {
			return nil, fmt.Errorf("invalid environment metadata cache")
		}
	} else {
		if err := c.read(ctx, &resources, "environment", "list", "--workspace", workspace, "--json"); err != nil {
			return nil, err
		}
	}
	if err := validateResources(resources); err != nil {
		return nil, err
	}
	if !cached {
		data, _ := json.Marshal(resources)
		if err := writeCacheFile(file, data); err != nil {
			return nil, err
		}
	}
	return &Environments{cli: c, resources: resources, loaded: map[string]model.Environment{}}, nil
}

func (s *Environments) Candidates() [][]string {
	out := make([][]string, len(s.resources))
	for i, r := range s.resources {
		out[i] = []string{r.ID}
	}
	return out
}
func (s *Environments) Describe(ids []string) (string, error) {
	if len(ids) == 1 {
		for _, r := range s.resources {
			if r.ID == ids[0] {
				return strings.Map(func(r rune) rune {
					if r < 32 || (r >= 127 && r <= 159) {
						return ' '
					}
					return r
				}, r.Name), nil
			}
		}
	}
	return "", fmt.Errorf("environment is not available in this workspace")
}
func (s *Environments) Load(ids []string) (model.Environment, error) {
	return s.LoadContext(context.Background(), ids)
}
func (s *Environments) LoadContext(ctx context.Context, ids []string) (model.Environment, error) {
	name, err := s.Describe(ids)
	if err != nil {
		return model.Environment{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if env, ok := s.loaded[ids[0]]; ok {
		return cloneEnvironment(env), nil
	}
	var doc struct {
		ID     string `json:"id"`
		Owner  string `json:"owner"`
		Values []struct {
			Key     string `json:"key"`
			Value   any    `json:"value"`
			Enabled *bool  `json:"enabled"`
		} `json:"values"`
	}
	if err := s.cli.read(ctx, &doc, "environment", "get", ids[0], "--json"); err != nil {
		return model.Environment{}, err
	}
	if (doc.ID != ids[0] && (doc.Owner == "" || doc.Owner+"-"+doc.ID != ids[0])) || doc.Values == nil {
		return model.Environment{}, fmt.Errorf("Postman Cloud returned an invalid environment")
	}
	env := model.Environment{Name: name, Files: slices.Clone(ids)}
	for _, v := range doc.Values {
		if v.Enabled != nil && !*v.Enabled {
			continue
		}
		value, ok := v.Value.(string)
		if !ok {
			return model.Environment{}, fmt.Errorf("environment contains a non-text value")
		}
		if v.Key == "" {
			continue
		}
		env.Variables = append(env.Variables, model.Variable{Name: v.Key, Value: value, Source: name})
	}
	s.loaded[ids[0]] = env
	return cloneEnvironment(env), nil
}
func cloneEnvironment(env model.Environment) model.Environment {
	env.Files = slices.Clone(env.Files)
	env.Variables = slices.Clone(env.Variables)
	return env
}
