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

// ContactKeys holds the public keys for a contact used during group message fan-out.
type ContactKeys struct {
	BoxPubKey [32]byte
	PQCPubKey []byte // ML-KEM-768 encapsulation key (1184 bytes); nil for legacy contacts
}

const GroupProtocol = "/indra/group/1.0.0"

// GroupHandler handles group message fan-out sending and receiving.
type GroupHandler struct {
	host    host.Host
	id      *identity.Identity
	store   *store.Store
	log     *zap.Logger
	inbound chan<- types.Message
}

// NewGroupHandler registers the group stream handler on the host.
func NewGroupHandler(h host.Host, id *identity.Identity, st *store.Store, log *zap.Logger, inbound chan<- types.Message) *GroupHandler {
	gh := &GroupHandler{
		host:    h,
		id:      id,
		store:   st,
		log:     log,
		inbound: inbound,
	}
	h.SetStreamHandler(GroupProtocol, gh.handleStream)
	return gh
}

// Send delivers a group message by fan-out: one encrypted envelope per member.
// contacts maps peer.ID to that member's public keys; PQCPubKey may be nil for legacy contacts.
func (gh *GroupHandler) Send(ctx context.Context, group *types.Group, plaintext []byte, contacts map[peer.ID]ContactKeys) error {
	msgID := uuid.New().String()
	now := time.Now()

	for _, memberID := range group.Members {
		if memberID == gh.id.PeerID {
			continue // don't send to ourselves
		}

		keys, ok := contacts[memberID]
		if !ok {
			gh.log.Warn("no pubkey for group member, skipping",
				zap.String("member", memberID.String()),
				zap.String("group", group.ID),
			)
			continue
		}

		nonce, err := gh.store.NextNonce(group.ID + ":" + memberID.String())
		if err != nil {
			gh.log.Error("get nonce", zap.Error(err))
			continue
		}

		var ciphertext, kemCT []byte
		var cryptoVersion int32

		if len(keys.PQCPubKey) == indracrypto.PQCEncapsulationKeySize {
			ct, kct, err := indracrypto.HybridEncryptFor(plaintext, &nonce, keys.PQCPubKey, &keys.BoxPubKey, &gh.id.BoxPrivKey)
			if err != nil {
				gh.log.Error("hybrid encrypt for group member", zap.String("member", memberID.String()), zap.Error(err))
				continue
			}
			ciphertext = ct
			kemCT = kct
			cryptoVersion = indracrypto.CryptoVersionHybrid
		} else {
			ciphertext = indracrypto.EncryptFor(plaintext, &nonce, &keys.BoxPubKey, &gh.id.BoxPrivKey)
			cryptoVersion = indracrypto.CryptoVersionNaCl
		}

		sig, err := gh.signEnvelope(msgID, ciphertext, nonce[:])
		if err != nil {
			gh.log.Error("sign envelope", zap.Error(err))
			continue
		}

		env := &pb.Envelope{
			MessageId:        msgID,
			SenderId:         gh.id.PeerID.String(),
			RecipientId:      memberID.String(),
			GroupId:          group.ID,
			Ciphertext:       ciphertext,
			Nonce:            nonce[:],
			SenderPubkey:     gh.id.BoxPubKey[:],
			SentAtUnix:       now.Unix(),
			Type:             pb.EnvelopeType_GROUP,
			Signature:        sig,
			PQCKEMCiphertext: kemCT,
			CryptoVersion:    cryptoVersion,
		}

		if err := gh.deliver(ctx, memberID, env); err != nil {
			gh.log.Warn("deliver group message failed",
				zap.String("member", memberID.String()),
				zap.Error(err),
			)
		}
	}

	// Save our own copy (plaintext not persisted).
	msg := &types.Message{
		ID:             msgID,
		ConversationID: group.ID,
		SenderID:       gh.id.PeerID,
		GroupID:        group.ID,
		SentAt:         now,
		Direction:      types.Outbound,
		Status:         types.StatusSent,
	}
	return gh.store.SaveMessage(msg)
}

func (gh *GroupHandler) deliver(ctx context.Context, memberID peer.ID, env *pb.Envelope) error {
	s, err := gh.host.NewStream(ctx, memberID, GroupProtocol)
	if err != nil {
		return fmt.Errorf("open stream: %w", err)
	}
	defer s.Close()
	return pb.WriteDelimited(s, env)
}

func (gh *GroupHandler) handleStream(s network.Stream) {
	defer s.Close()
	remotePeer := s.Conn().RemotePeer()
	log := gh.log.With(zap.String("peer", remotePeer.String()))

	var env pb.Envelope
	if err := pb.ReadDelimited(s, &env); err != nil {
		log.Warn("read group envelope failed", zap.Error(err))
		return
	}

	if delivered, _ := gh.store.IsDelivered(env.MessageId); delivered {
		return
	}

	senderPubKey, err := indracrypto.BytesToKey(env.SenderPubkey)
	if err != nil {
		log.Warn("bad sender pubkey", zap.Error(err))
		return
	}
	nonce, err := indracrypto.BytesToNonce(env.Nonce)
	if err != nil {
		log.Warn("bad nonce", zap.Error(err))
		return
	}

	var plaintext []byte
	switch env.CryptoVersion {
	case indracrypto.CryptoVersionHybrid:
		if len(env.PQCKEMCiphertext) != mlkem.CiphertextSize768 {
			log.Warn("hybrid group envelope missing valid KEM ciphertext",
				zap.Int("got", len(env.PQCKEMCiphertext)))
			return
		}
		plaintext, err = indracrypto.HybridDecryptFrom(env.Ciphertext, nonce, env.PQCKEMCiphertext, senderPubKey, &gh.id.BoxPrivKey, gh.id.PQCDecapKey)
	default:
		plaintext, err = indracrypto.DecryptFrom(env.Ciphertext, nonce, senderPubKey, &gh.id.BoxPrivKey)
	}
	if err != nil {
		log.Warn("decryption failed", zap.Error(err))
		return
	}

	senderID, err := peer.Decode(env.SenderId)
	if err != nil {
		log.Warn("bad sender ID", zap.Error(err))
		return
	}

	msg := &types.Message{
		ID:             env.MessageId,
		ConversationID: env.GroupId,
		SenderID:       senderID,
		GroupID:        env.GroupId,
		Plaintext:      plaintext,
		Ciphertext:     env.Ciphertext,
		ReceivedAt:     time.Now(),
		SentAt:         time.Unix(env.SentAtUnix, 0),
		Direction:      types.Inbound,
		Status:         types.StatusDelivered,
	}
	copy(msg.Nonce[:], env.Nonce)

	if err := gh.store.SaveMessage(msg); err != nil {
		log.Error("save group message", zap.Error(err))
	}
	_ = gh.store.MarkDelivered(env.MessageId)

	select {
	case gh.inbound <- *msg:
	default:
		log.Warn("inbound channel full")
	}
}

func (gh *GroupHandler) signEnvelope(msgID string, ciphertext, nonce []byte) ([]byte, error) {
	payload := append([]byte(msgID), ciphertext...)
	payload = append(payload, nonce...)
	return gh.id.PrivKey.Sign(payload)
}
