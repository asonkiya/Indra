// Package mailbox implements store-and-forward offline delivery via the DHT.
//
// Offline messages are stored under:
//   /indra/inbox/<recipientPeerID>/<seq>
//
// Values are already-encrypted Envelope bytes (JSON). DHT nodes cannot read
// the content because it is encrypted with the recipient's Curve25519 key.
package mailbox

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/peer"
	"go.uber.org/zap"

	indracrypto "github.com/aryaman/indra/internal/crypto"
	"github.com/aryaman/indra/internal/identity"
	"github.com/aryaman/indra/internal/protocol/pb"
	"github.com/aryaman/indra/internal/store"
)

const (
	dhtKeyPrefix    = "/indra/inbox/"
	pollInterval    = 5 * time.Minute
	maxMailboxItems = 100
)

// Mailbox handles offline message delivery via the DHT.
type Mailbox struct {
	dht     *dht.IpfsDHT
	store   *store.Store
	myID    peer.ID
	log     *zap.Logger
	seq     atomic.Int64
	inbound chan<- pb.Envelope
}

// New creates a Mailbox and starts background polling.
func New(ctx context.Context, kadDHT *dht.IpfsDHT, st *store.Store, myID peer.ID, log *zap.Logger, inbound chan<- pb.Envelope) *Mailbox {
	m := &Mailbox{
		dht:     kadDHT,
		store:   st,
		myID:    myID,
		log:     log,
		inbound: inbound,
	}
	go m.pollLoop(ctx)
	return m
}

// PutOfflinePlaintext encrypts plaintext and stores it in the DHT mailbox.
// recipPQCKey is the recipient's ML-KEM-768 encapsulation key; pass nil for NaCl-only fallback.
func (m *Mailbox) PutOfflinePlaintext(ctx context.Context, recipientID peer.ID, plaintext []byte, recipBoxPub [32]byte, recipPQCKey []byte, id *identity.Identity) error {
	nonce, err := indracrypto.RandomNonce()
	if err != nil {
		return fmt.Errorf("generate nonce: %w", err)
	}

	var ciphertext, kemCT []byte
	var cryptoVersion int32

	if len(recipPQCKey) == indracrypto.PQCEncapsulationKeySize {
		ct, kct, err := indracrypto.HybridEncryptFor(plaintext, nonce, recipPQCKey, &recipBoxPub, &id.BoxPrivKey)
		if err != nil {
			return fmt.Errorf("hybrid encrypt: %w", err)
		}
		ciphertext = ct
		kemCT = kct
		cryptoVersion = indracrypto.CryptoVersionHybrid
	} else {
		ciphertext = indracrypto.EncryptFor(plaintext, nonce, &recipBoxPub, &id.BoxPrivKey)
		cryptoVersion = indracrypto.CryptoVersionNaCl
	}

	msgID := uuid.New().String()
	sig, err := id.PrivKey.Sign(append(append([]byte(msgID), ciphertext...), nonce[:]...))
	if err != nil {
		return fmt.Errorf("sign: %w", err)
	}
	env := &pb.Envelope{
		MessageId:        msgID,
		SenderId:         id.PeerID.String(),
		RecipientId:      recipientID.String(),
		Ciphertext:       ciphertext,
		Nonce:            nonce[:],
		SenderPubkey:     id.BoxPubKey[:],
		SentAtUnix:       time.Now().Unix(),
		Type:             pb.EnvelopeType_OFFLINE,
		Signature:        sig,
		PQCKEMCiphertext: kemCT,
		CryptoVersion:    cryptoVersion,
	}
	return m.PutOffline(ctx, recipientID, env)
}

// PutOffline stores an envelope in the DHT for a recipient who is offline.
func (m *Mailbox) PutOffline(ctx context.Context, recipientID peer.ID, env *pb.Envelope) error {
	seq := m.seq.Add(1)
	env.Seq = seq

	data, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("marshal envelope: %w", err)
	}

	key := fmt.Sprintf("%s%s/%d", dhtKeyPrefix, recipientID.String(), seq)
	if err := m.dht.PutValue(ctx, key, data); err != nil {
		return fmt.Errorf("DHT put: %w", err)
	}
	m.log.Debug("stored offline message", zap.String("key", key))
	return nil
}

// FetchInbox retrieves all pending envelopes from the DHT for this node.
func (m *Mailbox) FetchInbox(ctx context.Context) {
	for i := int64(1); i <= maxMailboxItems; i++ {
		key := fmt.Sprintf("%s%s/%d", dhtKeyPrefix, m.myID.String(), i)
		data, err := m.dht.GetValue(ctx, key)
		if err != nil {
			// No more items at this sequence number.
			break
		}

		var env pb.Envelope
		if err := json.Unmarshal(data, &env); err != nil {
			m.log.Warn("corrupt mailbox entry", zap.String("key", key), zap.Error(err))
			continue
		}

		// Dedup.
		if delivered, _ := m.store.IsDelivered(env.MessageId); delivered {
			continue
		}

		select {
		case m.inbound <- env:
		case <-ctx.Done():
			return
		default:
			m.log.Warn("mailbox inbound channel full")
		}
	}
}

func (m *Mailbox) pollLoop(ctx context.Context) {
	// Fetch immediately on startup.
	m.FetchInbox(ctx)

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.FetchInbox(ctx)
		}
	}
}
