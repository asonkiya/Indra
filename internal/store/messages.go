package store

import (
	"encoding/json"
	"fmt"
	"time"

	badger "github.com/dgraph-io/badger/v4"
	"github.com/aryaman/indra/pkg/types"
)

// Key layout:
//   msg:<convID>:<timestamp_ns>:<msgID> -> JSON-serialized message

const msgPrefix = "msg:"

// SaveMessage persists a message to BadgerDB.
func (s *Store) SaveMessage(m *types.Message) error {
	if m.ID == "" {
		return fmt.Errorf("message ID must not be empty")
	}
	key := msgKey(m.ConversationID, m.SentAt, m.ID)
	// Never persist plaintext.
	safe := *m
	safe.Plaintext = nil
	data, err := json.Marshal(&safe)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}
	return s.set(key, data)
}

// ListMessages returns up to limit messages in a conversation, ordered oldest-first.
// If before is non-zero, only messages strictly before that time are returned.
func (s *Store) ListMessages(conversationID string, limit int, before time.Time) ([]*types.Message, error) {
	prefix := []byte(msgPrefix + conversationID + ":")
	var messages []*types.Message

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			var m types.Message
			err := item.Value(func(v []byte) error {
				return json.Unmarshal(v, &m)
			})
			if err != nil {
				s.log.Sugar().Warnf("skip corrupt message at key %s: %v", item.Key(), err)
				continue
			}
			if !before.IsZero() && !m.SentAt.Before(before) {
				continue
			}
			messages = append(messages, &m)
			if limit > 0 && len(messages) >= limit {
				break
			}
		}
		return nil
	})
	return messages, err
}

// UpdateStatus updates only the Status field of a stored message.
func (s *Store) UpdateStatus(m *types.Message) error {
	return s.SaveMessage(m) // full overwrite — safe since key includes msgID
}

// msgKey constructs a BadgerDB key that sorts messages by conversation then time.
func msgKey(convID string, t time.Time, msgID string) []byte {
	// Zero-pad nanoseconds to 19 digits so lexicographic order == time order.
	return []byte(fmt.Sprintf("%s%s:%019d:%s", msgPrefix, convID, t.UnixNano(), msgID))
}
