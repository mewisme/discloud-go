package appconfig

import (
	"testing"
	"time"

	"github.com/mewisme/discloud-go/internal/store"
)

func TestMergeDBWins(t *testing.T) {
	env := map[string]string{KeyGuildID: "env-guild", KeyChunkSize: "100"}
	values, sources, _ := Merge(env, nil)
	if values[KeyGuildID] != "env-guild" || sources[KeyGuildID] != SourceEnv {
		t.Fatalf("env path: %+v %+v", values, sources)
	}

	now := time.Now().UTC()
	values, sources, updated := Merge(env, []store.ConfigEntry{
		{Key: KeyGuildID, Value: "db-guild", UpdatedAt: now},
	})
	if values[KeyGuildID] != "db-guild" || sources[KeyGuildID] != SourceDB {
		t.Fatalf("db path: %+v %+v", values, sources)
	}
	if updated[KeyGuildID] == "" {
		t.Fatal("expected updatedAt for db row")
	}
}

func TestIsAllowlisted(t *testing.T) {
	if !IsAllowlisted(KeyGuildID) {
		t.Fatal("guild_id should be allowlisted")
	}
	if IsAllowlisted("bot_token") {
		t.Fatal("secrets must not be allowlisted")
	}
}

func TestParseInt64Empty(t *testing.T) {
	n, err := ParseInt64("")
	if err != nil || n != 0 {
		t.Fatalf("got %d %v", n, err)
	}
}
