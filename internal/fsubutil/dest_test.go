package fsubutil

import "testing"

func TestDestName(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{path: "", want: ""},
		{path: "   ", want: ""},
		{path: ".", want: ""},
		{path: "..", want: ""},
		{path: "./input.tpl", want: "input.tpl"},
		{path: "dir/input.tpl", want: "input.tpl"},
		{path: " dir/input.tpl ", want: "input.tpl"},
		{path: "dir/../input.tpl", want: "input.tpl"},
	}
	for _, tt := range tests {
		if got := DestName(tt.path); got != tt.want {
			t.Fatalf("DestName(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}
