package identity

import (
	"crypto/ed25519"
	"crypto/mlkem"
	"crypto/rand"
	"crypto/sha512"
	"fmt"

	"filippo.io/edwards25519"
	badger "github.com/dgraph-io/badger/v4"
	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/aryaman/indra/internal/store"
)

// Identity holds the node's cryptographic identity.
type Identity struct {
	PrivKey     libp2pcrypto.PrivKey // Ed25519 private key used by libp2p
	PubKey      libp2pcrypto.PubKey
	PeerID      peer.ID
	BoxPrivKey  [32]byte                    // Curve25519 private key for NaCl box encryption
	BoxPubKey   [32]byte                    // Curve25519 public key (share this with peers)
	PQCDecapKey *mlkem.DecapsulationKey768  // ML-KEM-768 decapsulation key; never nil after Load()
}

// Load loads an existing identity from the store, or generates a new one.
func Load(s *store.Store) (*Identity, error) {
	raw, err := s.LoadIdentityPrivKey()
	if err == badger.ErrKeyNotFound {
		return generate(s)
	}
	if err != nil {
		return nil, fmt.Errorf("load identity key: %w", err)
	}
	id, err := fromRaw(raw)
	if err != nil {
		return nil, err
	}
	if err := ensurePQCKey(id, s); err != nil {
		return nil, err
	}
	return id, nil
}

func generate(s *store.Store) (*Identity, error) {
	privKey, _, err := libp2pcrypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate Ed25519 key: %w", err)
	}
	raw, err := libp2pcrypto.MarshalPrivateKey(privKey)
	if err != nil {
		return nil, fmt.Errorf("marshal Ed25519 key: %w", err)
	}
	if err := s.SaveIdentityPrivKey(raw); err != nil {
		return nil, fmt.Errorf("save identity key: %w", err)
	}
	id, err := fromRaw(raw)
	if err != nil {
		return nil, err
	}
	if err := s.SaveBoxPubKey(id.BoxPubKey); err != nil {
		return nil, fmt.Errorf("save box pubkey: %w", err)
	}
	if err := ensurePQCKey(id, s); err != nil {
		return nil, err
	}
	return id, nil
}

func fromRaw(raw []byte) (*Identity, error) {
	privKey, err := libp2pcrypto.UnmarshalPrivateKey(raw)
	if err != nil {
		return nil, fmt.Errorf("unmarshal private key: %w", err)
	}
	pubKey := privKey.GetPublic()
	peerID, err := peer.IDFromPublicKey(pubKey)
	if err != nil {
		return nil, fmt.Errorf("derive peer ID: %w", err)
	}

	boxPriv, boxPub, err := ed25519ToCurve25519(privKey)
	if err != nil {
		return nil, fmt.Errorf("convert to Curve25519: %w", err)
	}

	return &Identity{
		PrivKey:    privKey,
		PubKey:     pubKey,
		PeerID:     peerID,
		BoxPrivKey: boxPriv,
		BoxPubKey:  boxPub,
	}, nil
}

// ensurePQCKey loads the ML-KEM-768 decapsulation key from the store, generating
// and persisting a new one if it doesn't exist yet (first run or upgrade).
func ensurePQCKey(id *Identity, s *store.Store) error {
	seed, err := s.LoadPQCDecapSeed()
	if err == badger.ErrKeyNotFound {
		dk, err := mlkem.GenerateKey768()
		if err != nil {
			return fmt.Errorf("generate ML-KEM key: %w", err)
		}
		if err := s.SavePQCDecapSeed(dk.Bytes()); err != nil {
			return fmt.Errorf("save ML-KEM seed: %w", err)
		}
		id.PQCDecapKey = dk
		return nil
	}
	if err != nil {
		return fmt.Errorf("load ML-KEM seed: %w", err)
	}
	dk, err := mlkem.NewDecapsulationKey768(seed)
	if err != nil {
		return fmt.Errorf("reconstruct ML-KEM key: %w", err)
	}
	id.PQCDecapKey = dk
	return nil
}

// ed25519ToCurve25519 converts an Ed25519 key pair to Curve25519 for NaCl box.
//
// Private key: SHA-512(seed)[0:32] with standard X25519 clamping.
// Public key:  birational map from Edwards to Montgomery coordinates.
func ed25519ToCurve25519(priv libp2pcrypto.PrivKey) (privOut, pubOut [32]byte, err error) {
	rawBytes, err := priv.Raw()
	if err != nil {
		return privOut, pubOut, fmt.Errorf("get raw key: %w", err)
	}

	var seed []byte
	switch len(rawBytes) {
	case ed25519.SeedSize: // 32
		seed = rawBytes
	case ed25519.PrivateKeySize: // 64: seed || public_key
		seed = rawBytes[:ed25519.SeedSize]
	default:
		return privOut, pubOut, fmt.Errorf("unexpected Ed25519 key length: %d", len(rawBytes))
	}

	edPriv := ed25519.NewKeyFromSeed(seed)

	// Derive Curve25519 private key from SHA-512(seed).
	digest := sha512.Sum512(seed)
	clamp := make([]byte, 32)
	copy(clamp, digest[:32])
	clamp[0] &= 248
	clamp[31] &= 127
	clamp[31] |= 64
	copy(privOut[:], clamp)

	// Derive Curve25519 public key via Edwards→Montgomery birational map.
	edPub := edPriv.Public().(ed25519.PublicKey)
	ep, err := new(edwards25519.Point).SetBytes(edPub)
	if err != nil {
		return privOut, pubOut, fmt.Errorf("parse Ed25519 public key: %w", err)
	}
	copy(pubOut[:], ep.BytesMontgomery())

	return privOut, pubOut, nil
}
