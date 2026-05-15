package types

import (
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

type MessageDirection int8

const (
	Outbound MessageDirection = 1
	Inbound  MessageDirection = 2
)

type MessageStatus int8

const (
	StatusPending   MessageStatus = 0
	StatusSent      MessageStatus = 1
	StatusStored    MessageStatus = 2 // placed in DHT mailbox
	StatusDelivered MessageStatus = 3
	StatusFailed    MessageStatus = 4
)

type Message struct {
	ID             string
	ConversationID string // DM: sorted(senderID+recipientID); group: groupID
	SenderID       peer.ID
	RecipientID    peer.ID // empty for group messages
	GroupID        string  // non-empty for group messages
	Plaintext      []byte  // never persisted; populated after decryption
	Ciphertext     []byte
	Nonce          [24]byte
	SentAt         time.Time
	ReceivedAt     time.Time
	Direction      MessageDirection
	Status         MessageStatus
}

type Contact struct {
	PeerID    peer.ID
	Alias     string
	PublicKey []byte `json:",omitempty"` // Curve25519 public key for NaCl encryption
	PQCPubKey []byte `json:",omitempty"` // ML-KEM-768 encapsulation key (1184 bytes); nil for legacy contacts
	Addrs     []string
	AddedAt   time.Time
	LastSeen  time.Time
}

type Conversation struct {
	ID           string
	IsGroup      bool
	Name         string
	Participants []peer.ID
	LastMessage  *Message
	UnreadCount  int
	Messages     []*Message // loaded at startup, appended on receive
}

type Group struct {
	ID        string
	Name      string
	Members   []peer.ID
	CreatorID peer.ID
	CreatedAt time.Time
}

// ConversationID returns a stable DM conversation ID for two peers.
// It sorts the IDs so A→B and B→A share the same conversation.
func ConversationID(a, b peer.ID) string {
	as, bs := a.String(), b.String()
	if as < bs {
		return as + ":" + bs
	}
	return bs + ":" + as
}
