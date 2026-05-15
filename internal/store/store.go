package store

import (
	"fmt"

	badger "github.com/dgraph-io/badger/v4"
	"go.uber.org/zap"
)

// Store wraps a BadgerDB instance and provides typed accessors.
type Store struct {
	db  *badger.DB
	log *zap.Logger
}

// Open opens (or creates) a BadgerDB database at path.
func Open(path string, log *zap.Logger) (*Store, error) {
	opts := badger.DefaultOptions(path).
		WithLogger(nil). // use our own logger
		WithNumGoroutines(4)
	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("open badger at %s: %w", path, err)
	}
	return &Store{db: db, log: log}, nil
}

// Close flushes and closes the database.
func (s *Store) Close() error {
	return s.db.Close()
}

// DB returns the underlying badger.DB for packages that need raw access.
func (s *Store) DB() *badger.DB {
	return s.db
}

// get retrieves raw bytes for key. Returns ErrKeyNotFound if missing.
func (s *Store) get(key []byte) ([]byte, error) {
	var val []byte
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			return err
		}
		val, err = item.ValueCopy(nil)
		return err
	})
	return val, err
}

// set writes raw bytes for key inside a single-item transaction.
func (s *Store) set(key, value []byte) error {
	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, value)
	})
}

// delete removes a key.
func (s *Store) delete(key []byte) error {
	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Delete(key)
	})
}

// exists returns true if the key is present.
func (s *Store) exists(key []byte) (bool, error) {
	err := s.db.View(func(txn *badger.Txn) error {
		_, err := txn.Get(key)
		return err
	})
	if err == badger.ErrKeyNotFound {
		return false, nil
	}
	return err == nil, err
}
