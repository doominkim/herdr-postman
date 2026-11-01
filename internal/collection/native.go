package collection

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"sync"

	"github.com/darrenburns/posting/v3/internal/model"
	"gopkg.in/yaml.v3"
)

const NativeSuffix = ".request.yaml"

type nativeSnapshot struct {
	data    []byte
	request model.Request
}

// NativeStore edits Postman Native Git documents without rebuilding their
// unrepresented GUI fields from Posting's narrower request model.
// Additional editable fields are added without discarding GUI-managed metadata.
type NativeStore struct {
	Root   string
	mu     sync.Mutex
	loaded map[string]nativeSnapshot
	scopes map[string][]model.Variable
}

func NewNativeStore(root string) *NativeStore {
	return &NativeStore{Root: root, loaded: make(map[string]nativeSnapshot)}
}

var _ Store = (*NativeStore)(nil)

func (s *NativeStore) RequestSuffix() string { return NativeSuffix }

func validNativePath(file string) bool {
	return fs.ValidPath(file) && strings.HasSuffix(file, NativeSuffix) && !strings.Contains(file, "\\")
}

func (s *NativeStore) Create(req model.Request) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !validNativePath(req.File) {
		return fmt.Errorf("invalid native request path %q", req.File)
	}
	data, err := marshalNativeRequest(req)
	if err != nil {
		return err
	}
	return s.createLocked(req, data)
}

// Duplicate preserves the native document, including fields managed by Desktop.
// The caller supplies the copy's name and a new file beside the source.
func (s *NativeStore) Duplicate(req model.Request, destination string) (model.Request, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !validNativePath(destination) || path.Dir(destination) != path.Dir(req.File) {
		return model.Request{}, fmt.Errorf("duplicate must be a native request beside its source")
	}
	data, err := s.renderLocked(req)
	if err != nil {
		return model.Request{}, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return model.Request{}, err
	}
	if len(doc.Content) == 1 && nativeNode(doc.Content[0], "id") != nil {
		// A copy is a new item. Reusing a GUI-assigned ID makes CLI selection
		// ambiguous even when the two requests have different display names.
		nativeSet(doc.Content[0], "id", rand.Text())
		data, err = yaml.Marshal(&doc)
		if err != nil {
			return model.Request{}, err
		}
	}
	data, cleanup, err := s.duplicateExamples(data, req.File, destination)
	if err != nil {
		return model.Request{}, err
	}
	dup := req.Clone()
	dup.File = destination
	dup.NativeRevision = fmt.Sprintf("%x", sha256.Sum256(data))
	if err := s.createLocked(dup, data); err != nil {
		cleanup()
		return model.Request{}, err
	}
	return dup, nil
}

func (s *NativeStore) createLocked(req model.Request, data []byte) error {
	root, err := os.OpenRoot(s.Root)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := root.MkdirAll(path.Dir(req.File), 0755); err != nil {
		return err
	}
	tmp := path.Join(path.Dir(req.File), ".herdr-postman-"+rand.Text()+".tmp")
	f, err := root.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer root.Remove(tmp)
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = root.Link(tmp, req.File); err != nil {
		return err
	}
	s.loaded[req.File] = nativeSnapshot{data: bytes.Clone(data), request: req.Clone()}
	return nil
}

func marshalNativeRequest(req model.Request) ([]byte, error) {
	req = nativeWireRequest(req)
	if req.Payload != nil || req.Scripts != (model.Scripts{}) || len(req.Captures) > 0 {
		return nil, fmt.Errorf("this native request configuration is not supported yet; no file was created")
	}
	node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	nativeSet(node, "$kind", "http-request")
	nativeSet(node, "name", req.Name)
	nativeSet(node, "method", string(req.Method))
	nativeSet(node, "url", req.URL)
	if err := nativeSetAuth(node, req.Auth); err != nil {
		return nil, err
	}
	settings := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for key, value := range map[string]bool{"strictSSL": req.Options.VerifySSL, "followRedirects": req.Options.FollowRedirects, "disableCookies": !req.Options.AttachCookies} {
		nativeSet(settings, key, fmt.Sprint(value))
		nativeNode(settings, key).Tag = "!!bool"
	}
	node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "settings"}, settings)
	if req.Description != "" {
		nativeSet(node, "description", req.Description)
	}
	for key, rows := range map[string][]model.KeyValue{"headers": req.Headers, "queryParams": req.Query, "pathVariables": req.PathParams} {
		if len(rows) > 0 {
			nativeSetRows(node, key, rows)
		}
	}
	if req.Body.Type != "" && req.Body.Type != model.BodyNone {
		if err := nativeSetBody(node, req.Body); err != nil {
			return nil, err
		}
	}
	return yaml.Marshal(node)
}

func (s *NativeStore) Delete(file string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !validNativePath(file) {
		return fmt.Errorf("invalid native request path %q", file)
	}
	root, err := os.OpenRoot(s.Root)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := root.Remove(file); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	delete(s.loaded, file)
	return nil
}

func (s *NativeStore) Load() (*model.Collection, []LoadError) {
	s.mu.Lock()
	defer s.mu.Unlock()
	coll := &model.Collection{Name: filepath.Base(s.Root)}
	root, err := os.OpenRoot(s.Root)
	if err != nil {
		return coll, []LoadError{{File: s.Root, Err: err}}
	}
	defer root.Close()
	var problems []LoadError
	scopes := map[string][]model.Variable{}
	if _, err := nativeVariableScope(root, ".", scopes); err != nil {
		problems = append(problems, LoadError{File: ".resources/definition.yaml", Err: err})
	}
	err = fs.WalkDir(root.FS(), ".", func(file string, entry fs.DirEntry, err error) error {
		if err != nil {
			problems = append(problems, LoadError{File: file, Err: err})
			return nil
		}
		if entry.IsDir() {
			if file != "." && strings.HasPrefix(entry.Name(), ".") {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(file, NativeSuffix) {
			return nil
		}
		data, err := root.ReadFile(file)
		if err != nil {
			problems = append(problems, LoadError{File: file, Err: err})
			return nil
		}
		var doc yaml.Node
		if err = yaml.Unmarshal(data, &doc); err != nil {
			problems = append(problems, LoadError{File: file, Err: err})
			return nil
		}
		if len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
			problems = append(problems, LoadError{File: file, Err: fmt.Errorf("expected a Postman request mapping")})
			return nil
		}
		node := doc.Content[0]
		if nativeValue(node, "$kind") != "http-request" {
			problems = append(problems, LoadError{File: file, Err: fmt.Errorf("native request kind %q not yet supported", nativeValue(node, "$kind"))})
			return nil
		}
		req := model.NewRequest()
		req.NativeRevision = fmt.Sprintf("%x", sha256.Sum256(data))
		req.File = file
		req.Name = nativeValue(node, "name")
		if req.Name == "" {
			req.Name = strings.TrimSuffix(path.Base(file), NativeSuffix)
		}
		req.Auth = nativeAuth(nativeNode(node, "auth"))
		req.URL = nativeValue(node, "url")
		req.Method = model.Method(nativeValue(node, "method"))
		req.Description = nativeValue(node, "description")
		req.Headers = nativeRows(nativeNode(node, "headers"))
		req.Query = nativeRows(nativeNode(node, "queryParams"))
		req.PathParams = nativeRows(nativeNode(node, "pathVariables"))
		settings := nativeNode(node, "settings")
		if nativeNode(settings, "strictSSL") != nil {
			req.Options.VerifySSL = nativeValue(settings, "strictSSL") == "true"
		}
		if nativeNode(settings, "followRedirects") != nil {
			req.Options.FollowRedirects = nativeValue(settings, "followRedirects") == "true"
		}
		if nativeNode(settings, "disableCookies") != nil {
			req.Options.AttachCookies = nativeValue(settings, "disableCookies") != "true"
		}
		if body := nativeNode(node, "body"); body != nil {
			kind := nativeValue(body, "type")
			if kind == "file" {
				req.Body = model.Body{Type: model.BodyFile, File: nativeValue(nativeNode(body, "content"), "src")}
				for _, header := range req.Headers {
					if header.Enabled && strings.EqualFold(header.Name, "Content-Type") {
						req.Body.ContentType = header.Value
						break
					}
				}
			}
			if kind == "formdata" {
				req.Body = model.Body{Type: model.BodyMultipart, Multipart: nativeParts(nativeNode(body, "content"))}
			}
			if kind == "urlencoded" {
				req.Body = model.Body{Type: model.BodyForm, Form: nativeRows(nativeNode(body, "content"))}
			}
			if mime, ok := nativeBodyMIME[kind]; ok {
				req.Body = model.Body{Type: model.BodyRaw, ContentType: mime, Raw: nativeValue(body, "content")}
			}
		}
		if _, err := nativeVariableScope(root, path.Dir(file), scopes); err != nil {
			problems = append(problems, LoadError{File: file, Err: err})
		}
		req = nativeGraphQLEditor(req)
		folder := EnsureFolder(coll, path.Dir(file))
		folder.Requests = append(folder.Requests, req)
		s.loaded[file] = nativeSnapshot{data: bytes.Clone(data), request: req.Clone()}
		return nil
	})
	if err != nil {
		problems = append(problems, LoadError{File: s.Root, Err: err})
	}
	var labels func(*model.Collection)
	labels = func(folder *model.Collection) {
		file := path.Join(folder.Path, ".resources/definition.yaml")
		data, err := root.ReadFile(file)
		if err == nil {
			var definition struct {
				Name string `yaml:"name"`
			}
			if yaml.Unmarshal(data, &definition) != nil {
				problems = append(problems, LoadError{File: file, Err: fmt.Errorf("invalid native folder definition")})
			} else if definition.Name != "" {
				folder.Name = definition.Name
			}
		} else if !errors.Is(err, fs.ErrNotExist) {
			problems = append(problems, LoadError{File: file, Err: err})
		}
		for _, child := range folder.Children {
			labels(child)
		}
	}
	labels(coll)
	coll.Sort()
	if len(problems) == 0 {
		s.scopes = scopes
	}
	return coll, problems
}

func nativeValue(node *yaml.Node, key string) string {
	if value := nativeNode(node, key); value != nil {
		return value.Value
	}
	return ""
}

func nativeNode(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func nativeRows(node *yaml.Node) []model.KeyValue {
	items := nativeRowNodes(node)
	if items == nil {
		return nil
	}
	rows := make([]model.KeyValue, 0, len(items))
	for _, item := range items {
		rows = append(rows, model.KeyValue{Name: nativeValue(item, "key"), Value: nativeValue(item, "value"), Enabled: nativeValue(item, "disabled") != "true"})
	}
	return rows
}

// Desktop compacts simple key/value rows into a YAML mapping. Expand that
// representation when reading or editing; sequence rows retain their metadata.
func nativeRowNodes(node *yaml.Node) []*yaml.Node {
	if node == nil {
		return nil
	}
	if node.Kind == yaml.SequenceNode {
		return node.Content
	}
	if node.Kind != yaml.MappingNode {
		return nil
	}
	items := make([]*yaml.Node, 0, len(node.Content)/2)
	for i := 0; i < len(node.Content); i += 2 {
		item := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		item.Content = []*yaml.Node{
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: "key"}, node.Content[i],
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: "value"}, node.Content[i+1],
		}
		items = append(items, item)
	}
	return items
}

func nativeSetRows(node *yaml.Node, key string, rows []model.KeyValue) {
	seq := nativeNode(node, key)
	if seq == nil {
		seq = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, seq)
	}
	old := nativeRowNodes(seq)
	used := make(map[int]bool)
	seq.Kind = yaml.SequenceNode
	seq.Tag = "!!seq"
	seq.Style = 0
	seq.Content = nil
	for _, row := range rows {
		item := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		for i, candidate := range old {
			if !used[i] && nativeValue(candidate, "key") == row.Name {
				item = candidate
				used[i] = true
				break
			}
		}
		nativeSet(item, "key", row.Name)
		nativeSet(item, "value", row.Value)
		if !row.Enabled || nativeNode(item, "disabled") != nil {
			nativeSet(item, "disabled", fmt.Sprint(!row.Enabled))
			nativeNode(item, "disabled").Tag = "!!bool"
		}
		seq.Content = append(seq.Content, item)
	}
}

func nativeSet(node *yaml.Node, key, value string) {
	for i := 0; i < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			node.Content[i+1].SetString(value)
			return
		}
	}
	node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value})
}

// Save patches the fields edited by this client; scripts, auth and examples
// from Postman GUI remain authoritative and untouched.
func (s *NativeStore) renderLocked(req model.Request) ([]byte, error) {
	req = nativeWireRequest(req)
	if !validNativePath(req.File) {
		return nil, fmt.Errorf("invalid native request path %q", req.File)
	}
	previous, ok := s.loaded[req.File]
	if !ok {
		return nil, fmt.Errorf("request must be loaded before saving")
	}
	previous.request = nativeWireRequest(previous.request)
	if req.NativeRevision != previous.request.NativeRevision {
		return nil, fmt.Errorf("request changed on disk; reload before saving")
	}
	unchanged := req.Clone()
	unchanged.URL = previous.request.URL
	unchanged.Method = previous.request.Method
	unchanged.Name = previous.request.Name
	unchanged.Description = previous.request.Description
	unchanged.Headers = previous.request.Headers
	unchanged.Query = previous.request.Query
	unchanged.PathParams = previous.request.PathParams
	unchanged.Body = previous.request.Body
	unchanged.Auth = previous.request.Auth
	unchanged.Options.AttachCookies = previous.request.Options.AttachCookies
	unchanged.Options.VerifySSL = previous.request.Options.VerifySSL
	unchanged.Options.FollowRedirects = previous.request.Options.FollowRedirects
	if !reflect.DeepEqual(unchanged, previous.request) {
		return nil, fmt.Errorf("this native document edit is not supported yet; no changes were saved")
	}
	root, err := os.OpenRoot(s.Root)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	current, err := root.ReadFile(req.File)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(current, previous.data) {
		return nil, fmt.Errorf("request changed on disk; reload before saving")
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(current, &doc); err != nil {
		return nil, err
	}
	if req.Options.VerifySSL != previous.request.Options.VerifySSL || req.Options.FollowRedirects != previous.request.Options.FollowRedirects || req.Options.AttachCookies != previous.request.Options.AttachCookies {
		settings := nativeNode(doc.Content[0], "settings")
		if settings == nil {
			settings = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			doc.Content[0].Content = append(doc.Content[0].Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "settings"}, settings)
		}
		for key, pair := range map[string][2]bool{"strictSSL": {previous.request.Options.VerifySSL, req.Options.VerifySSL}, "followRedirects": {previous.request.Options.FollowRedirects, req.Options.FollowRedirects}, "disableCookies": {!previous.request.Options.AttachCookies, !req.Options.AttachCookies}} {
			if pair[0] != pair[1] {
				nativeSet(settings, key, fmt.Sprint(pair[1]))
				nativeNode(settings, key).Tag = "!!bool"
			}
		}
	}
	if req.Auth != previous.request.Auth {
		if err := nativeSetAuth(doc.Content[0], req.Auth); err != nil {
			return nil, err
		}
	}
	if !reflect.DeepEqual(req.Headers, previous.request.Headers) {
		nativeSetRows(doc.Content[0], "headers", req.Headers)
	}
	if !reflect.DeepEqual(req.Query, previous.request.Query) {
		nativeSetRows(doc.Content[0], "queryParams", req.Query)
	}
	if !reflect.DeepEqual(req.PathParams, previous.request.PathParams) {
		nativeSetRows(doc.Content[0], "pathVariables", req.PathParams)
	}
	if !reflect.DeepEqual(req.Body, previous.request.Body) {
		if previous.request.Body.Type == model.BodyFile && previous.request.Body.ContentType != "" && req.Body.ContentType != previous.request.Body.ContentType {
			rows := nativeRows(nativeNode(doc.Content[0], "headers"))
			kept := rows[:0]
			for _, row := range rows {
				if row.Enabled && strings.EqualFold(row.Name, "Content-Type") && row.Value == previous.request.Body.ContentType {
					if req.Body.ContentType == "" {
						continue
					}
					row.Value = req.Body.ContentType
				}
				kept = append(kept, row)
			}
			nativeSetRows(doc.Content[0], "headers", kept)
		}

		if err := nativeSetBody(doc.Content[0], req.Body); err != nil {
			return nil, err
		}
	}
	for key, pair := range map[string][2]string{
		"url":         {previous.request.URL, req.URL},
		"method":      {string(previous.request.Method), string(req.Method)},
		"name":        {previous.request.Name, req.Name},
		"description": {previous.request.Description, req.Description},
	} {
		if pair[0] != pair[1] {
			nativeSet(doc.Content[0], key, pair[1])
		}
	}
	data, err := yaml.Marshal(&doc)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// Render builds an export or execution snapshot without saving editor changes.
func (s *NativeStore) Render(req model.Request) ([]byte, error) {
	if req.File == "" {
		return marshalNativeRequest(req)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.renderLocked(req)
}

func (s *NativeStore) Save(req model.Request) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := s.renderLocked(req)
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(s.Root)
	if err != nil {
		return err
	}
	defer root.Close()
	current, err := root.ReadFile(req.File)
	if err != nil {
		return err
	}
	if !bytes.Equal(current, s.loaded[req.File].data) {
		return fmt.Errorf("request changed on disk; reload before saving")
	}
	tmp := path.Join(path.Dir(req.File), ".herdr-postman-"+rand.Text()+".tmp")
	f, err := root.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer root.Remove(tmp)
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = root.Rename(tmp, req.File); err != nil {
		return err
	}
	s.loaded[req.File] = nativeSnapshot{data: bytes.Clone(data), request: req.Clone()}
	return nil
}

var nativeBodyMIME = map[string]string{
	"json": "application/json", "text": "text/plain", "xml": "application/xml",
	"html": "text/html", "javascript": "application/javascript",
}
