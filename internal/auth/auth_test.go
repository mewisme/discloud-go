package auth

import (
	"strings"
	"testing"
)

func TestHashVerifyPassword(t *testing.T) {
	hash, err := HashPassword("secret-password")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword(hash, "secret-password") {
		t.Fatal("expected password to verify")
	}
	if VerifyPassword(hash, "wrong") {
		t.Fatal("wrong password should not verify")
	}
}

func TestValidUsername(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"ab", false},
		{"abc", true},
		{"user_name-1", true},
		{"_bad", false},
		{"-bad", false},
		{"Bad", false}, // not normalized
		{NormalizeUsername("  Alice_1  "), true},
		{strings.Repeat("a", 33), false},
	}
	for _, tc := range cases {
		if got := ValidUsername(tc.in); got != tc.want {
			t.Errorf("ValidUsername(%q)=%v want %v", tc.in, got, tc.want)
		}
	}
}

func TestDeriveKeysDomainSeparated(t *testing.T) {
	k := DeriveKeys("a-secret-that-is-long-enough-123456")
	if string(k.Session) == string(k.File) || string(k.Session) == string(k.CSRF) {
		t.Fatal("HMAC keys must be domain-separated")
	}
	a := k.HashSessionToken("tok")
	b := k.HashSessionToken("tok")
	if a != b {
		t.Fatal("hash must be stable")
	}
	if k.HashFileToken("tok") == a {
		t.Fatal("session and file hashes must differ")
	}
	up := k.HashUploadToken("tok")
	if up == a || up == k.HashFileToken("tok") {
		t.Fatal("upload hash must differ from session and file")
	}
	if !k.UploadTokenMatch("tok", up) {
		t.Fatal("UploadTokenMatch should accept matching token")
	}
	if k.UploadTokenMatch("other", up) {
		t.Fatal("UploadTokenMatch should reject wrong token")
	}
	sig := k.SignFileUnlock("abc123", 9999999999)
	if !k.FileUnlockMatch("abc123", sig, 1000) {
		t.Fatal("FileUnlockMatch should accept valid unlock")
	}
	if k.FileUnlockMatch("abc123", sig, 99999999999) {
		t.Fatal("FileUnlockMatch should reject expired unlock")
	}
	if k.FileUnlockMatch("other", sig, 1000) {
		t.Fatal("FileUnlockMatch should reject wrong file id")
	}
}
