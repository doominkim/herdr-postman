package collection

import (
	"fmt"
	"github.com/darrenburns/posting/v3/internal/model"
	"gopkg.in/yaml.v3"
)

func nativeAuth(node *yaml.Node) model.Auth {
	if node == nil {
		return model.Auth{Type: model.AuthInherit}
	}
	auth := model.Auth{Type: model.AuthManaged}
	switch nativeValue(node, "type") {
	case "noauth":
		auth.Type = model.AuthNone
	case "basic":
		auth.Type = model.AuthBasic
	case "digest":
		auth.Type = model.AuthDigest
	case "bearer":
		auth.Type = model.AuthBearer
	default:
		return auth
	}
	if creds := nativeNode(node, "credentials"); creds != nil && creds.Kind == yaml.SequenceNode {
		for _, row := range creds.Content {
			switch nativeValue(row, "key") {
			case "username":
				auth.Username = nativeValue(row, "value")
			case "password":
				auth.Password = nativeValue(row, "value")
			case "token":
				auth.Token = nativeValue(row, "value")
			}
		}
	}
	return auth
}

func nativeRemove(node *yaml.Node, key string) {
	for i := 0; i < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			node.Content = append(node.Content[:i], node.Content[i+2:]...)
			return
		}
	}
}

func nativeSetAuth(root *yaml.Node, auth model.Auth) error {
	var scheme string
	switch auth.Type {
	case model.AuthInherit:
		nativeRemove(root, "auth")
		return nil
	case "", model.AuthNone:
		scheme = "noauth"
	case model.AuthBasic:
		scheme = "basic"
	case model.AuthDigest:
		scheme = "digest"
	case model.AuthBearer:
		scheme = "bearer"
	default:
		return fmt.Errorf("editing native authentication %q is not supported yet", auth.Type)
	}
	node := nativeNode(root, "auth")
	if node == nil || node.Kind != yaml.MappingNode || nativeValue(node, "type") != scheme {
		nativeRemove(root, "auth")
		node = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "auth"}, node)
	}
	nativeSet(node, "type", scheme)
	if scheme == "noauth" {
		nativeRemove(node, "credentials")
		return nil
	}
	creds := nativeNode(node, "credentials")
	if creds == nil || creds.Kind != yaml.SequenceNode {
		nativeRemove(node, "credentials")
		creds = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "credentials"}, creds)
	}
	values := []struct{ key, value string }{{"username", auth.Username}, {"password", auth.Password}}
	if scheme == "bearer" {
		values = []struct{ key, value string }{{"token", auth.Token}}
	}
	for _, pair := range values {
		var found *yaml.Node
		for _, row := range creds.Content {
			if nativeValue(row, "key") == pair.key {
				found = row
				break
			}
		}
		if found == nil {
			found = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			nativeSet(found, "key", pair.key)
			creds.Content = append(creds.Content, found)
		}
		nativeSet(found, "value", pair.value)
	}
	return nil
}
