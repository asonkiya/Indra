package store_test

import (
	"testing"

	"go.uber.org/zap"

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

func TestIdentityKeyRoundTrip(t *testing.T) {
	st := newTestStore(t)

	key := []byte("fake-ed25519-key-bytes-32bytesXX")
	if err := st.SaveIdentityPrivKey(key); err != nil {
		t.Fatal(err)
	}
	got, err := st.LoadIdentityPrivKey()
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(key) {
		t.Fatalf("want %q, got %q", key, got)
	}
}

func TestBoxPubKeyRoundTrip(t *testing.T) {
	st := newTestStore(t)

	var k [32]byte
	copy(k[:], "curve25519-pub-key-32-bytes-here")
	if err := st.SaveBoxPubKey(k); err != nil {
		t.Fatal(err)
	}
	got, err := st.LoadBoxPubKey()
	if err != nil {
		t.Fatal(err)
	}
	if got != k {
		t.Fatalf("pubkey mismatch")
	}
}

func TestNonceMonotonicity(t *testing.T) {
	st := newTestStore(t)

	prev := [24]byte{}
	for i := 0; i < 5; i++ {
		nonce, err := st.NextNonce("conv:abc")
		if err != nil {
			t.Fatal(err)
		}
		if nonce == prev && i > 0 {
			t.Fatal("nonce did not increment")
		}
		prev = nonce
	}
}

func TestMarkDelivered(t *testing.T) {
	st := newTestStore(t)

	msgID := "test-msg-123"
	delivered, err := st.IsDelivered(msgID)
	if err != nil {
		t.Fatal(err)
	}
	if delivered {
		t.Fatal("should not be delivered yet")
	}

	if err := st.MarkDelivered(msgID); err != nil {
		t.Fatal(err)
	}

	delivered, err = st.IsDelivered(msgID)
	if err != nil {
		t.Fatal(err)
	}
	if !delivered {
		t.Fatal("should be delivered now")
	}
}
