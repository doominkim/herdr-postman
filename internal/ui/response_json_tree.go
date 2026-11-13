package ui

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/x/ansi"
	t "github.com/darrenburns/terma"
	"github.com/theory/jsonpath"
	"github.com/theory/jsonpath/spec"
)

// jsonNode is one value of a response body shown in the JSON tree.
type jsonNode struct {
	Path  string // RFC 9535 normalized JSONPath, e.g. $['users'][2]['name'].
	Label string // Object key, array index in brackets, or $ for the root.
	Value any    // map[string]any, []any, string, json.Number, bool or nil, for JSONPath.
	// Raw is the value exactly as the response wrote it, so copies keep the
	// response's key order and number lexemes.
	Raw []byte
	// previewParts are an object's first entries for its preview (see preview).
	previewParts []string
	// ancestor marks a node shown only as context for a JSONPath match.
	ancestor bool
}

// jsonTreeState is a response body as a tree. Unlike terma's JSONViewer it
// keeps object keys in the order the response sent them, and lists the
// document's top-level entries without a root row.
type jsonTreeState struct {
	Tree   *t.TreeState[jsonNode]
	scroll *t.ScrollState
	body   []byte
	root   t.TreeNode[jsonNode]
	// width is the tree's width as of its last layout, which previews fill.
	// Until the first layout it is 0.
	width t.Signal[int]

	path    *jsonpath.Path
	query   t.Signal[string]
	matches t.Signal[int]
}

// newJSONTreeState parses one JSON document, with only its top level showing.
// Anything else, including trailing values, is an error.
func newJSONTreeState(body []byte) (*jsonTreeState, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	var raw json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		return nil, err
	}
	raw = within(body, raw, decoder)
	if _, err := decoder.Token(); err != io.EOF {
		return nil, errors.New("expected a single JSON value")
	}
	root, err := parseJSONNode(raw, "$", nil)
	if err != nil {
		return nil, err
	}
	s := &jsonTreeState{
		Tree:    t.NewTreeState[jsonNode](nil),
		scroll:  t.NewScrollState(),
		body:    body,
		root:    root,
		width:   t.NewSignal(0),
		query:   t.NewSignal(""),
		matches: t.NewSignal(1),
	}
	s.applyQuery()
	s.collapseAll()
	return s, nil
}

// parseJSONNode builds the node for raw, keeping object keys in their
// written order. A repeated key keeps its first position and last value, as
// decoding it would.
func parseJSONNode(raw json.RawMessage, label string, path spec.NormalizedPath) (t.TreeNode[jsonNode], error) {
	raw = bytes.TrimSpace(raw)
	node := t.TreeNode[jsonNode]{Data: jsonNode{Path: path.String(), Label: label, Raw: raw}}
	child := func(raw json.RawMessage, label string, selector spec.NormalSelector) (t.TreeNode[jsonNode], error) {
		childPath := append(spec.NormalizedPath{}, path...)
		return parseJSONNode(raw, label, append(childPath, selector))
	}
	switch raw[0] {
	case '{':
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.Token()
		object := map[string]any{}
		index := map[string]int{}
		for decoder.More() {
			token, err := decoder.Token()
			if err != nil {
				return node, err
			}
			key := token.(string)
			var value json.RawMessage
			if err := decoder.Decode(&value); err != nil {
				return node, err
			}
			value = within(raw, value, decoder)
			c, err := child(value, jsonKeyLabel(key), spec.Name(key))
			if err != nil {
				return node, err
			}
			object[key] = c.Data.Value
			if i, ok := index[key]; ok {
				node.Children[i] = c
				continue
			}
			index[key] = len(node.Children)
			node.Children = append(node.Children, c)
		}
		node.Data.Value = object
		node.Data.previewParts = previewParts(node.Children)
	case '[':
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.Token()
		array := []any{}
		for i := 0; decoder.More(); i++ {
			var value json.RawMessage
			if err := decoder.Decode(&value); err != nil {
				return node, err
			}
			value = within(raw, value, decoder)
			c, err := child(value, fmt.Sprintf("[%d]", i), spec.Index(i))
			if err != nil {
				return node, err
			}
			array = append(array, c.Data.Value)
			node.Children = append(node.Children, c)
		}
		node.Data.Value = array
	default:
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if err := decoder.Decode(&node.Data.Value); err != nil {
			return node, err
		}
	}
	return node, nil
}

const (
	// maxPreviewWidth bounds the entries kept for previews: wider than any
	// terminal row, so the row's width decides what is shown.
	maxPreviewWidth = 512
	// previewStringWidth bounds each string in a preview, quotes included.
	previewStringWidth = 20
)

// previewParts are an object's entries as previews show them, "key: value",
// as many as could fit in maxPreviewWidth.
func previewParts(entries []t.TreeNode[jsonNode]) []string {
	parts := []string{}
	width := 0
	for _, entry := range entries {
		if width > maxPreviewWidth {
			break
		}
		part := entry.Data.Label + ": " + previewValue(entry.Data)
		parts = append(parts, part)
		width += ansi.StringWidth(part) + 2
	}
	return parts
}

// preview is a closed object in at most width cells: its entries until the
// next would not fit, then "…", such as {id: 1, name: "Ada", …}. Nested
// objects and arrays show as {…} and […]. Arrays and empty objects have none.
func (n jsonNode) preview(width int) string {
	object, ok := n.Value.(map[string]any)
	if !ok || len(object) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("{")
	used := 1
	for i := range len(object) {
		separator := ""
		if i > 0 {
			separator = ", "
		}
		// The last entry only needs room for "}"; others leave room for ", …}".
		room := 4
		if i == len(object)-1 {
			room = 1
		}
		part := separator
		if i < len(n.previewParts) {
			part += n.previewParts[i]
		}
		if i == len(n.previewParts) || used+ansi.StringWidth(part)+room > width {
			b.WriteString(separator + "…")
			break
		}
		b.WriteString(part)
		used += ansi.StringWidth(part)
	}
	b.WriteString("}")
	return b.String()
}

// previewValue is a value as an object preview shows it.
func previewValue(node jsonNode) string {
	switch value := node.Value.(type) {
	case map[string]any:
		if len(value) == 0 {
			return "{}"
		}
		return "{…}"
	case []any:
		if len(value) == 0 {
			return "[]"
		}
		return "[…]"
	case string:
		if runes := []rune(string(node.Raw)); len(runes) > previewStringWidth {
			return string(runes[:previewStringWidth-2]) + `…"`
		}
	}
	return string(node.Raw)
}

// within is the part of src that decoder just decoded into decoded, so
// nodes share the body instead of each keeping a copy.
func within(src []byte, decoded json.RawMessage, decoder *json.Decoder) []byte {
	end := int(decoder.InputOffset())
	return src[end-len(decoded) : end]
}

// jsonKeyLabel shows a key as written, quoting only keys that would be
// unreadable bare: empty ones and ones with spaces at the ends or control
// characters.
func jsonKeyLabel(key string) string {
	if key == "" || strings.TrimSpace(key) != key || strings.ContainsFunc(key, func(r rune) bool { return r < ' ' || r == 0x7f }) {
		quoted, _ := json.Marshal(key)
		return string(quoted)
	}
	return key
}

// rows are the document's top-level entries. A scalar or empty document has
// none, so it is shown as its own row.
func (s *jsonTreeState) rows() []t.TreeNode[jsonNode] {
	if len(s.root.Children) == 0 {
		return []t.TreeNode[jsonNode]{s.root}
	}
	return s.root.Children
}

// Query returns the last valid query, subscribing to changes.
func (s *jsonTreeState) Query() string { return s.query.Get() }

// MatchCount is the number of values the query selects; an empty query
// selects the whole document.
func (s *jsonTreeState) MatchCount() int { return s.matches.Get() }

// SetQuery filters by RFC 9535 JSONPath. Matches keep their ancestors and
// whole subtrees, and paths and indices refer to the original document.
// Ancestors of matches open. An invalid query leaves the last valid view.
func (s *jsonTreeState) SetQuery(query string) error {
	query = strings.TrimSpace(query)
	if query == s.query.Peek() {
		return nil
	}
	var path *jsonpath.Path
	if query != "" {
		var err error
		if path, err = jsonpath.Parse(query); err != nil {
			return err
		}
	}
	s.path = path
	s.query.Set(query)
	s.applyQuery()
	return nil
}

func (s *jsonTreeState) applyQuery() {
	nodes, count := s.rows(), 1
	if s.path != nil {
		matched := map[string]bool{}
		ancestors := map[string]bool{}
		for _, node := range s.path.SelectLocated(s.root.Data.Value) {
			matched[node.Path.String()] = true
			for i := range len(node.Path) {
				ancestors[node.Path[:i].String()] = true
			}
		}
		count = len(matched)
		if !matched[s.root.Data.Path] {
			nodes = filterJSONNodes(nodes, matched, ancestors)
		}
		collapsed := make(map[string]bool, len(s.Tree.Collapsed.Peek()))
		for id := range s.Tree.Collapsed.Peek() {
			if !ancestors[id] {
				collapsed[id] = true
			}
		}
		s.Tree.Collapsed.Set(collapsed)
	}
	s.Tree.Nodes.Set(nodes)
	s.Tree.ClearSelection()
	s.Tree.ClearAnchor()
	cursor := []int{}
	if len(nodes) > 0 {
		cursor = []int{0}
	}
	s.Tree.CursorPath.Set(cursor)
	s.scroll.SetOffset(0)
	s.scroll.SetOffsetX(0)
	s.matches.Set(count)
}

func filterJSONNodes(nodes []t.TreeNode[jsonNode], matched, ancestors map[string]bool) []t.TreeNode[jsonNode] {
	filtered := make([]t.TreeNode[jsonNode], 0, len(nodes))
	for _, node := range nodes {
		if matched[node.Data.Path] {
			filtered = append(filtered, node)
		} else if ancestors[node.Data.Path] {
			node.Data.ancestor = true
			node.Children = filterJSONNodes(node.Children, matched, ancestors)
			filtered = append(filtered, node)
		}
	}
	return filtered
}

// collapseAll closes every object and array. The tree only learns that
// nodes are identified by path when it builds, so this keys them itself.
func (s *jsonTreeState) collapseAll() {
	collapsed := map[string]bool{}
	var walk func([]t.TreeNode[jsonNode])
	walk = func(nodes []t.TreeNode[jsonNode]) {
		for _, node := range nodes {
			if len(node.Children) > 0 {
				collapsed[node.Data.Path] = true
				walk(node.Children)
			}
		}
	}
	walk(s.Tree.Nodes.Peek())
	s.Tree.Collapsed.Set(collapsed)
}

// expandAll opens every object and array.
func (s *jsonTreeState) expandAll() {
	s.Tree.Collapsed.Set(map[string]bool{})
}

// toggleCursor opens or closes the object or array under the cursor.
func (s *jsonTreeState) toggleCursor() {
	path := s.Tree.CursorPath.Peek()
	if node, ok := s.Tree.NodeAtPath(path); ok && len(node.Children) > 0 {
		s.Tree.Toggle(path)
	}
}

// jsonNodeSummary is what a row shows after its label: an object's or
// array's size, or ": " and a scalar as the response wrote it.
func jsonNodeSummary(node jsonNode, theme t.ThemeData) (separator, text string, color t.Color) {
	switch value := node.Value.(type) {
	case map[string]any:
		if len(value) == 0 {
			return " ", "{}", theme.TextMuted
		}
		return " ", fmt.Sprintf("{%d}", len(value)), theme.TextMuted
	case []any:
		if len(value) == 0 {
			return " ", "[]", theme.TextMuted
		}
		return " ", fmt.Sprintf("(%d)", len(value)), theme.TextMuted
	case string:
		color = theme.Success
	case json.Number:
		color = theme.Warning
	case bool:
		color = theme.Accent
	default:
		color = theme.TextMuted
	}
	return ": ", string(node.Raw), color
}

// copyText is the node's value, indented, as the response wrote it.
func (n jsonNode) copyText() string {
	var out bytes.Buffer
	if err := json.Indent(&out, n.Raw, "", "  "); err != nil {
		return string(n.Raw)
	}
	return out.String()
}
