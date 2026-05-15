package store

import (
	"encoding/binary"
	"fmt"

	badger "github.com/dgraph-io/badger/v4"
)

// Key layout:
//   identity:privkey            -> marshaled Ed25519 private key bytes
//   identity:boxpubkey          -> Curve25519 public key (32 bytes)
//   nonce:<convID>              -> uint64 little-endian counter
//   delivered:<msgID>           -> tombstone (empty value)

const (
	keyIdentityPriv    = "identity:privkey"
	keyIdentityBoxPub  = "identity:boxpubkey"
	keyPQCDecapSeed    = "identity:pqc_decap_seed" // 64-byte ML-KEM-768 seed
	keyNoncePrefix     = "nonce:"
	keyDeliveredPrefix = "delivered:"
)

// SaveIdentityPrivKey persists the raw Ed25519 private key bytes.
func (s *Store) SaveIdentityPrivKey(raw []byte) error {
	return s.set([]byte(keyIdentityPriv), raw)
}

// LoadIdentityPrivKey retrieves the raw Ed25519 private key bytes.
// Returns badger.ErrKeyNotFound if no identity has been generated yet.
func (s *Store) LoadIdentityPrivKey() ([]byte, error) {
	return s.get([]byte(keyIdentityPriv))
}

// SaveBoxPubKey persists the Curve25519 public key derived from the Ed25519 identity.
func (s *Store) SaveBoxPubKey(key [32]byte) error {
	return s.set([]byte(keyIdentityBoxPub), key[:])
}

// LoadBoxPubKey retrieves the Curve25519 public key.
func (s *Store) LoadBoxPubKey() ([32]byte, error) {
	raw, err := s.get([]byte(keyIdentityBoxPub))
	if err != nil {
		return [32]byte{}, err
	}
	if len(raw) != 32 {
		return [32]byte{}, fmt.Errorf("invalid box pubkey length: %d", len(raw))
	}
	var k [32]byte
	copy(k[:], raw)
	return k, nil
}

// SavePQCDecapSeed persists the 64-byte ML-KEM-768 decapsulation key seed.
func (s *Store) SavePQCDecapSeed(seed []byte) error {
	return s.set([]byte(keyPQCDecapSeed), seed)
}

// LoadPQCDecapSeed retrieves the 64-byte ML-KEM-768 decapsulation key seed.
// Returns badger.ErrKeyNotFound if no PQC key has been generated yet.
func (s *Store) LoadPQCDecapSeed() ([]byte, error) {
	return s.get([]byte(keyPQCDecapSeed))
}

// NextNonce atomically reads the nonce counter for a conversation,
// increments it, and returns a 24-byte nonce (counter in bytes 0-7, rest zero).
func (s *Store) NextNonce(conversationID string) ([24]byte, error) {
	key := []byte(keyNoncePrefix + conversationID)
	var nonce [24]byte

	err := s.db.Update(func(txn *badger.Txn) error {
		var counter uint64
		item, err := txn.Get(key)
		if err == nil {
			err = item.Value(func(v []byte) error {
				if len(v) >= 8 {
					counter = binary.LittleEndian.Uint64(v)
				}
				return nil
			})
			if err != nil {
				return err
			}
		} else if err != badger.ErrKeyNotFound {
			return err
		}

		// Encode the current counter into the nonce before incrementing.
		binary.LittleEndian.PutUint64(nonce[:8], counter)

		// Store counter+1.
		var buf [8]byte
		binary.LittleEndian.PutUint64(buf[:], counter+1)
		return txn.Set(key, buf[:])
	})
	return nonce, err
}

// MarkDelivered records a tombstone so that a message is not processed twice.
func (s *Store) MarkDelivered(msgID string) error {
	return s.set([]byte(keyDeliveredPrefix+msgID), []byte{1})
}

// IsDelivered returns true if the message has already been processed.
func (s *Store) IsDelivered(msgID string) (bool, error) {
	return s.exists([]byte(keyDeliveredPrefix + msgID))
}
