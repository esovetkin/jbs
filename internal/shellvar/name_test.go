package shellvar

import "testing"

func TestValidName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{name: "simple", in: "x", want: true},
		{name: "underscore", in: "_x", want: true},
		{name: "digit after first", in: "x1", want: true},
		{name: "empty", in: "", want: false},
		{name: "leading digit", in: "1x", want: false},
		{name: "dot", in: "a.b", want: false},
		{name: "dash", in: "a-b", want: false},
		{name: "space", in: "a b", want: false},
		{name: "unicode", in: "alpha_β", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ValidName(tc.in); got != tc.want {
				t.Fatalf("ValidName(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
