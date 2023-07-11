// Package clibase offers an all-in-one solution for a highly configurable CLI
// application. Within Coder, we use it for all of our subcommands, which
// demands more functionality than cobra/viber offers.
//
// The Command interface is loosely based on the chi middleware pattern and
// http.Handler/HandlerFunc.
package clibase

import (
	"golang.org/x/exp/maps"
	"strings"
)

// Group describes a hierarchy of groups that an option or command belongs to.
type Group struct {
	Parent      *Group `json:"parent,omitempty"`
	Name        string `json:"name,omitempty"`
	YAML        string `json:"yaml,omitempty"`
	Description string `json:"description,omitempty"`
}

// Ancestry returns a slice of Group values, beginning with the current group and
// ascending to the root. If the group is nil, it returns nil. Changes to the
// returned slice do not affect the original groups.
func (g *Group) Ancestry() []Group {
	if g == nil {
		return nil
	}
	groups := []Group{*g}
	for p := g.Parent; p != nil; p = p.Parent {
		// Prepend to the slice so that the order is correct.
		groups = append([]Group{*p}, groups...)
	}
	return groups
}

// FullName returns the full path of the group from the root to the current group.
// Each group is separated by a " / ". If the group is nil, it returns an empty string.
func (g *Group) FullName() string {
	var names []string
	for _, g := range g.Ancestry() {
		names = append(names, g.Name)
	}
	return strings.Join(names, " / ")
}

// Annotations is an arbitrary key-mapping used to extend the Option and Command types.
// Its methods won't panic if the map is nil.
type Annotations map[string]string

// Mark sets a key-value pair in a copy of the Annotations map, creating a new one if necessary.
// It returns the modified copy, leaving the original map as it is. Suitable for chaining operations.
func (a Annotations) Mark(key string, value string) Annotations {
	var aa Annotations
	if a != nil {
		aa = maps.Clone(a)
	} else {
		aa = make(Annotations)
	}
	aa[key] = value
	return aa
}

// IsSet checks whether the provided key exists in the annotations map. If the
// map is nil or the key is not present, it returns false. Otherwise, it returns true.
func (a Annotations) IsSet(key string) bool {
	if a == nil {
		return false
	}
	_, ok := a[key]
	return ok
}

// Get returns the value associated with the key from the Annotations.
// Returns false if the key does not exist or Annotations is nil, otherwise true.
func (a Annotations) Get(key string) (string, bool) {
	if a == nil {
		return "", false
	}
	v, ok := a[key]
	return v, ok
}
