package store

import (
	"encoding/json"
	"fmt"

	badger "github.com/dgraph-io/badger/v4"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/aryaman/indra/pkg/types"
)

// Key layout:
//   contact:<peerID> -> JSON-serialized Contact

const contactPrefix = "contact:"

// SaveContact upserts a contact.
func (s *Store) SaveContact(c *types.Contact) error {
	if c.PeerID == "" {
		return fmt.Errorf("contact PeerID must not be empty")
	}
	data, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshal contact: %w", err)
	}
	return s.set([]byte(contactPrefix+c.PeerID.String()), data)
}

// GetContact looks up a contact by PeerID. Returns badger.ErrKeyNotFound if absent.
func (s *Store) GetContact(id peer.ID) (*types.Contact, error) {
	raw, err := s.get([]byte(contactPrefix + id.String()))
	if err != nil {
		return nil, err
	}
	var c types.Contact
	return &c, json.Unmarshal(raw, &c)
}

// ListContacts returns all known contacts.
func (s *Store) ListContacts() ([]*types.Contact, error) {
	prefix := []byte(contactPrefix)
	var contacts []*types.Contact

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			var c types.Contact
			err := it.Item().Value(func(v []byte) error {
				return json.Unmarshal(v, &c)
			})
			if err != nil {
				continue
			}
			contacts = append(contacts, &c)
		}
		return nil
	})
	return contacts, err
}

// DeleteContact removes a contact.
func (s *Store) DeleteContact(id peer.ID) error {
	return s.delete([]byte(contactPrefix + id.String()))
}
