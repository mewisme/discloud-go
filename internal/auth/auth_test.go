package auth

import "testing"

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
}
