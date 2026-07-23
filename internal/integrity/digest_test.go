package integrity

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestFileSHA256SingleChunkEqualsContentHash(t *testing.T) {
	payload := []byte("hello-integrity-world")
	sum := sha256.Sum256(payload)
	contentHex := hex.EncodeToString(sum[:])

	digest, err := FileSHA256([]string{contentHex}, []int64{int64(len(payload))})
	if err != nil {
		t.Fatal(err)
	}
	if digest != contentHex {
		t.Fatalf("single-chunk digest = %s, want content sha256 %s", digest, contentHex)
	}
}

func TestFileSHA256TwoChunksGolden(t *testing.T) {
	c1 := []byte("AAAA")
	c2 := []byte("BB")
	h1 := sha256.Sum256(c1)
	h2 := sha256.Sum256(c2)
	hex1 := hex.EncodeToString(h1[:])
	hex2 := hex.EncodeToString(h2[:])

	got, err := FileSHA256([]string{hex1, hex2}, []int64{4, 2})
	if err != nil {
		t.Fatal(err)
	}
	const golden = "0c8985e7a5b7eb04b48ff7e6d12d1c604c86460e5d2d21bff8f63dd20c616cee"
	if got != golden {
		t.Fatalf("digest = %s, want %s", got, golden)
	}
	// Must differ from either chunk hash alone.
	if got == hex1 || got == hex2 {
		t.Fatal("multi-chunk digest collapsed to a chunk hash")
	}
}

func TestFileSHA256FromReaderMatchesSplit(t *testing.T) {
	payload := bytes.Repeat([]byte("x"), 100)
	fromReader, err := FileSHA256FromReader(bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	contentHex := hex.EncodeToString(sum[:])
	if fromReader != contentHex {
		t.Fatalf("FromReader = %s, want %s", fromReader, contentHex)
	}
}

func TestFileSHA256FromReaderMultiChunk(t *testing.T) {
	// Two full tiny chunks via a custom reader size — use ChunkSize+1 bytes.
	payload := bytes.Repeat([]byte("a"), ChunkSize+1)
	got, err := FileSHA256FromReader(bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	c1 := payload[:ChunkSize]
	c2 := payload[ChunkSize:]
	h1 := sha256.Sum256(c1)
	h2 := sha256.Sum256(c2)
	want, err := FileSHA256(
		[]string{hex.EncodeToString(h1[:]), hex.EncodeToString(h2[:])},
		[]int64{int64(ChunkSize), 1},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestFileSHA256RejectsMismatch(t *testing.T) {
	_, err := FileSHA256([]string{"aa"}, []int64{1, 2})
	if err == nil {
		t.Fatal("expected error")
	}
	_, err = FileSHA256(nil, nil)
	if err == nil {
		t.Fatal("expected empty error")
	}
	_, err = FileSHA256([]string{strings.Repeat("0", 64)}, []int64{-1})
	if err == nil {
		t.Fatal("expected negative size error")
	}
}
