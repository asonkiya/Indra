package identity_test

import (
	"testing"

	"go.uber.org/zap"

	"github.com/aryaman/indra/internal/identity"
	"github.com/aryaman/indra/internal/store"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(t.TempDir(), zap.NewNop())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestGenerateAndReload(t *testing.T) {
	st := newTestStore(t)

	id1, err := identity.Load(st)
	if err != nil {
		t.Fatalf("first load: %v", err)
	}

	// Reload from the same store — must produce the same PeerID.
	id2, err := identity.Load(st)
	if err != nil {
		t.Fatalf("second load: %v", err)
	}

	if id1.PeerID != id2.PeerID {
		t.Fatalf("PeerID changed across reload: %s vs %s", id1.PeerID, id2.PeerID)
	}
	if id1.BoxPubKey != id2.BoxPubKey {
		t.Fatal("Curve25519 pubkey changed across reload")
	}
}

func TestCurve25519KeysAreValid(t *testing.T) {
	st := newTestStore(t)
	id, err := identity.Load(st)
	if err != nil {
		t.Fatal(err)
	}

	// BoxPrivKey and BoxPubKey must be non-zero.
	var zero [32]byte
	if id.BoxPrivKey == zero {
		t.Fatal("BoxPrivKey is all zeros")
	}
	if id.BoxPubKey == zero {
		t.Fatal("BoxPubKey is all zeros")
	}
	// They must be different from each other.
	if id.BoxPrivKey == id.BoxPubKey {
		t.Fatal("BoxPrivKey == BoxPubKey (should never happen)")
	}
}

func TestTwoIdentitiesHaveDifferentKeys(t *testing.T) {
	st1 := newTestStore(t)
	st2 := newTestStore(t)

	id1, _ := identity.Load(st1)
	id2, _ := identity.Load(st2)

	if id1.PeerID == id2.PeerID {
		t.Fatal("two fresh identities produced the same PeerID")
	}
	if id1.BoxPubKey == id2.BoxPubKey {
		t.Fatal("two fresh identities produced the same Curve25519 pubkey")
	}
}
