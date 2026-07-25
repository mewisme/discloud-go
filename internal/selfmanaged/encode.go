package selfmanaged

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
)

var (
	magic                      = [4]byte{'D', 'C', 'L', 'D'}
	ErrInvalidMagic            = errors.New("not a .discloud bundle: invalid magic")
	ErrUnsupportedVersion      = errors.New("unsupported .discloud container version")
	ErrCorruptBundle           = errors.New("Unable to decrypt bundle: the password is incorrect or the file is corrupted.")
	ErrPasswordRequired        = errors.New("password required")
	ErrIdentityMismatch        = errors.New("The current self-upload configuration does not match the configuration used to create this bundle.\n\nCheck the configured bot tokens, guild ID, and channel ID.\nBot token order does not matter.")
	containerVersion      byte = 1
	modeObfuscated        byte = 1
	modePassword          byte = 2
	maxPayloadBytes            = 32 << 20 // 32 MiB metadata cap
)

// DetectBundle reports whether path looks like a .discloud container (by magic).
func DetectBundle(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	var m [4]byte
	if _, err := io.ReadFull(f, m[:]); err != nil {
		return false
	}
	return m == magic
}

// EncodeOptions controls how a bundle is written.
type EncodeOptions struct {
	Password string // empty → obfuscated
}

// Encode writes meta to path as a .discloud container.
func Encode(path string, meta *BundleMeta, opt EncodeOptions) error {
	meta.FormatVersion = MetadataVersion
	if opt.Password != "" {
		meta.Protection = ProtectionPassword
	} else {
		meta.Protection = ProtectionObfuscated
	}
	raw, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	var mode byte
	var payload []byte
	var headerExtra []byte
	if opt.Password != "" {
		mode = modePassword
		payload, headerExtra, err = encryptPassword(raw, opt.Password)
		if err != nil {
			return err
		}
	} else {
		mode = modeObfuscated
		payload, headerExtra, err = obfuscate(raw)
		if err != nil {
			return err
		}
	}
	return writeContainer(path, mode, headerExtra, payload)
}

// DecodeOptions supplies a password when needed.
type DecodeOptions struct {
	Password string
}

// DecodePeekMode reads protection mode without decrypting payload.
func DecodePeekMode(path string) (protection string, err error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	hdr, _, err := readContainerHeader(f)
	if err != nil {
		return "", err
	}
	switch hdr.mode {
	case modePassword:
		return ProtectionPassword, nil
	case modeObfuscated:
		return ProtectionObfuscated, nil
	default:
		return "", ErrUnsupportedVersion
	}
}

// Decode loads and decrypts/deobfuscates a bundle.
func Decode(path string, opt DecodeOptions) (*BundleMeta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return DecodeBytes(data, opt)
}

// DecodeBytes decodes from memory.
func DecodeBytes(data []byte, opt DecodeOptions) (*BundleMeta, error) {
	hdr, rest, err := parseContainer(data)
	if err != nil {
		return nil, err
	}
	var raw []byte
	switch hdr.mode {
	case modePassword:
		if opt.Password == "" {
			return nil, ErrPasswordRequired
		}
		raw, err = decryptPassword(rest, hdr.extra, opt.Password)
		if err != nil {
			return nil, ErrCorruptBundle
		}
	case modeObfuscated:
		raw, err = deobfuscate(rest, hdr.extra)
		if err != nil {
			return nil, ErrCorruptBundle
		}
	default:
		return nil, ErrUnsupportedVersion
	}
	var meta BundleMeta
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil, ErrCorruptBundle
	}
	return &meta, nil
}

type containerHeader struct {
	mode  byte
	extra []byte
}

func writeContainer(path string, mode byte, extra, payload []byte) error {
	if len(payload) > maxPayloadBytes {
		return fmt.Errorf("bundle payload too large")
	}
	buf := make([]byte, 0, 16+len(extra)+len(payload))
	buf = append(buf, magic[:]...)
	buf = append(buf, containerVersion, mode, 0, 0) // flags reserved
	var lens [8]byte
	binary.BigEndian.PutUint32(lens[0:4], uint32(len(extra)))
	binary.BigEndian.PutUint32(lens[4:8], uint32(len(payload)))
	buf = append(buf, lens[:]...)
	buf = append(buf, extra...)
	buf = append(buf, payload...)

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func readContainerHeader(r io.Reader) (containerHeader, []byte, error) {
	var fixed [12]byte
	if _, err := io.ReadFull(r, fixed[:]); err != nil {
		return containerHeader{}, nil, ErrInvalidMagic
	}
	if fixed[0] != magic[0] || fixed[1] != magic[1] || fixed[2] != magic[2] || fixed[3] != magic[3] {
		return containerHeader{}, nil, ErrInvalidMagic
	}
	if fixed[4] != containerVersion {
		return containerHeader{}, nil, ErrUnsupportedVersion
	}
	mode := fixed[5]
	extraLen := binary.BigEndian.Uint32(fixed[8:12])
	// need payload length too — read next 4
	var plenBuf [4]byte
	if _, err := io.ReadFull(r, plenBuf[:]); err != nil {
		return containerHeader{}, nil, ErrCorruptBundle
	}
	payloadLen := binary.BigEndian.Uint32(plenBuf[:])
	if extraLen > uint32(maxPayloadBytes) || payloadLen > uint32(maxPayloadBytes) {
		return containerHeader{}, nil, ErrCorruptBundle
	}
	extra := make([]byte, extraLen)
	if _, err := io.ReadFull(r, extra); err != nil {
		return containerHeader{}, nil, ErrCorruptBundle
	}
	payload := make([]byte, payloadLen)
	if _, err := io.ReadFull(r, payload); err != nil {
		return containerHeader{}, nil, ErrCorruptBundle
	}
	return containerHeader{mode: mode, extra: extra}, payload, nil
}

func parseContainer(data []byte) (containerHeader, []byte, error) {
	if len(data) < 16 {
		return containerHeader{}, nil, ErrInvalidMagic
	}
	if data[0] != magic[0] || data[1] != magic[1] || data[2] != magic[2] || data[3] != magic[3] {
		return containerHeader{}, nil, ErrInvalidMagic
	}
	if data[4] != containerVersion {
		return containerHeader{}, nil, ErrUnsupportedVersion
	}
	mode := data[5]
	extraLen := binary.BigEndian.Uint32(data[8:12])
	payloadLen := binary.BigEndian.Uint32(data[12:16])
	if int(extraLen)+int(payloadLen)+16 != len(data) {
		// allow trailing? no — exact
		if 16+int(extraLen)+int(payloadLen) > len(data) {
			return containerHeader{}, nil, ErrCorruptBundle
		}
	}
	if extraLen > uint32(maxPayloadBytes) || payloadLen > uint32(maxPayloadBytes) {
		return containerHeader{}, nil, ErrCorruptBundle
	}
	extra := data[16 : 16+extraLen]
	payload := data[16+extraLen : 16+int(extraLen)+int(payloadLen)]
	return containerHeader{mode: mode, extra: append([]byte(nil), extra...)}, append([]byte(nil), payload...), nil
}

func randomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	return b, err
}
