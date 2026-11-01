package collection

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"slices"

	"github.com/darrenburns/posting/v3/internal/model"
	"gopkg.in/yaml.v3"
)

// Variables supplies display-only inherited values; execution still uses the
// actual Postman definitions, never a flattened environment override.
// Native v3 folders have auth and scripts but no variable scope.
func (s *NativeStore) Variables(file string) []model.Variable {
	s.mu.Lock()
	defer s.mu.Unlock()
	folder := path.Dir(file)
	for {
		if values, ok := s.scopes[folder]; ok {
			return slices.Clone(values)
		}
		if folder == "." || folder == "/" {
			return nil
		}
		folder = path.Dir(folder)
	}
}

func nativeVariableScope(root *os.Root, folder string, scopes map[string][]model.Variable) ([]model.Variable, error) {
	if values, ok := scopes[folder]; ok {
		return values, nil
	}
	var inherited []model.Variable
	if folder != "." {
		inherited, err := nativeVariableScope(root, path.Dir(folder), scopes)
		if err == nil {
			scopes[folder] = inherited
		}
		return inherited, err
	}
	file := path.Join(folder, ".resources/definition.yaml")
	data, err := root.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		scopes[folder] = inherited
		return inherited, nil
	}
	if err != nil {
		return nil, err
	}
	var doc yaml.Node
	invalid := func() error { return fmt.Errorf("invalid native collection variables in %s", file) }
	if yaml.Unmarshal(data, &doc) != nil || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, invalid()
	}
	node := nativeNode(doc.Content[0], "variables")
	if node == nil {
		scopes[folder] = inherited
		return inherited, nil
	}
	if node.Kind != yaml.SequenceNode {
		return nil, invalid()
	}
	var own []model.Variable
	for _, row := range node.Content {
		key, value, disabled := nativeNode(row, "key"), nativeNode(row, "value"), nativeNode(row, "disabled")
		if key == nil || key.Tag != "!!str" || value == nil || value.Tag != "!!str" || disabled != nil && disabled.Tag != "!!bool" {
			return nil, invalid()
		}
		if disabled != nil && disabled.Value == "true" {
			continue
		}
		own = append(own, model.Variable{Name: key.Value, Value: value.Value, Source: file})
	}
	scopes[folder] = model.Merge(inherited, own)
	return scopes[folder], nil
}
