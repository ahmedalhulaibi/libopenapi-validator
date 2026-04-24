// Copyright 2026 Princess B33f Heavy Industries / Dave Shanley
// SPDX-License-Identifier: MIT

// Package radix provides a radix tree (prefix tree) implementation optimized for
// URL path matching with support for parameterized segments.
//
// The tree provides O(k) lookup complexity where k is the number of path segments
// (typically 3-5 for REST APIs), making it ideal for routing and path matching.
//
// Example usage:
//
//	tree := radix.New[*MyHandler]()
//	tree.Insert("/users/{id}", handler1)
//	tree.Insert("/users/{id}/posts", handler2)
//
//	handler, path, found := tree.Lookup("/users/123/posts")
//	// handler = handler2, path = "/users/{id}/posts", found = true
package radix

import "strings"

// Tree is a radix tree optimized for URL path matching.
// It supports both literal path segments and parameterized segments like {id}.
// T is the type of value stored at leaf nodes.
type Tree[T any] struct {
	root *node[T]
	size int
}

// node represents a node in the radix tree.
type node[T any] struct {
	// children maps literal path segments to child nodes
	children map[string]*node[T]

	// paramChild handles parameterized segments like {id}
	// Only one param child is allowed per node
	paramChild *node[T]

	// paramName stores the parameter name without braces (e.g., "id" from "{id}")
	paramName string

	// suffixChildren handles segments like {param}:action where a param is
	// followed by a literal suffix. Maps suffix → child node.
	// During lookup, the request segment is checked for each suffix.
	suffixChildren map[string]*node[T]

	// leaves contains the stored values and path templates for endpoints.
	// Multiple leaves occur when different param names map to the same
	// node (e.g., /v1/{groupName} and /v1/{name}).
	leaves []leafData[T]
}

// leafData stores a value and its original path template.
type leafData[T any] struct {
	value T
	path  string
}

func (n *node[T]) firstLeaf() *leafData[T] {
	if len(n.leaves) == 0 {
		return nil
	}

	return &n.leaves[0]
}

// New creates a new empty radix tree.
func New[T any]() *Tree[T] {
	return &Tree[T]{
		root: &node[T]{
			children: make(map[string]*node[T]),
		},
	}
}

// Insert adds a path and its associated value to the tree.
// The path should use {param} syntax for parameterized segments.
// Examples: "/users", "/users/{id}", "/users/{userId}/posts/{postId}"
//
// Returns true if a new path was inserted, false if an existing path was updated.
func (t *Tree[T]) Insert(path string, value T) bool {
	if t.root == nil {
		t.root = &node[T]{children: make(map[string]*node[T])}
	}

	segments := splitPath(path)
	n := t.root

	for _, seg := range segments {
		if suffix := paramSuffix(seg); suffix != "" {
			// Parameter with literal suffix: {param}:action
			if n.suffixChildren == nil {
				n.suffixChildren = make(map[string]*node[T])
			}

			child, exists := n.suffixChildren[suffix]
			if !exists {
				child = &node[T]{
					children:  make(map[string]*node[T]),
					paramName: extractParamName(seg),
				}
				n.suffixChildren[suffix] = child
			}

			n = child
		} else if isParam(seg) {
			// Pure parameter segment
			if n.paramChild == nil {
				n.paramChild = &node[T]{
					children:  make(map[string]*node[T]),
					paramName: extractParamName(seg),
				}
			}
			n = n.paramChild
		} else {
			// Literal segment
			child, exists := n.children[seg]
			if !exists {
				child = &node[T]{children: make(map[string]*node[T])}
				n.children[seg] = child
			}
			n = child
		}
	}

	// Check if this path already exists (update vs new).
	for i, l := range n.leaves {
		if l.path == path {
			n.leaves[i] = leafData[T]{value: value, path: path}

			return false
		}
	}

	n.leaves = append(n.leaves, leafData[T]{value: value, path: path})
	t.size++

	return true
}

// Lookup finds the value for a given URL path.
// Returns the value, the matched path template, and whether a match was found.
//
// Literal matches take precedence over parameter matches per OpenAPI specification.
// For example, "/users/admin" will match "/users/admin" before "/users/{id}".
func (t *Tree[T]) Lookup(urlPath string) (value T, matchedPath string, found bool) {
	var zero T
	if t.root == nil {
		return zero, "", false
	}

	segments := splitPath(urlPath)
	leaf := t.lookupRecursive(t.root, segments, 0)

	if leaf != nil {
		return leaf.value, leaf.path, true
	}
	return zero, "", false
}

// LookupAll returns all matching leaves for a given URL path. Multiple leaves
// occur when different param names map to the same tree node (e.g.,
// /v1/{groupName} and /v1/{name}). The caller should check which leaf's
// PathItem has the desired HTTP method.
func (t *Tree[T]) LookupAll(urlPath string) []leafData[T] {
	if t.root == nil {
		return nil
	}

	segments := splitPath(urlPath)
	n := t.lookupNode(t.root, segments, 0)

	if n == nil {
		return nil
	}

	return n.leaves
}

func (t *Tree[T]) lookupNode(n *node[T], segments []string, depth int) *node[T] {
	if depth == len(segments) {
		if len(n.leaves) > 0 {
			return n
		}

		return nil
	}

	seg := segments[depth]

	if child, exists := n.children[seg]; exists {
		if result := t.lookupNode(child, segments, depth+1); result != nil {
			return result
		}
	}

	for suffix, child := range n.suffixChildren {
		if strings.HasSuffix(seg, suffix) {
			if result := t.lookupNode(child, segments, depth+1); result != nil {
				return result
			}
		}
	}

	if n.paramChild != nil {
		if result := t.lookupNode(n.paramChild, segments, depth+1); result != nil {
			return result
		}
	}

	return nil
}

// lookupRecursive performs the tree traversal.
// It prioritizes literal matches over parameter matches.
func (t *Tree[T]) lookupRecursive(n *node[T], segments []string, depth int) *leafData[T] {
	// Base case: consumed all segments
	if depth == len(segments) {
		return n.firstLeaf()
	}

	seg := segments[depth]

	// Try literal match first (highest specificity)
	if child, exists := n.children[seg]; exists {
		if result := t.lookupRecursive(child, segments, depth+1); result != nil {
			return result
		}
	}

	// Try suffix matches (e.g., {param}:action — higher specificity than pure param)
	for suffix, child := range n.suffixChildren {
		if strings.HasSuffix(seg, suffix) {
			if result := t.lookupRecursive(child, segments, depth+1); result != nil {
				return result
			}
		}
	}

	// Fall back to parameter match (lowest specificity)
	if n.paramChild != nil {
		if result := t.lookupRecursive(n.paramChild, segments, depth+1); result != nil {
			return result
		}
	}

	return nil
}

// Size returns the number of paths stored in the tree.
func (t *Tree[T]) Size() int {
	return t.size
}

// Clear removes all entries from the tree.
func (t *Tree[T]) Clear() {
	t.root = &node[T]{children: make(map[string]*node[T])}
	t.size = 0
}

// Walk calls the given function for each path in the tree.
// The function receives the path template and its associated value.
// If the function returns false, iteration stops.
func (t *Tree[T]) Walk(fn func(path string, value T) bool) {
	if t.root == nil {
		return
	}
	t.walkRecursive(t.root, fn)
}

func (t *Tree[T]) walkRecursive(n *node[T], fn func(path string, value T) bool) bool {
	for _, l := range n.leaves {
		if !fn(l.path, l.value) {
			return false
		}
	}

	for _, child := range n.children {
		if !t.walkRecursive(child, fn) {
			return false
		}
	}

	for _, child := range n.suffixChildren {
		if !t.walkRecursive(child, fn) {
			return false
		}
	}

	if n.paramChild != nil {
		if !t.walkRecursive(n.paramChild, fn) {
			return false
		}
	}

	return true
}

// splitPath splits a path into segments, removing empty segments.
// "/users/{id}/posts" -> ["users", "{id}", "posts"]
func splitPath(path string) []string {
	path = strings.Trim(path, "/")
	if path == "" {
		return nil
	}

	parts := strings.Split(path, "/")

	// Filter out empty segments (from double slashes, etc.)
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// isParam checks if a segment is a pure parameter (e.g., "{id}")
func isParam(seg string) bool {
	return len(seg) > 2 && seg[0] == '{' && seg[len(seg)-1] == '}'
}

// isParamWithSuffix checks if a segment is a parameter followed by a literal
// suffix, e.g., "{rootDomain}:rotateChallenges". Returns the suffix (including
// the separator) or "" if not a param-with-suffix.
func paramSuffix(seg string) string {
	closeBrace := strings.IndexByte(seg, '}')
	if closeBrace < 0 || seg[0] != '{' {
		return ""
	}

	if closeBrace == len(seg)-1 {
		return "" // pure param, no suffix
	}

	return seg[closeBrace+1:]
}

// extractParamName extracts the parameter name from a segment.
// "{id}" -> "id", "{userId}" -> "userId", "{rootDomain}:action" -> "rootDomain"
func extractParamName(seg string) string {
	if len(seg) < 3 || seg[0] != '{' {
		return seg
	}

	closeBrace := strings.IndexByte(seg, '}')
	if closeBrace < 2 {
		return seg
	}

	return seg[1:closeBrace]
}
