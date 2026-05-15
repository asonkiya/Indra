package crypto_test

import (
	"crypto/mlkem"
	"crypto/rand"
	"testing"

	"golang.org/x/crypto/nacl/box"

	indracrypto "github.com/aryaman/indra/internal/crypto"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	// Generate two random Curve25519 key pairs.
	alicePub, alicePriv, err := box.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	bobPub, bobPriv, err := box.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	plaintext := []byte("hello from alice to bob")

	nonce, err := indracrypto.RandomNonce()
	if err != nil {
		t.Fatal(err)
	}

	ciphertext := indracrypto.EncryptFor(plaintext, nonce, bobPub, alicePriv)

	got, err := indracrypto.DecryptFrom(ciphertext, nonce, alicePub, bobPriv)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if string(got) != string(plaintext) {
		t.Fatalf("want %q, got %q", plaintext, got)
	}
}

func TestDecryptWrongKey(t *testing.T) {
	alicePub, alicePriv, _ := box.GenerateKey(rand.Reader)
	bobPub, _, _ := box.GenerateKey(rand.Reader)
	_, evePriv, _ := box.GenerateKey(rand.Reader) // wrong key

	nonce, _ := indracrypto.RandomNonce()
	ciphertext := indracrypto.EncryptFor([]byte("secret"), nonce, bobPub, alicePriv)

	_, err := indracrypto.DecryptFrom(ciphertext, nonce, alicePub, evePriv)
	if err == nil {
		t.Fatal("expected decryption to fail with wrong key")
	}
}

func TestBytesToNonce(t *testing.T) {
	_, err := indracrypto.BytesToNonce(make([]byte, 23))
	if err == nil {
		t.Fatal("expected error for wrong length")
	}
	_, err = indracrypto.BytesToNonce(make([]byte, 24))
	if err != nil {
		t.Fatal(err)
	}
}

func TestBytesToKey(t *testing.T) {
	_, err := indracrypto.BytesToKey(make([]byte, 31))
	if err == nil {
		t.Fatal("expected error for wrong length")
	}
	_, err = indracrypto.BytesToKey(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
}

func TestHybridEncryptDecryptRoundTrip(t *testing.T) {
	// Generate classical Curve25519 key pairs.
	alicePub, alicePriv, err := box.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	bobPub, bobPriv, err := box.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	// Generate ML-KEM-768 key pair for Bob (recipient).
	bobDecap, err := mlkem.GenerateKey768()
	if err != nil {
		t.Fatal(err)
	}
	bobEncapKey := bobDecap.EncapsulationKey().Bytes()

	plaintext := []byte("quantum-safe hello from alice to bob")

	nonce, err := indracrypto.RandomNonce()
	if err != nil {
		t.Fatal(err)
	}

	ciphertext, kemCT, err := indracrypto.HybridEncryptFor(plaintext, nonce, bobEncapKey, bobPub, alicePriv)
	if err != nil {
		t.Fatalf("HybridEncryptFor: %v", err)
	}

	got, err := indracrypto.HybridDecryptFrom(ciphertext, nonce, kemCT, alicePub, bobPriv, bobDecap)
	if err != nil {
		t.Fatalf("HybridDecryptFrom: %v", err)
	}
	if string(got) != string(plaintext) {
		t.Fatalf("want %q, got %q", plaintext, got)
	}
}

func TestHybridDecryptWrongKEMKey(t *testing.T) {
	alicePub, alicePriv, _ := box.GenerateKey(rand.Reader)
	bobPub, bobPriv, _ := box.GenerateKey(rand.Reader)

	bobDecap, _ := mlkem.GenerateKey768()
	bobEncapKey := bobDecap.EncapsulationKey().Bytes()

	eveDecap, _ := mlkem.GenerateKey768() // wrong KEM key

	nonce, _ := indracrypto.RandomNonce()
	ciphertext, kemCT, err := indracrypto.HybridEncryptFor([]byte("secret"), nonce, bobEncapKey, bobPub, alicePriv)
	if err != nil {
		t.Fatal(err)
	}

	_, err = indracrypto.HybridDecryptFrom(ciphertext, nonce, kemCT, alicePub, bobPriv, eveDecap)
	if err == nil {
		t.Fatal("expected hybrid decryption to fail with wrong KEM key")
	}
}

func TestHybridDecryptWrongNaClKey(t *testing.T) {
	alicePub, alicePriv, _ := box.GenerateKey(rand.Reader)
	bobPub, _, _ := box.GenerateKey(rand.Reader)
	_, evePriv, _ := box.GenerateKey(rand.Reader) // wrong Curve25519 key

	bobDecap, _ := mlkem.GenerateKey768()
	bobEncapKey := bobDecap.EncapsulationKey().Bytes()

	nonce, _ := indracrypto.RandomNonce()
	ciphertext, kemCT, err := indracrypto.HybridEncryptFor([]byte("secret"), nonce, bobEncapKey, bobPub, alicePriv)
	if err != nil {
		t.Fatal(err)
	}

	// Use wrong Curve25519 private key — decryption should fail.
	_, err = indracrypto.HybridDecryptFrom(ciphertext, nonce, kemCT, alicePub, evePriv, bobDecap)
	if err == nil {
		t.Fatal("expected hybrid decryption to fail with wrong NaCl key")
	}
}
