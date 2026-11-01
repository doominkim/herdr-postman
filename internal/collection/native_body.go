package collection

import (
	"fmt"
	"strings"

	"github.com/darrenburns/posting/v3/internal/model"
	"gopkg.in/yaml.v3"
)

func nativeSetBody(root *yaml.Node, value model.Body) error {
	if value.Type != model.BodyFile && value.Type != model.BodyMultipart && value.Type != model.BodyRaw && value.Type != model.BodyForm && value.Type != model.BodyNone && value.Type != "" {
		return fmt.Errorf("native body type %q is not supported yet", value.Type)
	}
	body := nativeNode(root, "body")
	if body == nil || body.Kind != yaml.MappingNode {
		nativeRemove(root, "body")
		body = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "body"}, body)
	}
	if value.Type == model.BodyFile {
		nativeSet(body, "type", "file")
		nativeRemove(body, "content")
		content := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		nativeSet(content, "src", value.File)
		body.Content = append(body.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "content"}, content)
		if value.ContentType != "" {
			rows := nativeRows(nativeNode(root, "headers"))
			found := false
			for i := range rows {
				if rows[i].Enabled && strings.EqualFold(rows[i].Name, "Content-Type") {
					rows[i].Value = value.ContentType
					found = true
				}
			}
			if !found {
				rows = append(rows, model.KeyValue{Name: "Content-Type", Value: value.ContentType, Enabled: true})
			}
			nativeSetRows(root, "headers", rows)
		}
		return nil
	}
	if value.Type == model.BodyMultipart {
		for _, part := range value.Multipart {
			if part.Filename != "" {
				return fmt.Errorf("native multipart filename overrides are not supported yet")
			}
		}
		nativeSet(body, "type", "formdata")
		if content := nativeNode(body, "content"); content != nil && content.Kind != yaml.SequenceNode {
			nativeRemove(body, "content")
		}
		if content := nativeNode(body, "content"); content != nil {
			content.Content = expandedNativeFileRows(content.Content)
		}
		rows := make([]model.KeyValue, len(value.Multipart))
		for i, part := range value.Multipart {
			rows[i] = model.KeyValue{Name: part.Name, Value: part.Value, Enabled: part.Enabled}
		}
		nativeSetRows(body, "content", rows)
		for i, row := range nativeNode(body, "content").Content {
			part := value.Multipart[i]
			if part.Type == model.PartFile {
				nativeSet(row, "type", "file")
				nativeSet(row, "src", part.File)
				nativeRemove(row, "value")
			} else {
				nativeSet(row, "type", "text")
				nativeRemove(row, "src")
			}
			if part.ContentType != "" {
				nativeSet(row, "contentType", part.ContentType)
			} else {
				nativeRemove(row, "contentType")
			}
		}
		return nil
	}
	if value.Type == model.BodyForm {
		nativeSet(body, "type", "urlencoded")
		if content := nativeNode(body, "content"); content != nil && content.Kind != yaml.SequenceNode {
			nativeRemove(body, "content")
		}
		nativeSetRows(body, "content", value.Form)
		return nil
	}
	kind := "text"
	for candidate, mime := range nativeBodyMIME {
		if value.ContentType == mime {
			kind = candidate
			break
		}
	}
	if value.Type == model.BodyNone || value.Type == "" {
		kind = "none"
	}
	nativeSet(body, "type", kind)
	nativeSet(body, "content", value.Raw)
	return nil
}

func nativeParts(node *yaml.Node) []model.Part {
	if node == nil || node.Kind != yaml.SequenceNode {
		return nil
	}
	parts := make([]model.Part, 0, len(node.Content))
	for _, row := range expandedNativeFileRows(node.Content) {
		kind := model.PartType(nativeValue(row, "type"))
		if kind == "" {
			kind = model.PartText
		}
		parts = append(parts, model.Part{Type: kind, Name: nativeValue(row, "key"), Value: nativeValue(row, "value"), File: nativeValue(row, "src"), ContentType: nativeValue(row, "contentType"), Enabled: nativeValue(row, "disabled") != "true"})
	}
	return parts
}

// Expand GUI multi-file fields to ordered editable rows. Cloning each row
// retains its description and other metadata when a neighboring field changes.
func expandedNativeFileRows(rows []*yaml.Node) []*yaml.Node {
	var expanded []*yaml.Node
	for _, row := range rows {
		src := nativeNode(row, "src")
		if nativeValue(row, "type") != "file" || src == nil || src.Kind != yaml.SequenceNode {
			expanded = append(expanded, row)
			continue
		}
		for _, file := range src.Content {
			copy := cloneNativeNode(row)
			nativeSet(copy, "src", file.Value)
			expanded = append(expanded, copy)
		}
	}
	return expanded
}

func cloneNativeNode(node *yaml.Node) *yaml.Node {
	copy := *node
	copy.Content = make([]*yaml.Node, len(node.Content))
	for i, child := range node.Content {
		copy.Content[i] = cloneNativeNode(child)
	}
	return &copy
}
