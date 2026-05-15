// Package node wires together the libp2p host, DHT, discovery, and messaging.
package node

import (
	"context"
	"fmt"
	"time"

	libp2p "github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	"github.com/multiformats/go-multiaddr"
	"go.uber.org/zap"

	"github.com/aryaman/indra/internal/discovery"
	"github.com/aryaman/indra/internal/identity"
	"github.com/aryaman/indra/internal/mailbox"
	"github.com/aryaman/indra/internal/protocol"
	"github.com/aryaman/indra/internal/protocol/pb"
	"github.com/aryaman/indra/internal/relay"
	"github.com/aryaman/indra/internal/store"
	"github.com/aryaman/indra/pkg/types"
)

// Node is the central coordinator for an Indra peer.
type Node struct {
	Host      host.Host
	DHT       *dht.IpfsDHT
	Identity  *identity.Identity
	Store     *store.Store
	Log       *zap.Logger
	Discovery *discovery.Discovery
	Mailbox   *mailbox.Mailbox
	DM        *protocol.DMHandler
	Group     *protocol.GroupHandler

	// Relay is an optional push notification relay client.
	// When set, the node notifies the relay after storing a mailbox message.
	Relay *relay.Client

	// InboundMessages receives decrypted messages from stream handlers.
	// Consumers (e.g. TUI) read from this channel.
	InboundMessages chan types.Message

	// mailboxInbound carries raw envelopes pulled from the DHT mailbox.
	mailboxInbound chan pb.Envelope

	cancel context.CancelFunc
}


// Config holds runtime configuration for a Node.
type Config struct {
	ListenAddrs  []string
	BootstrapPeers []string
	DataDir      string
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		ListenAddrs: []string{
			"/ip4/0.0.0.0/tcp/4001",
			"/ip4/0.0.0.0/udp/4001/quic-v1",
		},
	}
}

// New creates and starts a fully configured Indra node.
func New(ctx context.Context, id *identity.Identity, st *store.Store, cfg Config, log *zap.Logger) (*Node, error) {
	ctx, cancel := context.WithCancel(ctx)

	// Parse listen addresses.
	var listenAddrs []multiaddr.Multiaddr
	for _, a := range cfg.ListenAddrs {
		ma, err := multiaddr.NewMultiaddr(a)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("invalid listen addr %q: %w", a, err)
		}
		listenAddrs = append(listenAddrs, ma)
	}

	// Connection manager: keep between 25 and 100 connections.
	cm, err := connmgr.NewConnManager(25, 100, connmgr.WithGracePeriod(time.Minute))
	if err != nil {
		cancel()
		return nil, fmt.Errorf("create conn manager: %w", err)
	}

	// Build the libp2p host.
	h, err := libp2p.New(
		libp2p.Identity(id.PrivKey),
		libp2p.ListenAddrs(listenAddrs...),
		libp2p.ConnectionManager(cm),
		libp2p.NATPortMap(),
		libp2p.EnableNATService(),
		libp2p.EnableRelay(),
		libp2p.EnableHolePunching(),
	)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("create libp2p host: %w", err)
	}

	// Start the DHT in server mode so this node helps route others.
	kadDHT, err := dht.New(ctx, h,
		dht.Mode(dht.ModeServer),
		dht.ProtocolPrefix("/indra"),
	)
	if err != nil {
		_ = h.Close()
		cancel()
		return nil, fmt.Errorf("create DHT: %w", err)
	}

	inbound := make(chan types.Message, 64)
	mailboxIn := make(chan pb.Envelope, 64)

	n := &Node{
		Host:            h,
		DHT:             kadDHT,
		Identity:        id,
		Store:           st,
		Log:             log,
		InboundMessages: inbound,
		mailboxInbound:  mailboxIn,
		cancel:          cancel,
	}

	// Wire up protocol handlers.
	n.DM = protocol.NewDMHandler(h, id, st, log, inbound)
	n.Group = protocol.NewGroupHandler(h, id, st, log, inbound)

	// Bootstrap the DHT.
	if err := n.bootstrap(ctx, cfg.BootstrapPeers); err != nil {
		log.Warn("DHT bootstrap had errors (network may be limited)", zap.Error(err))
	}

	// Start peer discovery.
	n.Discovery = discovery.New(ctx, h, kadDHT, log)

	// Start offline mailbox polling.
	n.Mailbox = mailbox.New(ctx, kadDHT, st, id.PeerID, log, mailboxIn)

	// Forward mailbox envelopes through the DM handler's delivery path.
	go n.drainMailbox(ctx)

	return n, nil
}

// drainMailbox processes envelopes pulled from the DHT mailbox.
func (n *Node) drainMailbox(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case env := <-n.mailboxInbound:
			if err := n.DM.DeliverEnvelope(&env); err != nil {
				n.Log.Warn("mailbox envelope delivery failed",
					zap.String("msgID", env.MessageId),
					zap.String("from", env.SenderId),
					zap.Error(err),
				)
				continue
			}
			n.Log.Debug("mailbox envelope delivered",
				zap.String("msgID", env.MessageId),
				zap.String("from", env.SenderId),
			)
		}
	}
}

// bootstrap connects to bootstrap peers and starts DHT bootstrapping.
func (n *Node) bootstrap(ctx context.Context, extra []string) error {
	// dht.DefaultBootstrapPeers is []multiaddr.Multiaddr; collect extras the same way.
	addrs := make([]multiaddr.Multiaddr, len(dht.DefaultBootstrapPeers))
	copy(addrs, dht.DefaultBootstrapPeers)

	for _, addrStr := range extra {
		ma, err := multiaddr.NewMultiaddr(addrStr)
		if err != nil {
			n.Log.Warn("skip invalid bootstrap addr", zap.String("addr", addrStr), zap.Error(err))
			continue
		}
		addrs = append(addrs, ma)
	}

	var firstErr error
	for _, ma := range addrs {
		ai, err := peer.AddrInfoFromP2pAddr(ma)
		if err != nil {
			n.Log.Debug("skip unparseable bootstrap addr", zap.Error(err))
			continue
		}
		if err := n.Host.Connect(ctx, *ai); err != nil {
			n.Log.Debug("bootstrap connect failed", zap.String("peer", ai.ID.String()), zap.Error(err))
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		n.Log.Debug("connected to bootstrap peer", zap.String("peer", ai.ID.String()))
	}

	return n.DHT.Bootstrap(ctx)
}

// SendMessage encrypts and delivers a direct message to recipientID.
// It tries a direct stream first; if that fails, it falls back to the DHT mailbox.
func (n *Node) SendMessage(ctx context.Context, recipientID peer.ID, plaintext []byte) (*types.Message, error) {
	contact, err := n.Store.GetContact(recipientID)
	if err != nil {
		return nil, fmt.Errorf("unknown contact %s: add them first with AddContact", recipientID)
	}
	var recipBoxPub [32]byte
	if len(contact.PublicKey) != 32 {
		return nil, fmt.Errorf("contact %s has no valid Curve25519 pubkey", recipientID)
	}
	copy(recipBoxPub[:], contact.PublicKey)

	// Try direct delivery with a timeout.
	directCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	msg, err := n.DM.Send(directCtx, recipientID, plaintext, recipBoxPub, contact.PQCPubKey)
	if err == nil {
		return msg, nil
	}
	n.Log.Warn("direct delivery failed, trying DHT mailbox",
		zap.String("peer", recipientID.String()),
		zap.Error(err),
	)

	// Fallback: store in DHT mailbox for offline delivery.
	if mbErr := n.Mailbox.PutOfflinePlaintext(ctx, recipientID, plaintext, recipBoxPub, contact.PQCPubKey, n.Identity); mbErr != nil {
		return nil, fmt.Errorf("direct: %w; mailbox: %v", err, mbErr)
	}

	// Notify the push relay so the recipient wakes up and polls the mailbox.
	if n.Relay != nil {
		go func() {
			if relayErr := n.Relay.Notify(context.Background(), n.Identity.PeerID.String(), recipientID.String()); relayErr != nil {
				n.Log.Debug("relay notify failed (non-fatal)", zap.Error(relayErr))
			}
		}()
	}

	// Return a StatusStored message.
	convID := types.ConversationID(n.Identity.PeerID, recipientID)
	storedMsg := &types.Message{
		ConversationID: convID,
		SenderID:       n.Identity.PeerID,
		RecipientID:    recipientID,
		Plaintext:      plaintext,
		Direction:      types.Outbound,
		Status:         types.StatusStored,
	}
	return storedMsg, nil
}

// SendGroupMessage encrypts and delivers a message to all group members via fan-out.
func (n *Node) SendGroupMessage(ctx context.Context, groupID string, plaintext []byte) error {
	group, err := n.Store.GetGroup(groupID)
	if err != nil {
		return fmt.Errorf("unknown group %s: %w", groupID, err)
	}

	// Build keys map for all members we have as contacts.
	contacts := make(map[peer.ID]protocol.ContactKeys)
	for _, memberID := range group.Members {
		if memberID == n.Identity.PeerID {
			continue
		}
		c, err := n.Store.GetContact(memberID)
		if err != nil {
			n.Log.Warn("group member not in contacts, skipping",
				zap.String("member", memberID.String()))
			continue
		}
		if len(c.PublicKey) == 32 {
			var key [32]byte
			copy(key[:], c.PublicKey)
			contacts[memberID] = protocol.ContactKeys{
				BoxPubKey: key,
				PQCPubKey: c.PQCPubKey,
			}
		}
	}

	return n.Group.Send(ctx, group, plaintext, contacts)
}

// AddContact saves a contact with their Curve25519 public key.
func (n *Node) AddContact(id peer.ID, alias string, boxPubKey [32]byte) error {
	c := &types.Contact{
		PeerID:    id,
		Alias:     alias,
		PublicKey: boxPubKey[:],
	}
	return n.Store.SaveContact(c)
}

// Connect dials a remote peer by multiaddr string.
func (n *Node) Connect(ctx context.Context, addrStr string) error {
	ma, err := multiaddr.NewMultiaddr(addrStr)
	if err != nil {
		return fmt.Errorf("parse multiaddr: %w", err)
	}
	ai, err := peer.AddrInfoFromP2pAddr(ma)
	if err != nil {
		return fmt.Errorf("parse peer addr info: %w", err)
	}
	return n.Host.Connect(ctx, *ai)
}

// Addrs returns the node's listen addresses as strings.
func (n *Node) Addrs() []string {
	var addrs []string
	for _, ma := range n.Host.Addrs() {
		addrs = append(addrs, ma.String()+"/p2p/"+n.Host.ID().String())
	}
	return addrs
}

// SetRelayURL configures the push notification relay.
func (n *Node) SetRelayURL(url string) {
	if url != "" {
		n.Relay = relay.New(url)
	}
}

// RegisterPushToken registers this device's push token with the relay.
func (n *Node) RegisterPushToken(ctx context.Context, peerID, token, platform string) error {
	if n.Relay == nil {
		return fmt.Errorf("relay not configured")
	}
	return n.Relay.Register(ctx, peerID, token, platform)
}

// Close shuts down the node gracefully.
func (n *Node) Close() error {
	n.cancel()
	if err := n.DHT.Close(); err != nil {
		n.Log.Warn("DHT close error", zap.Error(err))
	}
	return n.Host.Close()
}
