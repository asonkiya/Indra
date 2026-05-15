package main

import "sync"

// Registration holds a push token and its platform.
type Registration struct {
	Token    string `json:"token"`
	Platform string `json:"platform"` // "ios" or "android"
}

// Store is a thread-safe in-memory map of peerID → push registration.
// Tokens change frequently so persistence is not needed; devices re-register on launch.
type Store struct {
	mu    sync.RWMutex
	peers map[string]Registration
}

// NewStore creates an empty Store.
func NewStore() *Store {
	return &Store{peers: make(map[string]Registration)}
}

// Register saves or updates a push token for a peer.
func (s *Store) Register(peerID string, reg Registration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.peers[peerID] = reg
}

// Lookup returns the registration for a peer, if any.
func (s *Store) Lookup(peerID string) (Registration, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.peers[peerID]
	return r, ok
}

// Remove deletes a peer's registration.
func (s *Store) Remove(peerID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.peers, peerID)
}

// Count returns the number of registered peers.
func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.peers)
}
