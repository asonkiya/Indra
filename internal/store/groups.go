package store

import (
	"encoding/json"
	"fmt"

	badger "github.com/dgraph-io/badger/v4"
	"github.com/aryaman/indra/pkg/types"
)

// Key layout:
//   group:<groupID> -> JSON-serialized Group

const groupPrefix = "group:"

// SaveGroup upserts a group.
func (s *Store) SaveGroup(g *types.Group) error {
	if g.ID == "" {
		return fmt.Errorf("group ID must not be empty")
	}
	data, err := json.Marshal(g)
	if err != nil {
		return fmt.Errorf("marshal group: %w", err)
	}
	return s.set([]byte(groupPrefix+g.ID), data)
}

// GetGroup looks up a group by ID. Returns badger.ErrKeyNotFound if absent.
func (s *Store) GetGroup(id string) (*types.Group, error) {
	raw, err := s.get([]byte(groupPrefix + id))
	if err != nil {
		return nil, err
	}
	var g types.Group
	return &g, json.Unmarshal(raw, &g)
}

// ListGroups returns all groups this node is a member of.
func (s *Store) ListGroups() ([]*types.Group, error) {
	prefix := []byte(groupPrefix)
	var groups []*types.Group

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			var g types.Group
			err := it.Item().Value(func(v []byte) error {
				return json.Unmarshal(v, &g)
			})
			if err != nil {
				continue
			}
			groups = append(groups, &g)
		}
		return nil
	})
	return groups, err
}
