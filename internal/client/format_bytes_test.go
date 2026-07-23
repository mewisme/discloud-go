package client

import "testing"

func TestFormatBytes(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KiB"},
		{1536, "1.5 KiB"},
		{1024 * 1024, "1.0 MiB"},
	}
	for _, tc := range cases {
		if got := FormatBytes(tc.n); got != tc.want {
			t.Errorf("FormatBytes(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}
