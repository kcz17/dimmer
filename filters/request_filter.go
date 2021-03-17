package filters

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// RequestFilterRule is formatted by "[METHOD] [PATH]".
type RequestFilterRule = string

// RequestFilter checks whether a given request path-method-referer combination
// matches a rule within its rules set. Matches can be excluded if the referer
// for matching rule contains an exclusion from refererExclusions. The filter
// is insensitive to the leading slash of a path.
//
// A key invariant is that Matches operations must be insensitive of a path's
// leading slash. To keep Matches lookup O(1), AddPath is responsible for O(n)
// string operations which add both leading slash inclusive and exclusive paths
// to the map, enabling O(1) Matches lookup.
type RequestFilter struct {
	// rules are a set of Method-Path combinations which
	rules map[RequestFilterRule]bool
	// refererExclusions specifies substrings which should exclude a request
	// from the filter if they occur inside a Referer header.
	refererExclusions map[RequestFilterRule][]string
}

func NewRequestFilter() *RequestFilter {
	return &RequestFilter{
		rules:             map[RequestFilterRule]bool{},
		refererExclusions: map[RequestFilterRule][]string{},
	}
}

func (r *RequestFilter) Matches(path string, method string, referer string) bool {
	rule := toRequestFilterRule(path, method)

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

	// RequestFilterRule found and not excluded.
	return true
}

// AddPath adds rules a given path and method both inclusive and exclusive of
// the path's leading slash. Both are added to the set at AddPath-time so that
// Matches does not require string manipulation.
func (r *RequestFilter) AddPath(path string, method string) {
	path = prependLeadingSlashIfMissing(path)
	r.rules[toRequestFilterRule(path[1:], method)] = true
	r.rules[toRequestFilterRule(path, method)] = true
}

func (r *RequestFilter) AddPathForAllMethods(path string) {
	methods := []string{http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete}
	for _, method := range methods {
		r.AddPath(path, method)
	}
}

// AddRefererExclusion adds refererExclusions for an existing rule both
// inclusive and exclusive of the given path's leading slash.
func (r *RequestFilter) AddRefererExclusion(path string, method string, substring string) error {
	path = prependLeadingSlashIfMissing(path)
	rule := toRequestFilterRule(path, method)
	ruleWithoutPrependingSlash := toRequestFilterRule(path[1:], method)

	if !r.rules[rule] {
		return errors.New(fmt.Sprintf("AddRefererExclusion() expected rules contains rule %v; none found", rule))
	}

	if r.refererExclusions[rule] == nil {
		r.refererExclusions[rule] = []string{}
		r.refererExclusions[ruleWithoutPrependingSlash] = []string{}

	}
	r.refererExclusions[rule] = append(r.refererExclusions[rule], substring)
	r.refererExclusions[ruleWithoutPrependingSlash] =
		append(r.refererExclusions[ruleWithoutPrependingSlash], substring)

	return nil
}

func toRequestFilterRule(path string, method string) RequestFilterRule {
	return method + " " + path
}
