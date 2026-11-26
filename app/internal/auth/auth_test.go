package auth

import (
	"strings"
	"testing"
)

func TestValidationBoundaries(t *testing.T) {
	for _, tc := range []struct {
		login, password string
		valid           bool
	}{
		{"abc", strings.Repeat("a", 12), true}, {strings.Repeat("z", 32), strings.Repeat("a", 72), true},
		{"abc", strings.Repeat("я", 36), true}, {"abc", strings.Repeat("я", 37), false},
		{"abc", strings.Repeat("a", 73), false}, {"abc", strings.Repeat("я", 11), false},
		{"ab", strings.Repeat("a", 12), false}, {"abc", strings.Repeat(" ", 12), true},
	} {
		if (Validate(tc.login, tc.password) == nil) != tc.valid {
			t.Errorf("validation failed for %q, %d bytes", tc.login, len(tc.password))
		}
	}
	if NormalizeLogin(" ALIce ") != "alice" {
		t.Fatal("normalization")
	}
}
