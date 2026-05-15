// Package protocol implements Indra's libp2p stream protocols.
package protocol

import (
	"context"
	"crypto/mlkem"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"go.uber.org/zap"

	indracrypto "github.com/aryaman/indra/internal/crypto"
	"github.com/aryaman/indra/internal/identity"
	"github.com/aryaman/indra/internal/protocol/pb"
	"github.com/aryaman/indra/internal/store"
	"github.com/aryaman/indra/pkg/types"
)

const DMProtocol = "/indra/dm/1.0.0"

// DMHandler handles direct message sending and receiving.
type DMHandler struct {
	host    host.Host
	id      *identity.Identity
	store   *store.Store
	log     *zap.Logger
	inbound chan<- types.Message // notifies consumers of new messages
}

// NewDMHandler registers the DM stream handler on the host.
func NewDMHandler(h host.Host, id *identity.Identity, st *store.Store, log *zap.Logger, inbound chan<- types.Message) *DMHandler {
	dh := &DMHandler{
		host:    h,
		id:      id,
		store:   st,
		log:     log,
		inbound: inbound,
	}
	h.SetStreamHandler(DMProtocol, dh.handleStream)
	return dh
}

// Send encrypts and delivers a direct message to recipientID.
// recipPQCKey is the recipient's ML-KEM-768 encapsulation key (1184 bytes);
// pass nil or empty to fall back to NaCl-only encryption.
// Returns the saved Message on success.
func (dh *DMHandler) Send(ctx context.Context, recipientID peer.ID, plaintext []byte, recipBoxPubKey [32]byte, recipPQCKey []byte) (*types.Message, error) {
	msgID := uuid.New().String()
	convID := types.ConversationID(dh.id.PeerID, recipientID)

	nonce, err := dh.store.NextNonce(convID)
	if err != nil {
		return nil, fmt.Errorf("get nonce: %w", err)
	}

	var ciphertext, kemCT []byte
	var cryptoVersion int32

	if len(recipPQCKey) == indracrypto.PQCEncapsulationKeySize {
		ct, kct, err := indracrypto.HybridEncryptFor(plaintext, &nonce, recipPQCKey, &recipBoxPubKey, &dh.id.BoxPrivKey)
		if err != nil {
			return nil, fmt.Errorf("hybrid encrypt: %w", err)
		}
		ciphertext = ct
		kemCT = kct
		cryptoVersion = indracrypto.CryptoVersionHybrid
	} else {
		ciphertext = indracrypto.EncryptFor(plaintext, &nonce, &recipBoxPubKey, &dh.id.BoxPrivKey)
		cryptoVersion = indracrypto.CryptoVersionNaCl
	}

	// Sign (messageID || ciphertext || nonce) with Ed25519.
	sig, err := dh.sign(msgID, ciphertext, nonce[:])
	if err != nil {
		return nil, fmt.Errorf("sign envelope: %w", err)
	}

	env := &pb.Envelope{
		MessageId:        msgID,
		SenderId:         dh.id.PeerID.String(),
		RecipientId:      recipientID.String(),
		Ciphertext:       ciphertext,
		Nonce:            nonce[:],
		SenderPubkey:     dh.id.BoxPubKey[:],
		SentAtUnix:       time.Now().Unix(),
		Type:             pb.EnvelopeType_DIRECT,
		Signature:        sig,
		PQCKEMCiphertext: kemCT,
		CryptoVersion:    cryptoVersion,
	}

	// Open a stream to the recipient.
	s, err := dh.host.NewStream(ctx, recipientID, DMProtocol)
	if err != nil {
		return nil, fmt.Errorf("open stream to %s: %w", recipientID, err)
	}
	defer s.Close()

	if err := pb.WriteDelimited(s, env); err != nil {
		return nil, fmt.Errorf("write envelope: %w", err)
	}

	// Read ACK.
	var ack pb.Ack
	if err := pb.ReadDelimited(s, &ack); err != nil {
		dh.log.Warn("no ACK received", zap.String("msgID", msgID), zap.Error(err))
	}

	msg := &types.Message{
		ID:             msgID,
		ConversationID: convID,
		SenderID:       dh.id.PeerID,
		RecipientID:    recipientID,
		Ciphertext:     ciphertext,
		Nonce:          nonce,
		SentAt:         time.Now(),
		Direction:      types.Outbound,
		Status:         types.StatusSent,
	}
	if ack.Ok {
		msg.Status = types.StatusDelivered
	}

	if err := dh.store.SaveMessage(msg); err != nil {
		dh.log.Error("save sent message", zap.Error(err))
	}
	return msg, nil
}

// handleStream is called by libp2p when a remote peer opens a DM stream.
func (dh *DMHandler) handleStream(s network.Stream) {
	defer s.Close()

	remotePeer := s.Conn().RemotePeer()
	log := dh.log.With(zap.String("peer", remotePeer.String()))

	var env pb.Envelope
	if err := pb.ReadDelimited(s, &env); err != nil {
		log.Warn("read envelope failed", zap.Error(err))
		return
	}

	// Dedup check before heavy crypto — send ACK for already-delivered messages.
	if delivered, _ := dh.store.IsDelivered(env.MessageId); delivered {
		_ = pb.WriteDelimited(s, &pb.Ack{MessageId: env.MessageId, Ok: true})
		return
	}

	if err := dh.DeliverEnvelope(&env); err != nil {
		log.Warn("deliver envelope failed", zap.Error(err))
		return
	}

	// Send ACK.
	_ = pb.WriteDelimited(s, &pb.Ack{MessageId: env.MessageId, Ok: true})
}

// DeliverEnvelope verifies, decrypts, deduplicates, saves, and notifies
// consumers of an inbound envelope. It is used by both the stream handler
// and the offline mailbox drain path.
func (dh *DMHandler) DeliverEnvelope(env *pb.Envelope) error {
	// Verify signature.
	if err := dh.verifyEnvelope(env); err != nil {
		return fmt.Errorf("invalid signature: %w", err)
	}

	// Dedup.
	if delivered, _ := dh.store.IsDelivered(env.MessageId); delivered {
		return nil // already processed
	}

	// Decrypt.
	senderPubKey, err := indracrypto.BytesToKey(env.SenderPubkey)
	if err != nil {
		return fmt.Errorf("bad sender pubkey: %w", err)
	}
	nonce, err := indracrypto.BytesToNonce(env.Nonce)
	if err != nil {
		return fmt.Errorf("bad nonce: %w", err)
	}

	var plaintext []byte
	switch env.CryptoVersion {
	case indracrypto.CryptoVersionHybrid:
		if len(env.PQCKEMCiphertext) != mlkem.CiphertextSize768 {
			return fmt.Errorf("hybrid envelope missing valid KEM ciphertext (got %d bytes)", len(env.PQCKEMCiphertext))
		}
		plaintext, err = indracrypto.HybridDecryptFrom(env.Ciphertext, nonce, env.PQCKEMCiphertext, senderPubKey, &dh.id.BoxPrivKey, dh.id.PQCDecapKey)
	default:
		plaintext, err = indracrypto.DecryptFrom(env.Ciphertext, nonce, senderPubKey, &dh.id.BoxPrivKey)
	}
	if err != nil {
		return fmt.Errorf("decryption failed: %w", err)
	}

	senderID, err := peer.Decode(env.SenderId)
	if err != nil {
		return fmt.Errorf("bad sender ID: %w", err)
	}
	convID := types.ConversationID(senderID, dh.id.PeerID)

	msg := &types.Message{
		ID:             env.MessageId,
		ConversationID: convID,
		SenderID:       senderID,
		RecipientID:    dh.id.PeerID,
		Plaintext:      plaintext,
		Ciphertext:     env.Ciphertext,
		ReceivedAt:     time.Now(),
		SentAt:         time.Unix(env.SentAtUnix, 0),
		Direction:      types.Inbound,
		Status:         types.StatusDelivered,
	}
	copy(msg.Nonce[:], env.Nonce)

	if err := dh.store.SaveMessage(msg); err != nil {
		dh.log.Error("save received message", zap.Error(err))
	}
	_ = dh.store.MarkDelivered(env.MessageId)

	// Notify consumers (non-blocking).
	select {
	case dh.inbound <- *msg:
	default:
		dh.log.Warn("inbound message channel full, dropping notification")
	}

	return nil
}

// sign computes an Ed25519 signature over (messageID || ciphertext || nonce).
func (dh *DMHandler) sign(msgID string, ciphertext, nonce []byte) ([]byte, error) {
	payload := append([]byte(msgID), ciphertext...)
	payload = append(payload, nonce...)
	return dh.id.PrivKey.Sign(payload)
}

// verifyEnvelope checks the Ed25519 signature in the envelope.
func (dh *DMHandler) verifyEnvelope(env *pb.Envelope) error {
	senderID, err := peer.Decode(env.SenderId)
	if err != nil {
		return fmt.Errorf("decode sender ID: %w", err)
	}
	pubKey, err := senderID.ExtractPublicKey()
	if err != nil {
		return fmt.Errorf("extract pubkey from peerID: %w", err)
	}

	payload := append([]byte(env.MessageId), env.Ciphertext...)
	payload = append(payload, env.Nonce...)

	ok, err := pubKey.Verify(payload, env.Signature)
	if err != nil {
		return fmt.Errorf("verify signature: %w", err)
	}
	if !ok {
		return fmt.Errorf("signature mismatch")
	}
	return nil
}

