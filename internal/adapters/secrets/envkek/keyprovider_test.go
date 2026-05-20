package envkek

import (
	"bytes"
	"context"
	"testing"
)

func TestKeyProviderRoundTrip(t *testing.T) {
	t.Parallel()
	kp := newTestKeyProvider(t)
	dek := bytes.Repeat([]byte{7}, keySize)
	aad := "ssh_private_key:lab-1"

	ciphertext, nonce, version, err := kp.EncryptDEK(context.Background(), dek, aad)
	if err != nil {
		t.Fatalf("EncryptDEK: %v", err)
	}
	got, err := kp.DecryptDEK(context.Background(), ciphertext, nonce, version, aad)
	if err != nil {
		t.Fatalf("DecryptDEK: %v", err)
	}
	if !bytes.Equal(got, dek) {
		t.Fatalf("DEK mismatch: got %x want %x", got, dek)
	}
}

func TestKeyProviderRejectsTamperedCiphertext(t *testing.T) {
	t.Parallel()
	kp := newTestKeyProvider(t)
	dek := bytes.Repeat([]byte{7}, keySize)
	ciphertext, nonce, version, err := kp.EncryptDEK(context.Background(), dek, "aad")
	if err != nil {
		t.Fatalf("EncryptDEK: %v", err)
	}
	ciphertext[0] ^= 0x01

	if _, err := kp.DecryptDEK(context.Background(), ciphertext, nonce, version, "aad"); err == nil {
		t.Fatal("expected decrypt error for tampered ciphertext")
	}
}

func TestKeyProviderRejectsWrongAAD(t *testing.T) {
	t.Parallel()
	kp := newTestKeyProvider(t)
	dek := bytes.Repeat([]byte{7}, keySize)
	ciphertext, nonce, version, err := kp.EncryptDEK(context.Background(), dek, "aad")
	if err != nil {
		t.Fatalf("EncryptDEK: %v", err)
	}

	if _, err := kp.DecryptDEK(context.Background(), ciphertext, nonce, version, "wrong"); err == nil {
		t.Fatal("expected decrypt error for wrong aad")
	}
}

func TestKeyProviderRejectsUnsupportedVersion(t *testing.T) {
	t.Parallel()
	kp := newTestKeyProvider(t)
	dek := bytes.Repeat([]byte{7}, keySize)
	ciphertext, nonce, _, err := kp.EncryptDEK(context.Background(), dek, "aad")
	if err != nil {
		t.Fatalf("EncryptDEK: %v", err)
	}

	if _, err := kp.DecryptDEK(context.Background(), ciphertext, nonce, 2, "aad"); err == nil {
		t.Fatal("expected decrypt error for unsupported version")
	}
}

func TestNewKeyProviderFromKeyRejectsWrongSize(t *testing.T) {
	t.Parallel()
	if _, err := NewKeyProviderFromKey([]byte("short")); err == nil {
		t.Fatal("expected key-size error")
	}
}

func newTestKeyProvider(t *testing.T) *KeyProvider {
	t.Helper()
	kp, err := NewKeyProviderFromKey(bytes.Repeat([]byte{1}, keySize))
	if err != nil {
		t.Fatalf("NewKeyProviderFromKey: %v", err)
	}
	return kp
}
