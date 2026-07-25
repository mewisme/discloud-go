package ui

import "testing"

func TestHueLerpLongTakesLongArc(t *testing.T) {
	t.Parallel()
	// 10° → 20° short arc is +10; long arc would be -350. Function forces long when short < 180...
	// Wait: if d=10 < 180, d -= 360 → -350. Midpoint from 10: 10-175 = -165 → mod 360 = 195.
	mid := hueLerpLong(10, 20, 0.5)
	if mid < 180 || mid > 210 {
		t.Fatalf("expected long-arc mid near ~195, got %v", mid)
	}
}

func TestGradientAtLongHueEndpoints(t *testing.T) {
	t.Parallel()
	c0 := gradientAt([]string{"#74ebd5", "#acb6e5"}, 0, true)
	c1 := gradientAt([]string{"#74ebd5", "#acb6e5"}, 1, true)
	r0, g0, b0, _ := c0.RGBA()
	r1, g1, b1, _ := c1.RGBA()
	// colorful/hex endpoints should stay near start/end (8-bit).
	if r0>>8 == r1>>8 && g0>>8 == g1>>8 && b0>>8 == b1>>8 {
		t.Fatal("endpoints should differ")
	}
}

func TestParseHexRGBShort(t *testing.T) {
	t.Parallel()
	r, g, b, err := parseHexRGB("#f00")
	if err != nil || r != 255 || g != 0 || b != 0 {
		t.Fatalf("got %d %d %d err=%v", r, g, b, err)
	}
}
