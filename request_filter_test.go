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
		toRule("/path", http.MethodGet): true,
		toRule("path", http.MethodGet):  true,
	}
	type fields struct {
		rules map[Rule]bool
	}
	type args struct {
		path   string
		method string
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   bool
	}{
		{
			name:   "Matches path and mehtod",
			fields: fields{rules: rules},
			args: args{
				path:   "/path",
				method: http.MethodGet,
			},
			want: true,
		},
		{
			name:   "Matches path but not method",
			fields: fields{rules: rules},
			args: args{
				path:   "/path",
				method: http.MethodDelete,
			},
			want: false,
		},
		{
			name:   "Matches method and path without leading slash",
			fields: fields{rules: rules},
			args: args{
				path:   "path",
				method: http.MethodGet,
			},
			want: true,
		},
		{
			name:   "Does not match path if followed by optional parts",
			fields: fields{rules: rules},
			args: args{
				path:   "/path?foo=bar#fragment",
				method: http.MethodGet,
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &RequestFilter{
				rules: tt.fields.rules,
			}
			if got := r.Matches(tt.args.path, tt.args.method); got != tt.want {
				t.Errorf("Matches() = %v, want %v", got, tt.want)
			}
		})
	}
}
