package main

import "net/http"

// Rule is formatted by "[METHOD] [PATH]".
type Rule = string

type RequestFilter struct {
	rules map[Rule]bool // Set of rules to compare against.
}

func NewRequestFilter() *RequestFilter {
	return &RequestFilter{rules: map[Rule]bool{}}
}

func (r *RequestFilter) Matches(path string, method string) bool {
	return r.rules[toRule(path, method)] == true
}

// AddPath adds rules a given path and method both inclusive and exclusive of
// the path's leading slash.
func (r *RequestFilter) AddPath(path string, method string) {
	path = prependLeadingSlashIfMissing(path)
	r.rules[toRule(path[1:], method)] = true
	r.rules[toRule(path, method)] = true
}

func (r *RequestFilter) AddPathForAllMethods(path string) {
	methods := []string{http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete}
	for _, method := range methods {
		r.AddPath(path, method)
	}
}

func prependLeadingSlashIfMissing(path string) string {
	if len(path) == 0 || path[0] != '/' {
		path = "/" + path
	}
	return path
}

func toRule(path string, method string) Rule {
	return method + " " + path
}
