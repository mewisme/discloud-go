package main

import "testing"

func TestMaskAPIToken(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"dc_abcde", "dc_*****"},
		{"dc_abcdefghij", "dc_*****fghij"},
		{"dc_1234567890abcde", "dc_**********abcde"},
		{"plaintextsecret", "**********ecret"},
	}
	for _, tc := range cases {
		if got := maskAPIToken(tc.in); got != tc.want {
			t.Fatalf("maskAPIToken(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
