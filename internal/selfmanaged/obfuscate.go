package selfmanaged

import (
	"bytes"
	"compress/flate"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
)

// Static material for keyless portable obfuscation. Not a secret — any compatible
// DisCloud CLI build can reverse it. Integrity is HMAC-SHA256.
var obfuscateMaterial = []byte("discloud-portable-obfuscation-v1-not-secure-encryption")

const hmacTagLen = 32

// obfuscate: flate → XOR keystream → append HMAC over ciphertext.
// extra holds: nonce(16)
func obfuscate(plain []byte) (payload, extra []byte, err error) {
	nonce, err := randomBytes(16)
	if err != nil {
		return nil, nil, err
	}
	var compressed bytes.Buffer
	w, err := flate.NewWriter(&compressed, flate.DefaultCompression)
	if err != nil {
		return nil, nil, err
	}
	if _, err := w.Write(plain); err != nil {
		return nil, nil, err
	}
	if err := w.Close(); err != nil {
		return nil, nil, err
	}
	raw := compressed.Bytes()
	if len(raw) > maxPayloadBytes {
		return nil, nil, fmt.Errorf("compressed metadata too large")
	}
	keystream(raw, nonce)
	mac := hmac.New(sha256.New, deriveMACKey(nonce))
	mac.Write(raw)
	tag := mac.Sum(nil)
	out := append(raw, tag...)
	return out, nonce, nil
}

func deobfuscate(payload, extra []byte) ([]byte, error) {
	if len(extra) != 16 || len(payload) < hmacTagLen {
		return nil, fmt.Errorf("bad obfuscation framing")
	}
	nonce := extra
	ct := payload[:len(payload)-hmacTagLen]
	tag := payload[len(payload)-hmacTagLen:]
	mac := hmac.New(sha256.New, deriveMACKey(nonce))
	mac.Write(ct)
	if !hmac.Equal(tag, mac.Sum(nil)) {
		return nil, fmt.Errorf("integrity check failed")
	}
	dup := append([]byte(nil), ct...)
	keystream(dup, nonce)
	r := flate.NewReader(bytes.NewReader(dup))
	defer r.Close()
	limited := io.LimitReader(r, int64(maxPayloadBytes)+1)
	out, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if len(out) > maxPayloadBytes {
		return nil, fmt.Errorf("decompressed metadata too large")
	}
	return out, nil
}

func deriveMACKey(nonce []byte) []byte {
	h := hmac.New(sha256.New, obfuscateMaterial)
	h.Write([]byte("mac"))
	h.Write(nonce)
	return h.Sum(nil)
}

func keystream(buf, nonce []byte) {
	var counter uint64
	for i := 0; i < len(buf); {
		var block [8]byte
		binary.BigEndian.PutUint64(block[:], counter)
		h := hmac.New(sha256.New, obfuscateMaterial)
		h.Write([]byte("xor"))
		h.Write(nonce)
		h.Write(block[:])
		ks := h.Sum(nil)
		for j := 0; j < len(ks) && i < len(buf); j, i = j+1, i+1 {
			buf[i] ^= ks[j]
		}
		counter++
	}
}
