// Package mobile provides a gomobile-compatible API for the Indra P2P
// messaging library. All exported types use primitives, []byte, or simple
// interfaces so that `gomobile bind` can generate Java/ObjC bindings.
package mobile

import (
	"context"
	"crypto/mlkem"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"go.uber.org/zap"

	"github.com/aryaman/indra/internal/identity"
	"github.com/aryaman/indra/internal/node"
	"github.com/aryaman/indra/internal/store"
	"github.com/aryaman/indra/pkg/types"
)

// InboundHandler is implemented by mobile code to receive messages.
// gomobile generates the bridge so Java/Swift can implement this interface.
type InboundHandler interface {
	// OnMessage is called with a JSON-encoded message when one arrives.
	OnMessage(jsonMessage string)
}

// Client is the single entry point for mobile consumers.
type Client struct {
	mu      sync.Mutex
	dataDir string
	st      *store.Store
	id      *identity.Identity
	node    *node.Node
	log     *zap.Logger
	cancel  context.CancelFunc

	handler InboundHandler
}

// NewClient initialises storage and identity in dataDir.
// Call Start() afterwards to bring up the network.
func NewClient(dataDir string) (*Client, error) {
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	log, _ := zap.NewProduction()

	st, err := store.Open(filepath.Join(dataDir, "db"), log)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}

	id, err := identity.Load(st)
	if err != nil {
		st.Close()
		return nil, fmt.Errorf("load identity: %w", err)
	}

	return &Client{
		dataDir: dataDir,
		st:      st,
		id:      id,
		log:     log,
	}, nil
}

// SetInboundHandler registers a callback for incoming messages.
// Must be called before Start().
func (c *Client) SetInboundHandler(h InboundHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handler = h
}

// Start brings up the libp2p node and begins listening.
// listenAddr is a multiaddr string (e.g. "/ip4/0.0.0.0/tcp/0").
// bootstrapPeer is an optional multiaddr of a bootstrap node (empty string to skip).
func (c *Client) Start(listenAddr, bootstrapPeer string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.node != nil {
		return fmt.Errorf("already started")
	}

	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel

	listen := []string{listenAddr}
	if listenAddr == "" {
		listen = []string{"/ip4/0.0.0.0/tcp/0", "/ip4/0.0.0.0/udp/0/quic-v1"}
	}

	var bootstrap []string
	if bootstrapPeer != "" {
		bootstrap = []string{bootstrapPeer}
	}

	cfg := node.Config{
		ListenAddrs:    listen,
		BootstrapPeers: bootstrap,
		DataDir:        c.dataDir,
	}

	n, err := node.New(ctx, c.id, c.st, cfg, c.log)
	if err != nil {
		cancel()
		return fmt.Errorf("start node: %w", err)
	}
	c.node = n

	// Pump inbound messages to the handler.
	go c.pumpInbound(ctx)

	return nil
}

// Stop shuts down the node gracefully.
func (c *Client) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
	if c.node != nil {
		c.node.Close()
		c.node = nil
	}
}

// Close releases all resources including the database.
// The Client cannot be reused after Close.
func (c *Client) Close() {
	c.Stop()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.st != nil {
		c.st.Close()
		c.st = nil
	}
}

// whoamiJSON is the JSON structure returned by Whoami.
type whoamiJSON struct {
	PeerID    string `json:"peer_id"`
	BoxPubkey string `json:"box_pubkey"`
	PQCPubkey string `json:"pqc_pubkey"`
}

// Whoami returns a JSON string with the node's peer ID, classical public key,
// and ML-KEM-768 encapsulation key. Share this with contacts so they can add you.
func (c *Client) Whoami() string {
	pqcPubkey := ""
	if c.id.PQCDecapKey != nil {
		pqcPubkey = hex.EncodeToString(c.id.PQCDecapKey.EncapsulationKey().Bytes())
	}
	w := whoamiJSON{
		PeerID:    c.id.PeerID.String(),
		BoxPubkey: hex.EncodeToString(c.id.BoxPubKey[:]),
		PQCPubkey: pqcPubkey,
	}
	data, _ := json.Marshal(w)
	return string(data)
}

// PeerID returns this node's peer ID string.
func (c *Client) PeerID() string {
	return c.id.PeerID.String()
}

// AddContact saves a contact. pubkeyHex is the 64-char hex Curve25519 public key.
func (c *Client) AddContact(peerID, pubkeyHex, alias string) error {
	pid, err := peer.Decode(peerID)
	if err != nil {
		return fmt.Errorf("invalid peer ID: %w", err)
	}

	pubBytes, err := hex.DecodeString(pubkeyHex)
	if err != nil {
		return fmt.Errorf("invalid pubkey hex: %w", err)
	}
	if len(pubBytes) != 32 {
		return fmt.Errorf("pubkey must be 32 bytes, got %d", len(pubBytes))
	}

	if alias == "" {
		alias = peerID[:12]
	}

	contact := &types.Contact{
		PeerID:    pid,
		Alias:     alias,
		PublicKey: pubBytes,
		AddedAt:   time.Now(),
	}
	return c.st.SaveContact(contact)
}

// AddContactPQC saves a contact with both classical and ML-KEM-768 public keys.
// pqcPubkeyHex is the 2368-char hex ML-KEM-768 encapsulation key.
// Pass an empty string for pqcPubkeyHex to add a legacy (NaCl-only) contact.
func (c *Client) AddContactPQC(peerID, pubkeyHex, alias, pqcPubkeyHex string) error {
	pid, err := peer.Decode(peerID)
	if err != nil {
		return fmt.Errorf("invalid peer ID: %w", err)
	}

	pubBytes, err := hex.DecodeString(pubkeyHex)
	if err != nil {
		return fmt.Errorf("invalid pubkey hex: %w", err)
	}
	if len(pubBytes) != 32 {
		return fmt.Errorf("pubkey must be 32 bytes, got %d", len(pubBytes))
	}

	var pqcBytes []byte
	if pqcPubkeyHex != "" {
		pqcBytes, err = hex.DecodeString(pqcPubkeyHex)
		if err != nil {
			return fmt.Errorf("invalid PQC pubkey hex: %w", err)
		}
		if len(pqcBytes) != mlkem.EncapsulationKeySize768 {
			return fmt.Errorf("PQC pubkey must be %d bytes, got %d", mlkem.EncapsulationKeySize768, len(pqcBytes))
		}
		// Validate the key parses correctly.
		if _, err := mlkem.NewEncapsulationKey768(pqcBytes); err != nil {
			return fmt.Errorf("invalid PQC encapsulation key: %w", err)
		}
	}

	if alias == "" {
		alias = peerID[:12]
	}

	contact := &types.Contact{
		PeerID:    pid,
		Alias:     alias,
		PublicKey: pubBytes,
		PQCPubKey: pqcBytes,
		AddedAt:   time.Now(),
	}
	return c.st.SaveContact(contact)
}

// ParseAndAddContact parses a Whoami JSON string (from a QR scan) and adds the contact.
// whoamiStr is the JSON returned by the other node's Whoami() call.
func (c *Client) ParseAndAddContact(whoamiStr, alias string) error {
	var w whoamiJSON
	if err := json.Unmarshal([]byte(whoamiStr), &w); err != nil {
		return fmt.Errorf("parse whoami: %w", err)
	}
	return c.AddContactPQC(w.PeerID, w.BoxPubkey, alias, w.PQCPubkey)
}

// SendMessage sends a direct message to the given peer ID.
func (c *Client) SendMessage(peerID, text string) error {
	c.mu.Lock()
	n := c.node
	c.mu.Unlock()

	if n == nil {
		return fmt.Errorf("node not started")
	}

	pid, err := peer.Decode(peerID)
	if err != nil {
		return fmt.Errorf("invalid peer ID: %w", err)
	}

	_, err = n.SendMessage(context.Background(), pid, []byte(text))
	return err
}

// SendGroupMessage sends a message to all members of the given group.
func (c *Client) SendGroupMessage(groupID, text string) error {
	c.mu.Lock()
	n := c.node
	c.mu.Unlock()

	if n == nil {
		return fmt.Errorf("node not started")
	}

	return n.SendGroupMessage(context.Background(), groupID, []byte(text))
}

// CreateGroup creates a group with the given name and member peer IDs (comma-separated).
func (c *Client) CreateGroup(name, memberPeerIDsCSV string) (string, error) {
	members := []peer.ID{c.id.PeerID}

	if memberPeerIDsCSV != "" {
		for _, s := range splitCSV(memberPeerIDsCSV) {
			pid, err := peer.Decode(s)
			if err != nil {
				return "", fmt.Errorf("invalid peer ID %q: %w", s, err)
			}
			members = append(members, pid)
		}
	}

	group := &types.Group{
		ID:        fmt.Sprintf("grp:%s:%d", name, time.Now().UnixNano()),
		Name:      name,
		Members:   members,
		CreatorID: c.id.PeerID,
		CreatedAt: time.Now(),
	}

	if err := c.st.SaveGroup(group); err != nil {
		return "", err
	}
	return group.ID, nil
}

// conversationJSON is the JSON-safe representation of a Conversation.
type conversationJSON struct {
	ID           string   `json:"id"`
	IsGroup      bool     `json:"is_group"`
	Name         string   `json:"name"`
	Participants []string `json:"participants"`
	UnreadCount  int      `json:"unread_count"`
}

// messageJSON is the JSON-safe representation of a Message.
type messageJSON struct {
	ID             string `json:"id"`
	ConversationID string `json:"conversation_id"`
	SenderID       string `json:"sender_id"`
	Text           string `json:"text"`
	SentAt         int64  `json:"sent_at_unix"`
	Direction      string `json:"direction"` // "outbound" or "inbound"
}

// GetConversations returns a JSON array of all conversations.
func (c *Client) GetConversations() string {
	result := []conversationJSON{}

	contacts, _ := c.st.ListContacts()
	for _, ct := range contacts {
		convID := types.ConversationID(c.id.PeerID, ct.PeerID)
		parts := []string{c.id.PeerID.String(), ct.PeerID.String()}
		result = append(result, conversationJSON{
			ID:           convID,
			Name:         ct.Alias,
			Participants: parts,
		})
	}

	groups, _ := c.st.ListGroups()
	for _, g := range groups {
		parts := make([]string, len(g.Members))
		for i, m := range g.Members {
			parts[i] = m.String()
		}
		result = append(result, conversationJSON{
			ID:           g.ID,
			IsGroup:      true,
			Name:         g.Name,
			Participants: parts,
		})
	}

	data, _ := json.Marshal(result)
	return string(data)
}

// GetMessages returns a JSON array of the last `limit` messages in a conversation.
func (c *Client) GetMessages(convID string, limit int) string {
	msgs, err := c.st.ListMessages(convID, limit, time.Time{})
	if err != nil {
		return "[]"
	}

	result := make([]messageJSON, len(msgs))
	for i, m := range msgs {
		dir := "outbound"
		if m.Direction == types.Inbound {
			dir = "inbound"
		}
		result[i] = messageJSON{
			ID:             m.ID,
			ConversationID: m.ConversationID,
			SenderID:       m.SenderID.String(),
			Text:           string(m.Plaintext),
			SentAt:         m.SentAt.Unix(),
			Direction:      dir,
		}
	}

	data, _ := json.Marshal(result)
	return string(data)
}

// SetRelayURL configures the push notification relay URL (e.g. "https://relay.indra.chat").
// Must be called after Start(). The relay sends silent pushes to wake offline recipients.
func (c *Client) SetRelayURL(url string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.node != nil {
		c.node.SetRelayURL(url)
	}
}

// RegisterPushToken registers this device's push token with the relay.
// platform is "ios" or "android". Must be called after Start() and SetRelayURL().
func (c *Client) RegisterPushToken(token, platform string) error {
	c.mu.Lock()
	n := c.node
	c.mu.Unlock()

	if n == nil {
		return fmt.Errorf("node not started")
	}

	return n.RegisterPushToken(context.Background(), c.id.PeerID.String(), token, platform)
}

// FetchMailbox triggers an immediate poll of the DHT mailbox for offline messages.
// Call this when the app wakes from a silent push notification.
func (c *Client) FetchMailbox() {
	c.mu.Lock()
	n := c.node
	c.mu.Unlock()
	if n != nil && n.Mailbox != nil {
		go n.Mailbox.FetchInbox(context.Background())
	}
}

// Addrs returns the node's listen addresses as a JSON array of strings.
// Returns "[]" if the node is not started.
func (c *Client) Addrs() string {
	c.mu.Lock()
	n := c.node
	c.mu.Unlock()

	if n == nil {
		return "[]"
	}

	data, _ := json.Marshal(n.Addrs())
	return string(data)
}

// pumpInbound drains the node's inbound channel and forwards to the handler.
func (c *Client) pumpInbound(ctx context.Context) {
	for {
		c.mu.Lock()
		n := c.node
		c.mu.Unlock()
		if n == nil {
			return
		}

		select {
		case <-ctx.Done():
			return
		case msg := <-n.InboundMessages:
			c.mu.Lock()
			h := c.handler
			c.mu.Unlock()
			if h == nil {
				continue
			}

			dir := "outbound"
			if msg.Direction == types.Inbound {
				dir = "inbound"
			}
			j := messageJSON{
				ID:             msg.ID,
				ConversationID: msg.ConversationID,
				SenderID:       msg.SenderID.String(),
				Text:           string(msg.Plaintext),
				SentAt:         msg.SentAt.Unix(),
				Direction:      dir,
			}
			data, _ := json.Marshal(j)
			h.OnMessage(string(data))
		}
	}
}

// splitCSV splits a comma-separated string, trimming whitespace.
func splitCSV(s string) []string {
	var result []string
	for _, part := range splitOn(s, ',') {
		trimmed := trimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func splitOn(s string, sep byte) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}
