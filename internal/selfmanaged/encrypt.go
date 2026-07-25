package selfmanaged

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/chacha20poly1305"
)

const (
	kdfArgon2id   byte = 1
	cipherXChaCha byte = 1
	argonTime          = 3
	argonMemory        = 64 * 1024 // KiB
	argonThreads       = 4
	argonKeyLen        = 32
	saltLen            = 16
)

// passwordExtra layout:
// kdfID(1) kdfVer(1) time(4) memory(4) threads(1) saltLen(1) salt cipherID(1) nonce(24)
func encryptPassword(plain []byte, password string) (ciphertext, extra []byte, err error) {
	salt, err := randomBytes(saltLen)
	if err != nil {
		return nil, nil, err
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, nil, err
	}
	nonce, err := randomBytes(chacha20poly1305.NonceSizeX)
	if err != nil {
		return nil, nil, err
	}
	ct := aead.Seal(nil, nonce, plain, nil)

	extra = make([]byte, 0, 64)
	extra = append(extra, kdfArgon2id, 1)
	var u32 [4]byte
	binary.BigEndian.PutUint32(u32[:], argonTime)
	extra = append(extra, u32[:]...)
	binary.BigEndian.PutUint32(u32[:], argonMemory)
	extra = append(extra, u32[:]...)
	extra = append(extra, argonThreads, byte(len(salt)))
	extra = append(extra, salt...)
	extra = append(extra, cipherXChaCha)
	extra = append(extra, nonce...)
	return ct, extra, nil
}

func decryptPassword(ciphertext, extra []byte, password string) ([]byte, error) {
	if len(extra) < 12 {
		return nil, fmt.Errorf("bad kdf header")
	}
	if extra[0] != kdfArgon2id {
		return nil, fmt.Errorf("unsupported kdf")
	}
	timeCost := binary.BigEndian.Uint32(extra[2:6])
	memory := binary.BigEndian.Uint32(extra[6:10])
	threads := extra[10]
	saltLenB := int(extra[11])
	if len(extra) < 12+saltLenB+1+chacha20poly1305.NonceSizeX {
		return nil, fmt.Errorf("truncated kdf header")
	}
	off := 12
	salt := extra[off : off+saltLenB]
	off += saltLenB
	if extra[off] != cipherXChaCha {
		return nil, fmt.Errorf("unsupported cipher")
	}
	off++
	nonce := extra[off : off+chacha20poly1305.NonceSizeX]
	key := argon2.IDKey([]byte(password), salt, timeCost, memory, threads, argonKeyLen)
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, err
	}
	return aead.Open(nil, nonce, ciphertext, nil)
}

// MinPasswordLen is the minimum accepted password length.
const MinPasswordLen = 8

// ValidatePassword checks password rules.
func ValidatePassword(pw string) error {
	if pw == "" {
		return fmt.Errorf("password must not be empty")
	}
	if len(pw) < MinPasswordLen {
		return fmt.Errorf("password must be at least %d characters", MinPasswordLen)
	}
	return nil
}

// ensure rand is referenced when tests import
var _ = rand.Reader
