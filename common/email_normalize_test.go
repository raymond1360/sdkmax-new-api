package common

import "testing"

func TestNormalizeEmail(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"already normalized", "person@example.com", "person@example.com"},
		{"mixed case", "Person@Example.COM", "person@example.com"},
		{"leading/trailing whitespace", "  person@example.com  ", "person@example.com"},
		{"whitespace and case together", "  Person@Example.com\t", "person@example.com"},
		{"empty string", "", ""},
		{"whitespace only", "   ", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeEmail(tt.in); got != tt.want {
				t.Errorf("NormalizeEmail(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
