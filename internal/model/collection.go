package model

import (
	"cmp"
	"sort"
	"strings"
	"time"
)

// Collection is a folder of saved requests. Sub-folders are child collections.
type Collection struct {
	Name     string
	Path     string // relative to the collection root; "" for the root
	Requests []Request
	Children []*Collection
}

// Sort orders children and requests by name with NaturalCompare, so "2"
// comes before "10", recursively. Method and kind don't affect the order.
func (c *Collection) Sort() {
	sort.SliceStable(c.Children, func(i, j int) bool { return NaturalCompare(c.Children[i].Name, c.Children[j].Name) < 0 })
	sort.SliceStable(c.Requests, func(i, j int) bool { return NaturalCompare(c.Requests[i].Name, c.Requests[j].Name) < 0 })
	for _, child := range c.Children {
		child.Sort()
	}
}

// NaturalCompare compares strings like strings.Compare, except that runs of
// ASCII digits compare by numeric value of any length. Equal values that are
// written differently ("007" and "7") fall back to strings.Compare.
func NaturalCompare(a, b string) int {
	x, y := a, b
	for x != "" && y != "" {
		if !isDigit(x[0]) || !isDigit(y[0]) {
			if x[0] != y[0] {
				return cmp.Compare(x[0], y[0])
			}
			x, y = x[1:], y[1:]
			continue
		}
		i, j := digitRun(x), digitRun(y)
		nx, ny := strings.TrimLeft(x[:i], "0"), strings.TrimLeft(y[:j], "0")
		if len(nx) != len(ny) {
			return cmp.Compare(len(nx), len(ny))
		}
		if c := strings.Compare(nx, ny); c != 0 {
			return c
		}
		x, y = x[i:], y[j:]
	}
	if c := cmp.Compare(len(x), len(y)); c != 0 {
		return c
	}
	return strings.Compare(a, b)
}

func isDigit(c byte) bool { return '0' <= c && c <= '9' }

func digitRun(s string) int {
	n := 0
	for n < len(s) && isDigit(s[n]) {
		n++
	}
	return n
}

// Walk visits every request in the collection tree, depth first.
func (c *Collection) Walk(fn func(folder *Collection, request Request)) {
	for _, child := range c.Children {
		child.Walk(fn)
	}
	for _, r := range c.Requests {
		fn(c, r)
	}
}

// HistoryEntry is a request that was sent and the response it received.
type HistoryEntry struct {
	ID       int64
	Request  Request
	Response *Response
	SentAt   time.Time
	// Status is StatusOf(Request, Response), read once when the entry is
	// made or loaded rather than each time it's drawn, since a GraphQL
	// status parses the whole body. It isn't saved.
	Status Status `json:"-"`
}

// Variable is a named value available for ${NAME} substitution.
type Variable struct {
	// SessionOverride gives an in-app override priority over request scope.
	// Source is a display label and may also be an environment file name.
	SessionOverride bool
	// Template is retained only for deferred imported variables. Value is its
	// evaluated value in the current scope. Ordinary dotenv values are literal.
	Template *string
	Name     string
	Value    string
	// Source describes where the value came from ("session", "host" or an
	// env file name).
	Source string
	// Overrides are the sources of the values this one replaced, nearest
	// first: with posting.env and staging.env layered, a variable set in
	// both comes from staging.env and overrides posting.env.
	Overrides []string
}

// Environment is a named set of variables loaded from one or more env files,
// layered in order.
type Environment struct {
	Name      string
	Files     []string
	Variables []Variable
}
