package appconfig_test

import (
	"context"
	"testing"

	"github.com/mewisme/discloud-go/internal/appconfig"
	"github.com/mewisme/discloud-go/internal/integrity"
)

func TestResolveEffectiveChunkNoGuild(t *testing.T) {
	size, tier, err := appconfig.ResolveEffectiveChunk(context.Background(), nil, "", 50<<20)
	if err != nil {
		t.Fatal(err)
	}
	if size != integrity.DefaultChunkSize || tier != 0 {
		t.Fatalf("size=%d tier=%d, want default 8MiB", size, tier)
	}
}
