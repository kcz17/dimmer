package main

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// Rule is formatted by "[METHOD] [PATH]".
type Rule = string

type RequestFilter struct {
	rules             map[Rule]bool     // Set of rules to compare against.
	refererExclusions map[Rule][]string // Specifies Referer header substrings which should exclude a request from the filter.
}

func NewRequestFilter() *RequestFilter {
	return &RequestFilter{rules: map[Rule]bool{}, refererExclusions: map[Rule][]string{}}
}

func (r *RequestFilter) Matches(path string, method string, referer string) bool {
	rule := toRule(path, method)

	// No rule found.
	if !r.rules[rule] {
		return false
	}

	// Enforce referer exclusions.
	for _, substring := range r.refererExclusions[rule] {
		if strings.Contains(referer, substring) {
			return false
		}
	}

	// Rule found and not excluded.
	return true
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

// Adds a {@link RefererFilter} for an existing rule.
func (r *RequestFilter) AddRefererExclusion(path string, method string, substring string) error {
	path = prependLeadingSlashIfMissing(path)
	rule := toRule(path, method)
	ruleWithoutPrependingSlash := toRule(path[1:], method)

	if !r.rules[rule] {
		return errors.New(fmt.Sprintf("AddRefererExclusion() expected rules contains rule %v; none found", rule))
	}

	if r.refererExclusions[rule] == nil {
		r.refererExclusions[rule] = []string{}
		r.refererExclusions[ruleWithoutPrependingSlash] = []string{}

	}
	r.refererExclusions[rule] = append(r.refererExclusions[rule], substring)
	r.refererExclusions[ruleWithoutPrependingSlash] = append(r.refererExclusions[ruleWithoutPrependingSlash], substring)

	return nil
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
