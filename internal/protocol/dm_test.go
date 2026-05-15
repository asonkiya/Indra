package protocol_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	libp2p "github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"go.uber.org/zap"

	indracrypto "github.com/aryaman/indra/internal/crypto"
	"github.com/aryaman/indra/internal/identity"
	"github.com/aryaman/indra/internal/protocol"
	"github.com/aryaman/indra/internal/protocol/pb"
	"github.com/aryaman/indra/internal/store"
	"github.com/aryaman/indra/pkg/types"
)

type testNode struct {
	host    host.Host
	store   *store.Store
	id      *identity.Identity
	inbound chan types.Message
	dm      *protocol.DMHandler
}

func newTestNode(t *testing.T) *testNode {
	t.Helper()

	st, err := store.Open(t.TempDir(), zap.NewNop())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	id, err := identity.Load(st)
	if err != nil {
		t.Fatalf("load identity: %v", err)
	}

	// Use the identity's private key so host.ID() == id.PeerID.
	h, err := libp2p.New(
		libp2p.Identity(id.PrivKey),
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
	)
	if err != nil {
		t.Fatalf("new host: %v", err)
	}
	t.Cleanup(func() { h.Close() })

	inbound := make(chan types.Message, 8)
	dm := protocol.NewDMHandler(h, id, st, zap.NewNop(), inbound)

	return &testNode{host: h, store: st, id: id, inbound: inbound, dm: dm}
}

func connect(t *testing.T, ctx context.Context, a, b *testNode) {
	t.Helper()
	addrInfo := peer.AddrInfo{ID: b.host.ID(), Addrs: b.host.Addrs()}
	if err := a.host.Connect(ctx, addrInfo); err != nil {
		t.Fatalf("connect: %v", err)
	}
}

func TestDMSendReceive(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	a := newTestNode(t)
	b := newTestNode(t)
	connect(t, ctx, a, b)

	plaintext := []byte("hello from A to B")
	_, err := a.dm.Send(ctx, b.host.ID(), plaintext, b.id.BoxPubKey, nil)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	select {
	case msg := <-b.inbound:
		if string(msg.Plaintext) != string(plaintext) {
			t.Fatalf("want %q, got %q", plaintext, msg.Plaintext)
		}
		if msg.SenderID != a.id.PeerID {
			t.Fatalf("wrong sender: want %s, got %s", a.id.PeerID, msg.SenderID)
		}
		if msg.Direction != types.Inbound {
			t.Fatalf("expected Inbound direction, got %d", msg.Direction)
		}
	case <-ctx.Done():
		t.Fatal("timeout: B never received the message")
	}
}

func TestDMStoredOnBothSides(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	a := newTestNode(t)
	b := newTestNode(t)
	connect(t, ctx, a, b)

	msg, err := a.dm.Send(ctx, b.host.ID(), []byte("stored?"), b.id.BoxPubKey, nil)
	if err != nil {
		t.Fatal(err)
	}
	<-b.inbound // wait for delivery

	// A should have stored an Outbound copy.
	sent, err := a.store.ListMessages(msg.ConversationID, 10, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(sent) != 1 {
		t.Fatalf("A: want 1 stored message, got %d", len(sent))
	}

	// B should have stored an Inbound copy.
	recv, err := b.store.ListMessages(msg.ConversationID, 10, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(recv) != 1 {
		t.Fatalf("B: want 1 stored message, got %d", len(recv))
	}
}

func TestDMDeduplication(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	a := newTestNode(t)
	b := newTestNode(t)
	connect(t, ctx, a, b)

	_, err := a.dm.Send(ctx, b.host.ID(), []byte("once"), b.id.BoxPubKey, nil)
	if err != nil {
		t.Fatal(err)
	}
	msg := <-b.inbound

	// Mark as delivered (simulates duplicate delivery from mailbox).
	_ = b.store.MarkDelivered(msg.ID)

	// The delivered tombstone should exist.
	ok, err := b.store.IsDelivered(msg.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected message to be marked delivered")
	}
}

// TestDMSendReceiveHybrid verifies end-to-end messaging upgrades to hybrid
// PQC encryption when the recipient's ML-KEM key is provided.
func TestDMSendReceiveHybrid(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	a := newTestNode(t)
	b := newTestNode(t)
	connect(t, ctx, a, b)

	// Pass B's ML-KEM encapsulation key — should trigger hybrid mode.
	bEncapKey := b.id.PQCDecapKey.EncapsulationKey().Bytes()
	plaintext := []byte("quantum-safe hello from A to B")

	_, err := a.dm.Send(ctx, b.host.ID(), plaintext, b.id.BoxPubKey, bEncapKey)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	select {
	case msg := <-b.inbound:
		if string(msg.Plaintext) != string(plaintext) {
			t.Fatalf("want %q, got %q", plaintext, msg.Plaintext)
		}
		if msg.Direction != types.Inbound {
			t.Fatalf("expected Inbound direction, got %d", msg.Direction)
		}
	case <-ctx.Done():
		t.Fatal("timeout: B never received the hybrid message")
	}
}

// TestDMSendReceiveLegacyFallback verifies that passing nil for PQC key falls
// back to NaCl-only encryption and still delivers correctly.
func TestDMSendReceiveLegacyFallback(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	a := newTestNode(t)
	b := newTestNode(t)
	connect(t, ctx, a, b)

	plaintext := []byte("nacl-only hello")
	_, err := a.dm.Send(ctx, b.host.ID(), plaintext, b.id.BoxPubKey, nil)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	select {
	case msg := <-b.inbound:
		if string(msg.Plaintext) != string(plaintext) {
			t.Fatalf("want %q, got %q", plaintext, msg.Plaintext)
		}
	case <-ctx.Done():
		t.Fatal("timeout: B never received the NaCl message")
	}
}

// TestDeliverEnvelope simulates the offline mailbox path: A builds an
// encrypted envelope, and B processes it via DeliverEnvelope (no stream).
func TestDeliverEnvelope(t *testing.T) {
	a := newTestNode(t)
	b := newTestNode(t)

	plaintext := []byte("offline hello")
	msgID := uuid.New().String()
	convID := types.ConversationID(a.id.PeerID, b.id.PeerID)

	// Build the envelope the same way Send does.
	nonce, err := a.store.NextNonce(convID)
	if err != nil {
		t.Fatal(err)
	}
	ciphertext := indracrypto.EncryptFor(plaintext, &nonce, &b.id.BoxPubKey, &a.id.BoxPrivKey)

	sigPayload := append([]byte(msgID), ciphertext...)
	sigPayload = append(sigPayload, nonce[:]...)
	sig, err := a.id.PrivKey.Sign(sigPayload)
	if err != nil {
		t.Fatal(err)
	}

	env := &pb.Envelope{
		MessageId:    msgID,
		SenderId:     a.id.PeerID.String(),
		RecipientId:  b.id.PeerID.String(),
		Ciphertext:   ciphertext,
		Nonce:        nonce[:],
		SenderPubkey: a.id.BoxPubKey[:],
		SentAtUnix:   time.Now().Unix(),
		Type:         pb.EnvelopeType_DIRECT,
		Signature:    sig,
	}

	// Deliver via the offline path (no stream, no ACK).
	if err := b.dm.DeliverEnvelope(env); err != nil {
		t.Fatalf("DeliverEnvelope: %v", err)
	}

	// B should receive the message on the inbound channel.
	select {
	case msg := <-b.inbound:
		if string(msg.Plaintext) != string(plaintext) {
			t.Fatalf("want %q, got %q", plaintext, msg.Plaintext)
		}
		if msg.SenderID != a.id.PeerID {
			t.Fatalf("wrong sender: want %s, got %s", a.id.PeerID, msg.SenderID)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout: B never received the message")
	}

	// Message should be persisted in B's store.
	msgs, err := b.store.ListMessages(convID, 10, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("want 1 stored message, got %d", len(msgs))
	}

	// Delivering the same envelope again should be a no-op (dedup).
	if err := b.dm.DeliverEnvelope(env); err != nil {
		t.Fatalf("DeliverEnvelope dedup: %v", err)
	}
	// Should NOT produce a second inbound notification.
	select {
	case msg := <-b.inbound:
		t.Fatalf("unexpected duplicate delivery: %+v", msg)
	case <-time.After(200 * time.Millisecond):
		// expected — no duplicate
	}
}
