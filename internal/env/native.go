package env

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/darrenburns/posting/v3/internal/model"
	"gopkg.in/yaml.v3"
)

// NativeFiles reads Postman's shared environment documents without modifying them.
type NativeFiles struct{ Roots []string }

func (s NativeFiles) Candidates() [][]string {
	seen := map[string]bool{}
	for _, root := range s.Roots {
		_ = filepath.WalkDir(root, func(file string, entry fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if entry.IsDir() {
				if file != root && strings.HasPrefix(entry.Name(), ".") {
					return fs.SkipDir
				}
				return nil
			}
			if entry.Type().IsRegular() && strings.HasSuffix(file, ".environment.yaml") {
				absolute, err := filepath.Abs(file)
				if err == nil {
					seen[absolute] = true
				}
			}
			return nil
		})
	}
	paths := make([]string, 0, len(seen))
	for file := range seen {
		paths = append(paths, file)
	}
	sort.Strings(paths)
	groups := make([][]string, len(paths))
	for i, file := range paths {
		groups[i] = []string{file}
	}
	return groups
}

func (s NativeFiles) Load(files []string) (model.Environment, error) {
	result := model.Environment{Files: append([]string(nil), files...)}
	names := []string{}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return model.Environment{}, err
		}
		var doc yaml.Node
		invalid := func() (model.Environment, error) {
			return model.Environment{}, fmt.Errorf("invalid Postman environment %s: expected name and string-valued entries", file)
		}
		if yaml.Unmarshal(data, &doc) != nil || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
			return invalid()
		}
		root := doc.Content[0]
		name, values := envNode(root, "name"), envNode(root, "values")
		if name == nil || name.Tag != "!!str" || name.Value == "" || values == nil || values.Kind != yaml.SequenceNode {
			return invalid()
		}
		names = append(names, name.Value)
		layer := []model.Variable{}
		for _, entry := range values.Content {
			key, value, enabled := envNode(entry, "key"), envNode(entry, "value"), envNode(entry, "enabled")
			if key == nil || key.Tag != "!!str" || value == nil || value.Tag != "!!str" || enabled != nil && enabled.Tag != "!!bool" {
				return invalid()
			}
			if enabled != nil && enabled.Value == "false" {
				continue
			}
			layer = append(layer, model.Variable{Name: key.Value, Value: value.Value, Source: file})
		}
		result.Variables = model.Merge(result.Variables, layer)
	}
	result.Name = strings.Join(names, " + ")
	return result, nil
}

func envNode(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}
