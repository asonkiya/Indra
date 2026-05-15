// Package crypto provides NaCl box and hybrid PQC encryption for Indra messages.
//
// Classical messages use Curve25519+XSalsa20+Poly1305 (nacl/box).
// Hybrid messages combine ML-KEM-768 with the classical DH key via HKDF-SHA256,
// then use XSalsa20-Poly1305 (nacl/secretbox) for symmetric encryption.
// Keys are derived from the Ed25519 libp2p identity via the identity package.
package crypto

import (
	"crypto/hkdf"
	"crypto/mlkem"
	"crypto/rand"
	"crypto/sha256"
	"fmt"

	"golang.org/x/crypto/nacl/box"
	"golang.org/x/crypto/nacl/secretbox"
)

// CryptoVersionNaCl indicates classical NaCl box encryption (default, backwards-compatible).
const CryptoVersionNaCl = int32(0)

// CryptoVersionHybrid indicates hybrid ML-KEM-768 + NaCl encryption.
const CryptoVersionHybrid = int32(2)

// PQCEncapsulationKeySize is the byte length of an ML-KEM-768 encapsulation key.
const PQCEncapsulationKeySize = mlkem.EncapsulationKeySize768

// EncryptFor encrypts plaintext from sender to recipient.
//
//   - nonce:        caller-supplied 24-byte nonce (use store.NextNonce for sends)
//   - recipPubKey:  recipient's Curve25519 public key
//   - senderPrivKey: sender's Curve25519 private key
//
// Returns the ciphertext (includes a 16-byte Poly1305 MAC at the end).
func EncryptFor(
	plaintext []byte,
	nonce *[24]byte,
	recipPubKey *[32]byte,
	senderPrivKey *[32]byte,
) []byte {
	return box.Seal(nil, plaintext, nonce, recipPubKey, senderPrivKey)
}

// DecryptFrom decrypts a received ciphertext.
//
//   - ciphertext:   the raw bytes from the Envelope
//   - nonce:        the 24-byte nonce from the Envelope
//   - senderPubKey: sender's Curve25519 public key (from Envelope.SenderPubkey)
//   - recipPrivKey: recipient's own Curve25519 private key
func DecryptFrom(
	ciphertext []byte,
	nonce *[24]byte,
	senderPubKey *[32]byte,
	recipPrivKey *[32]byte,
) ([]byte, error) {
	plaintext, ok := box.Open(nil, ciphertext, nonce, senderPubKey, recipPrivKey)
	if !ok {
		return nil, fmt.Errorf("decryption failed: bad ciphertext or wrong keys")
	}
	return plaintext, nil
}

// RandomNonce generates a cryptographically random 24-byte nonce.
// Use this for one-off encryptions (e.g., DHT mailbox entries).
// For conversation messages, use store.NextNonce instead to ensure uniqueness.
func RandomNonce() (*[24]byte, error) {
	var nonce [24]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}
	return &nonce, nil
}

// BytesToNonce converts a 24-byte slice to a *[24]byte pointer.
// Returns an error if the slice is not exactly 24 bytes.
func BytesToNonce(b []byte) (*[24]byte, error) {
	if len(b) != 24 {
		return nil, fmt.Errorf("nonce must be 24 bytes, got %d", len(b))
	}
	var nonce [24]byte
	copy(nonce[:], b)
	return &nonce, nil
}

// BytesToKey converts a 32-byte slice to a *[32]byte key pointer.
func BytesToKey(b []byte) (*[32]byte, error) {
	if len(b) != 32 {
		return nil, fmt.Errorf("key must be 32 bytes, got %d", len(b))
	}
	var k [32]byte
	copy(k[:], b)
	return &k, nil
}

// HybridEncryptFor encrypts plaintext using a hybrid ML-KEM-768 + NaCl construction.
//
// The shared key is derived as:
//
//	classical  = box.Precompute(recipBoxPubKey, senderPrivKey)  // NaCl DH
//	kemSecret  = mlkem768.Encapsulate(recipEncapKey)            // PQ KEM
//	symKey     = HKDF-SHA256(classical || kemSecret, info="indra-hybrid-v1")
//	ciphertext = secretbox.Seal(plaintext, nonce, symKey)
//
// Returns the encrypted ciphertext and the 1088-byte KEM ciphertext that must
// be placed in Envelope.PQCKEMCiphertext so the recipient can decapsulate.
func HybridEncryptFor(
	plaintext     []byte,
	nonce         *[24]byte,
	recipEncapKey []byte,      // 1184-byte ML-KEM-768 encapsulation key
	recipBoxPubKey *[32]byte,  // Curve25519 public key
	senderPrivKey  *[32]byte,  // Curve25519 private key
) (ciphertext []byte, kemCT []byte, err error) {
	// 1. KEM encapsulate — returns (sharedKey, ciphertext).
	ek, err := mlkem.NewEncapsulationKey768(recipEncapKey)
	if err != nil {
		return nil, nil, fmt.Errorf("parse KEM encapsulation key: %w", err)
	}
	kemShared, kemCT := ek.Encapsulate()

	// 2. Classical DH via NaCl box.Precompute.
	var classicalShared [32]byte
	box.Precompute(&classicalShared, recipBoxPubKey, senderPrivKey)

	// 3. HKDF-SHA256 over (classicalShared || kemShared).
	ikm := append(classicalShared[:], kemShared...)
	symKeySlice, err := hkdf.Key(sha256.New, ikm, nil, "indra-hybrid-v1", 32)
	if err != nil {
		return nil, nil, fmt.Errorf("HKDF: %w", err)
	}
	var symKey [32]byte
	copy(symKey[:], symKeySlice)

	// 4. XSalsa20-Poly1305 via nacl/secretbox (symmetric key, not DH).
	ciphertext = secretbox.Seal(nil, plaintext, nonce, &symKey)
	return ciphertext, kemCT, nil
}

// HybridDecryptFrom decrypts a hybrid-mode envelope.
// kemCT is the 1088-byte ML-KEM ciphertext from Envelope.PQCKEMCiphertext.
func HybridDecryptFrom(
	ciphertext      []byte,
	nonce           *[24]byte,
	kemCT           []byte,
	senderBoxPubKey *[32]byte,
	recipPrivKey    *[32]byte,
	recipDecapKey   *mlkem.DecapsulationKey768,
) ([]byte, error) {
	// 1. KEM decapsulate.
	kemShared, err := recipDecapKey.Decapsulate(kemCT)
	if err != nil {
		return nil, fmt.Errorf("KEM decapsulate: %w", err)
	}

	// 2. Classical DH.
	var classicalShared [32]byte
	box.Precompute(&classicalShared, senderBoxPubKey, recipPrivKey)

	// 3. HKDF-SHA256 (same derivation as encrypt).
	ikm := append(classicalShared[:], kemShared...)
	symKeySlice, err := hkdf.Key(sha256.New, ikm, nil, "indra-hybrid-v1", 32)
	if err != nil {
		return nil, fmt.Errorf("HKDF: %w", err)
	}
	var symKey [32]byte
	copy(symKey[:], symKeySlice)

	// 4. Decrypt.
	plaintext, ok := secretbox.Open(nil, ciphertext, nonce, &symKey)
	if !ok {
		return nil, fmt.Errorf("hybrid decryption failed: bad ciphertext or wrong keys")
	}
	return plaintext, nil
}
