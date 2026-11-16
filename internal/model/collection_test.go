package model

import (
	"reflect"
	"testing"
)

func TestSortOrdersRequestsByNameOnly(t *testing.T) {
	gql := GraphQLKind.New()
	gql.Name = "step 2"
	c := &Collection{
		Children: []*Collection{{Name: "20"}, {Name: "2"}, {Name: "10"}, {Name: "1"}},
		Requests: []Request{
			{Name: "step 10", Method: MethodGet},
			gql,
			{Name: "step 1", Method: MethodDelete},
			{Name: "step 1a", Method: MethodPost},
		},
	}
	c.Sort()

	var children []string
	for _, child := range c.Children {
		children = append(children, child.Name)
	}
	if want := []string{"1", "2", "10", "20"}; !reflect.DeepEqual(children, want) {
		t.Errorf("children = %v, want %v", children, want)
	}
	var requests []string
	for _, r := range c.Requests {
		requests = append(requests, r.Name)
	}
	if want := []string{"step 1", "step 1a", "step 2", "step 10"}; !reflect.DeepEqual(requests, want) {
		t.Errorf("requests = %v, want %v", requests, want)
	}
}

func TestNaturalCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"2", "10", -1},
		{"a9b", "a10a", -1},
		{"v1.9", "v1.10", -1},
		{"b", "a10", 1},
		{"007", "7", -1}, // same value: leading zeros break the tie deterministically
		{"x", "x", 0},
		{"99999999999999999999", "100000000000000000000", -1}, // beyond int64
	}
	for _, tc := range cases {
		if got := NaturalCompare(tc.a, tc.b); got != tc.want {
			t.Errorf("NaturalCompare(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
		if got := NaturalCompare(tc.b, tc.a); got != -tc.want {
			t.Errorf("NaturalCompare(%q, %q) = %d, want %d", tc.b, tc.a, got, -tc.want)
		}
	}
}
