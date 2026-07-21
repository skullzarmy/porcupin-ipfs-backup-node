package main

import "testing"

func TestCoerceStringSlice(t *testing.T) {
	eq := func(a, b []string) bool {
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}

	tests := []struct {
		name    string
		in      interface{}
		want    []string
		wantErr bool
	}{
		{"nil", nil, []string{}, false},
		{"json array", []interface{}{"auto", "https://r/routing/v1"}, []string{"auto", "https://r/routing/v1"}, false},
		{"json array trims and drops blanks", []interface{}{"  auto ", "", "  "}, []string{"auto"}, false},
		{"string slice", []string{"auto", "x"}, []string{"auto", "x"}, false},
		{"comma string", "auto, https://r/routing/v1", []string{"auto", "https://r/routing/v1"}, false},
		{"newline string", "auto\nhttps://r/routing/v1\n", []string{"auto", "https://r/routing/v1"}, false},
		{"empty string", "", []string{}, false},
		{"non-string array element", []interface{}{"auto", 42}, nil, true},
		{"wrong type", 42, nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := coerceStringSlice(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (result %v)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !eq(got, tt.want) {
				t.Errorf("coerceStringSlice(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
