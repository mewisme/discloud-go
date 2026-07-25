package ui

import (
	"fmt"
	"image/color"
	"math"
	"strconv"
	"strings"
)

// hsv is hue [0,360), saturation/value [0,1].
type hsv struct {
	H, S, V float64
}

func parseHexRGB(hex string) (r, g, b uint8, err error) {
	hex = strings.TrimSpace(hex)
	if hex == "" {
		return 0, 0, 0, fmt.Errorf("empty hex")
	}
	if hex[0] == '#' {
		hex = hex[1:]
	}
	switch len(hex) {
	case 3:
		hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
	case 6:
	default:
		return 0, 0, 0, fmt.Errorf("invalid hex %q", hex)
	}
	n, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return 0, 0, 0, err
	}
	return uint8(n >> 16), uint8(n >> 8), uint8(n), nil
}

func rgbToHSV(r, g, b uint8) hsv {
	rf := float64(r) / 255
	gf := float64(g) / 255
	bf := float64(b) / 255
	max := math.Max(rf, math.Max(gf, bf))
	min := math.Min(rf, math.Min(gf, bf))
	v := max
	d := max - min
	var s float64
	if max > 0 {
		s = d / max
	}
	var h float64
	if d > 1e-9 {
		switch {
		case max == rf:
			h = 60 * (gf - bf) / d
			if gf < bf {
				h += 360
			}
		case max == gf:
			h = 60 * ((bf-rf)/d + 2)
		default:
			h = 60 * ((rf-gf)/d + 4)
		}
	}
	return hsv{H: h, S: s, V: v}
}

func (c hsv) toRGBA() color.RGBA {
	h := math.Mod(c.H, 360)
	if h < 0 {
		h += 360
	}
	s := clamp01(c.S)
	v := clamp01(c.V)
	if s <= 1e-9 {
		x := uint8(math.Round(v * 255))
		return color.RGBA{R: x, G: x, B: x, A: 255}
	}
	hh := h / 60
	i := int(math.Floor(hh))
	f := hh - float64(i)
	p := v * (1 - s)
	q := v * (1 - s*f)
	t := v * (1 - s*(1-f))
	var r, g, b float64
	switch i % 6 {
	case 0:
		r, g, b = v, t, p
	case 1:
		r, g, b = q, v, p
	case 2:
		r, g, b = p, v, t
	case 3:
		r, g, b = p, q, v
	case 4:
		r, g, b = t, p, v
	default:
		r, g, b = v, p, q
	}
	return color.RGBA{
		R: uint8(math.Round(r * 255)),
		G: uint8(math.Round(g * 255)),
		B: uint8(math.Round(b * 255)),
		A: 255,
	}
}

func clamp01(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}

// hueLerpLong interpolates hue along the longer arc (bokub/gradient-string pastel style).
func hueLerpLong(h1, h2, t float64) float64 {
	d := math.Mod(h2-h1, 360)
	if d < 0 {
		d += 360
	}
	if d < 180 {
		d -= 360
	}
	return math.Mod(h1+d*t+360, 360)
}

func lerpHSVLong(a, b hsv, t float64) hsv {
	t = clamp01(t)
	return hsv{
		H: hueLerpLong(a.H, b.H, t),
		S: a.S + (b.S-a.S)*t,
		V: a.V + (b.V-a.V)*t,
	}
}

// gradientAt returns the color at t∈[0,1] across hex stops.
// When longHue is true, adjacent stops blend on the long HSV hue arc.
func gradientAt(hexes []string, t float64, longHue bool) color.Color {
	t = clamp01(t)
	if len(hexes) == 0 {
		hexes = DefaultProgressColors()
	}
	stops := make([]hsv, 0, len(hexes))
	rgbs := make([]color.RGBA, 0, len(hexes))
	for _, h := range hexes {
		r, g, b, err := parseHexRGB(h)
		if err != nil {
			continue
		}
		rgbs = append(rgbs, color.RGBA{R: r, G: g, B: b, A: 255})
		stops = append(stops, rgbToHSV(r, g, b))
	}
	if len(stops) == 0 {
		return color.RGBA{R: 0x3f, G: 0x51, B: 0xb1, A: 255}
	}
	if len(stops) == 1 {
		return rgbs[0]
	}
	x := t * float64(len(stops)-1)
	i := int(math.Floor(x))
	if i >= len(stops)-1 {
		return rgbs[len(rgbs)-1]
	}
	local := x - float64(i)
	if longHue {
		return lerpHSVLong(stops[i], stops[i+1], local).toRGBA()
	}
	// Short RGB lerp (matches typical solid-stop blends).
	a, b := rgbs[i], rgbs[i+1]
	return color.RGBA{
		R: uint8(math.Round(float64(a.R) + float64(int(b.R)-int(a.R))*local)),
		G: uint8(math.Round(float64(a.G) + float64(int(b.G)-int(a.G))*local)),
		B: uint8(math.Round(float64(a.B) + float64(int(b.B)-int(a.B))*local)),
		A: 255,
	}
}
