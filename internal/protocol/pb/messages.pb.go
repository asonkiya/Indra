// Package pb defines Indra's wire types.
//
// NOTE: This file is hand-written to avoid a protoc dependency at build time.
// The canonical schema is messages.proto. When protoc is available, run
// "make proto" to regenerate this file from the .proto definition.
//
// Serialization uses length-prefixed JSON for now; the format is compatible
// with the proto field names (json tags match proto field names) so a future
// migration to binary protobuf only requires swapping the Marshal/Unmarshal
// calls without changing any other code.
package pb

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

// EnvelopeType indicates the kind of message carried in an Envelope.
type EnvelopeType int32

const (
	EnvelopeType_DIRECT  EnvelopeType = 0
	EnvelopeType_GROUP   EnvelopeType = 1
	EnvelopeType_ACK     EnvelopeType = 2
	EnvelopeType_OFFLINE EnvelopeType = 3
)

// Envelope is the wire format for all messages (stream and DHT).
type Envelope struct {
	MessageId       string       `json:"message_id"`
	SenderId        string       `json:"sender_id"`
	RecipientId     string       `json:"recipient_id"`
	GroupId         string       `json:"group_id,omitempty"`
	Ciphertext      []byte       `json:"ciphertext"`
	Nonce           []byte       `json:"nonce"` // 24 bytes
	SenderPubkey    []byte       `json:"sender_pubkey"`
	SentAtUnix      int64        `json:"sent_at_unix"`
	Type            EnvelopeType `json:"type"`
	Signature       []byte       `json:"signature"` // Ed25519 over (message_id || ciphertext || nonce)
	Seq             int64        `json:"seq,omitempty"`
	PQCKEMCiphertext []byte      `json:"pqc_kem_ciphertext,omitempty"` // ML-KEM-768 ciphertext (1088 bytes) when CryptoVersion==2
	CryptoVersion   int32        `json:"crypto_version,omitempty"`     // 0=NaCl (default), 2=hybrid PQC+NaCl
}

func (e *Envelope) GetMessageId() string         { return e.MessageId }
func (e *Envelope) GetSenderId() string          { return e.SenderId }
func (e *Envelope) GetRecipientId() string       { return e.RecipientId }
func (e *Envelope) GetGroupId() string           { return e.GroupId }
func (e *Envelope) GetCiphertext() []byte        { return e.Ciphertext }
func (e *Envelope) GetNonce() []byte             { return e.Nonce }
func (e *Envelope) GetSenderPubkey() []byte      { return e.SenderPubkey }
func (e *Envelope) GetSentAtUnix() int64         { return e.SentAtUnix }
func (e *Envelope) GetType() EnvelopeType        { return e.Type }
func (e *Envelope) GetSignature() []byte         { return e.Signature }
func (e *Envelope) GetSeq() int64                { return e.Seq }
func (e *Envelope) GetPQCKEMCiphertext() []byte  { return e.PQCKEMCiphertext }
func (e *Envelope) GetCryptoVersion() int32      { return e.CryptoVersion }

// GroupMemberList is stored in the DHT under /indra/group/<groupID>/members.
type GroupMemberList struct {
	GroupId   string   `json:"group_id"`
	GroupName string   `json:"group_name"`
	MemberIds []string `json:"member_ids"`
	UpdatedAt int64    `json:"updated_at"`
}

// Ack is sent back on a stream after successfully receiving a message.
type Ack struct {
	MessageId string `json:"message_id"`
	Ok        bool   `json:"ok"`
}

// Marshal serializes v to JSON bytes.
func Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

// Unmarshal deserializes JSON bytes into v.
func Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

// WriteDelimited writes a length-prefixed message to w.
// The 4-byte big-endian length prefix is followed by the JSON payload.
func WriteDelimited(w io.Writer, v any) error {
	data, err := Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	length := uint32(len(data))
	if err := binary.Write(w, binary.BigEndian, length); err != nil {
		return fmt.Errorf("write length: %w", err)
	}
	_, err = w.Write(data)
	return err
}

// ReadDelimited reads a length-prefixed message from r and unmarshals into v.
func ReadDelimited(r io.Reader, v any) error {
	var length uint32
	if err := binary.Read(r, binary.BigEndian, &length); err != nil {
		return fmt.Errorf("read length: %w", err)
	}
	if length == 0 || length > 4*1024*1024 { // 4 MB safety cap
		return fmt.Errorf("invalid message length: %d", length)
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return fmt.Errorf("read body: %w", err)
	}
	return Unmarshal(data, v)
}
