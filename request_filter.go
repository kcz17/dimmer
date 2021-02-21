package main

import "net/http"

type RequestFilter struct {
	rules map[Rule]bool // Set of rules to compare against.
}

type Rule struct {
	Path   string
	Method string
}

func NewRequestFilter() *RequestFilter {
	return &RequestFilter{rules: map[Rule]bool{}}
}

func (r *RequestFilter) Matches(path string, method string) bool {
	path = prependLeadingSlashIfMissing(path)
	return r.rules[Rule{Path: path, Method: method}] == true
}

func (r *RequestFilter) AddPath(path string, method string) {
	path = prependLeadingSlashIfMissing(path)
	r.rules[Rule{Path: path, Method: method}] = true
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
