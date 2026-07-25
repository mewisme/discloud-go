package selfmanaged

import "testing"

func TestResolveWorkers(t *testing.T) {
	t.Parallel()
	cases := []struct {
		req, tokens, chunks, want int
	}{
		{0, 1, 0, 1},
		{0, 6, 0, 6},    // auto = token count
		{0, 6, 3, 3},    // capped by chunks
		{0, 100, 0, 32}, // capped by max
		{2, 8, 10, 2},   // explicit kept
		{8, 2, 10, 8},   // explicit can exceed tokens
		{8, 2, 3, 3},    // explicit capped by chunks
		{-1, 4, 0, 4},   // auto
		{0, 0, 5, 1},    // no tokens → 1, then chunks irrelevant for floor
	}
	for _, tc := range cases {
		got := ResolveWorkers(tc.req, tc.tokens, tc.chunks)
		if got != tc.want {
			t.Fatalf("ResolveWorkers(%d,%d,%d)=%d want %d", tc.req, tc.tokens, tc.chunks, got, tc.want)
		}
	}
}
