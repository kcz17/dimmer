package main

import (
	"net/http"
	"testing"
)

func Test_prependLeadingSlashIfMissing(t *testing.T) {
	type args struct {
		path string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "Prepends to empty string",
			args: args{path: ""},
			want: "/",
		},
		{
			name: "Does not prepend to /",
			args: args{path: "/"},
			want: "/",
		},
		{
			name: "Prepends to foo",
			args: args{path: "foo"},
			want: "/foo",
		},
		{
			name: "Does not prepend to /foo",
			args: args{path: "/foo"},
			want: "/foo",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := prependLeadingSlashIfMissing(tt.args.path); got != tt.want {
				t.Errorf("prependLeadingSlashIfMissing() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRequestFilter_Matches(t *testing.T) {
	rules := map[Rule]bool{
		toRule("/path", http.MethodGet):                      true,
		toRule("path", http.MethodGet):                       true,
		toRule("/pathWithRefererExclusions", http.MethodGet): true,
		toRule("pathWithRefererExclusions", http.MethodGet):  true,
	}
	refererExclusions := map[Rule][]string{
		toRule("/pathWithRefererExclusions", http.MethodGet): {
			"foo", "bar",
		},
		toRule("pathWithRefererExclusions", http.MethodGet): {
			"foo", "bar",
		},
	}
	type fields struct {
		rules             map[Rule]bool
		refererExclusions map[Rule][]string
	}
	type args struct {
		path    string
		method  string
		referer string
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   bool
	}{
		{
			name:   "Matches path and mehtod",
			fields: fields{rules: rules, refererExclusions: refererExclusions},
			args: args{
				path:    "/path",
				method:  http.MethodGet,
				referer: "",
			},
			want: true,
		},
		{
			name:   "Matches path but not method",
			fields: fields{rules: rules, refererExclusions: refererExclusions},
			args: args{
				path:    "/path",
				method:  http.MethodDelete,
				referer: "",
			},
			want: false,
		},
		{
			name:   "Matches method and path without leading slash",
			fields: fields{rules: rules, refererExclusions: refererExclusions},
			args: args{
				path:    "path",
				method:  http.MethodGet,
				referer: "",
			},
			want: true,
		},
		{
			name:   "Does not match path if followed by optional parts",
			fields: fields{rules: rules, refererExclusions: refererExclusions},
			args: args{
				path:    "/path?foo=bar#fragment",
				method:  http.MethodGet,
				referer: "",
			},
			want: false,
		},
		{
			name:   "Matches method and path and \"\" referer not excluded",
			fields: fields{rules: rules, refererExclusions: refererExclusions},
			args: args{
				path:    "/pathWithRefererExclusions",
				method:  http.MethodGet,
				referer: "",
			},
			want: true,
		},
		{
			name:   "Matches method and path and \"baz\" referer not excluded",
			fields: fields{rules: rules, refererExclusions: refererExclusions},
			args: args{
				path:    "/pathWithRefererExclusions",
				method:  http.MethodGet,
				referer: "baz",
			},
			want: true,
		},
		{
			name:   "Matches method and path and referer excluded",
			fields: fields{rules: rules, refererExclusions: refererExclusions},
			args: args{
				path:    "/pathWithRefererExclusions",
				method:  http.MethodGet,
				referer: "bar",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &RequestFilter{
				rules:             tt.fields.rules,
				refererExclusions: tt.fields.refererExclusions,
			}
			if got := r.Matches(tt.args.path, tt.args.method, tt.args.referer); got != tt.want {
				t.Errorf("Matches() = %v, want %v", got, tt.want)
			}
		})
	}
}
