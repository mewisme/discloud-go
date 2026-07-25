package transfer

import "testing"

func TestMaxChunkForTier(t *testing.T) {
	cases := []struct {
		tier int
		want int64
	}{
		{0, 10 * MiB},
		{1, 10 * MiB},
		{2, 50 * MiB},
		{3, 100 * MiB},
		{-1, 10 * MiB},
		{99, 10 * MiB},
	}
	for _, c := range cases {
		if got := MaxChunkForTier(c.tier); got != c.want {
			t.Errorf("tier %d: got %d want %d", c.tier, got, c.want)
		}
	}
}

func TestMBRoundTrip(t *testing.T) {
	if MBToBytes(50) != 50*MiB {
		t.Fatal(MBToBytes(50))
	}
	if BytesToMB(50*MiB) != 50 {
		t.Fatal(BytesToMB(50 * MiB))
	}
	if FormatChunkMB(10*MiB) != "10 MB" {
		t.Fatal(FormatChunkMB(10 * MiB))
	}
}

func TestResolveChunkSizeDowngrade(t *testing.T) {
	r := ResolveChunkSize(50*MiB, 2, 0, false)
	if !r.Downgraded || r.EffectiveSize != 10*MiB {
		t.Fatalf("%+v", r)
	}
}

func TestResolveChunkSizeUpgradeInteractive(t *testing.T) {
	r := ResolveChunkSize(10*MiB, 0, 3, true)
	if !r.NeedsUpgrade || r.EffectiveSize != 10*MiB {
		t.Fatalf("%+v", r)
	}
	r2 := ResolveChunkSize(10*MiB, 0, 3, false)
	if r2.NeedsUpgrade || r2.EffectiveSize != 10*MiB {
		t.Fatalf("%+v", r2)
	}
}

func TestResolveChunkSizeInvalid(t *testing.T) {
	r := ResolveChunkSize(0, 0, 2, false)
	if !r.InvalidFixed || r.EffectiveSize != 50*MiB {
		t.Fatalf("%+v", r)
	}
}
