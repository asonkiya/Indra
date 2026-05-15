package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	bubbletea "github.com/charmbracelet/bubbletea"
	libp2ppeer "github.com/libp2p/go-libp2p/core/peer"
	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/aryaman/indra/internal/identity"
	"github.com/aryaman/indra/internal/node"
	"github.com/aryaman/indra/internal/store"
	indratui "github.com/aryaman/indra/internal/tui"
	indratypes "github.com/aryaman/indra/pkg/types"
)

func main() {
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	var (
		listenAddrs    []string
		dataDir        string
		bootstrapPeers []string
		debug          bool
	)

	cmd := &cobra.Command{
		Use:   "indra",
		Short: "Indra — decentralized P2P messaging",
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(listenAddrs, dataDir, bootstrapPeers, debug)
		},
	}

	home, _ := os.UserHomeDir()
	defaultData := filepath.Join(home, ".config", "indra")

	cmd.Flags().StringSliceVar(&listenAddrs, "listen", []string{
		"/ip4/0.0.0.0/tcp/4001",
		"/ip4/0.0.0.0/udp/4001/quic-v1",
	}, "Multiaddrs to listen on")
	cmd.Flags().StringVar(&dataDir, "data", defaultData, "Data directory")
	cmd.Flags().StringSliceVar(&bootstrapPeers, "bootstrap", nil, "Extra bootstrap peer multiaddrs")
	cmd.Flags().BoolVar(&debug, "debug", false, "Enable debug logging")

	// whoami — print this node's peer ID and Curve25519 pubkey for sharing.
	cmd.AddCommand(whoamiCmd(defaultData))

	// add-contact — save a peer's credentials before starting the node.
	cmd.AddCommand(addContactCmd(defaultData))

	// group — group chat management.
	cmd.AddCommand(groupCmd(defaultData))

	return cmd
}

func whoamiCmd(defaultData string) *cobra.Command {
	var dataDir string
	var jsonOutput bool
	c := &cobra.Command{
		Use:   "whoami",
		Short: "Print your peer ID and public keys for sharing with contacts",
		RunE: func(cmd *cobra.Command, args []string) error {
			log := zap.NewNop()
			if err := os.MkdirAll(dataDir, 0700); err != nil {
				return err
			}
			st, err := store.Open(filepath.Join(dataDir, "db"), log)
			if err != nil {
				return err
			}
			defer st.Close()

			id, err := identity.Load(st)
			if err != nil {
				return err
			}

			pqcPubkey := ""
			if id.PQCDecapKey != nil {
				pqcPubkey = hex.EncodeToString(id.PQCDecapKey.EncapsulationKey().Bytes())
			}

			if jsonOutput {
				// Machine-readable JSON — same format as mobile Whoami().
				w := struct {
					PeerID    string `json:"peer_id"`
					BoxPubkey string `json:"box_pubkey"`
					PQCPubkey string `json:"pqc_pubkey"`
				}{
					PeerID:    id.PeerID.String(),
					BoxPubkey: hex.EncodeToString(id.BoxPubKey[:]),
					PQCPubkey: pqcPubkey,
				}
				data, _ := json.Marshal(w)
				fmt.Println(string(data))
				return nil
			}

			fmt.Println("Peer ID:        ", id.PeerID)
			fmt.Println("Box pubkey:     ", hex.EncodeToString(id.BoxPubKey[:]))
			if pqcPubkey != "" {
				fmt.Println("PQC pubkey:     ", pqcPubkey[:32]+"...")
			}
			fmt.Println()
			fmt.Println("Share with contacts, or use --json for machine-readable output.")
			return nil
		},
	}
	c.Flags().StringVar(&dataDir, "data", defaultData, "Data directory")
	c.Flags().BoolVar(&jsonOutput, "json", false, "Output as JSON (for QR codes / contact exchange)")
	return c
}

func addContactCmd(defaultData string) *cobra.Command {
	var dataDir, alias string
	c := &cobra.Command{
		Use:   "add-contact <peerID> <box-pubkey-hex>",
		Short: "Save a contact's peer ID and public key",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			peerIDStr, pubkeyHex := args[0], args[1]

			peerID, err := libp2ppeer.Decode(peerIDStr)
			if err != nil {
				return fmt.Errorf("invalid peer ID: %w", err)
			}

			pubkeyBytes, err := hex.DecodeString(pubkeyHex)
			if err != nil {
				return fmt.Errorf("invalid pubkey hex: %w", err)
			}
			if len(pubkeyBytes) != 32 {
				return fmt.Errorf("pubkey must be 32 bytes (64 hex chars), got %d bytes", len(pubkeyBytes))
			}

			log := zap.NewNop()
			if err := os.MkdirAll(dataDir, 0700); err != nil {
				return err
			}
			st, err := store.Open(filepath.Join(dataDir, "db"), log)
			if err != nil {
				return err
			}
			defer st.Close()

			if alias == "" {
				alias = peerIDStr[:12]
			}

			contact := &indratypes.Contact{
				PeerID:    peerID,
				Alias:     alias,
				PublicKey: pubkeyBytes,
				AddedAt:   time.Now(),
			}
			if err := st.SaveContact(contact); err != nil {
				return fmt.Errorf("save contact: %w", err)
			}

			fmt.Printf("Contact saved: %s (%s)\n", alias, peerIDStr)
			return nil
		},
	}
	c.Flags().StringVar(&dataDir, "data", defaultData, "Data directory")
	c.Flags().StringVar(&alias, "alias", "", "Human-readable name (default: first 12 chars of peer ID)")
	return c
}

func groupCmd(defaultData string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "group",
		Short: "Group chat management",
	}

	cmd.AddCommand(groupCreateCmd(defaultData))
	return cmd
}

func groupCreateCmd(defaultData string) *cobra.Command {
	var dataDir string
	c := &cobra.Command{
		Use:   "create <name> <peerID>...",
		Short: "Create a group with the given members",
		Args:  cobra.MinimumNArgs(2), // name + at least one peer
		RunE: func(cmd *cobra.Command, args []string) error {
			groupName := args[0]
			peerStrs := args[1:]

			log := zap.NewNop()
			if err := os.MkdirAll(dataDir, 0700); err != nil {
				return err
			}
			st, err := store.Open(filepath.Join(dataDir, "db"), log)
			if err != nil {
				return err
			}
			defer st.Close()

			id, err := identity.Load(st)
			if err != nil {
				return err
			}

			members := []libp2ppeer.ID{id.PeerID} // include ourselves
			for _, ps := range peerStrs {
				pid, err := libp2ppeer.Decode(ps)
				if err != nil {
					return fmt.Errorf("invalid peer ID %q: %w", ps, err)
				}
				members = append(members, pid)
			}

			group := &indratypes.Group{
				ID:        fmt.Sprintf("grp:%s:%d", groupName, time.Now().UnixNano()),
				Name:      groupName,
				Members:   members,
				CreatorID: id.PeerID,
				CreatedAt: time.Now(),
			}

			if err := st.SaveGroup(group); err != nil {
				return fmt.Errorf("save group: %w", err)
			}

			fmt.Printf("Group created: %s\n", group.Name)
			fmt.Printf("Group ID:      %s\n", group.ID)
			fmt.Printf("Members:       %d\n", len(members))
			return nil
		},
	}
	c.Flags().StringVar(&dataDir, "data", defaultData, "Data directory")
	return c
}

func run(listenAddrs []string, dataDir string, bootstrapPeers []string, debug bool) error {
	log := newLogger(debug)
	defer log.Sync() //nolint:errcheck

	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	// Open storage.
	st, err := store.Open(filepath.Join(dataDir, "db"), log)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer st.Close()

	// Load or generate identity.
	id, err := identity.Load(st)
	if err != nil {
		return fmt.Errorf("load identity: %w", err)
	}

	log.Info("identity loaded",
		zap.String("peerID", id.PeerID.String()),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the node.
	cfg := node.Config{
		ListenAddrs:    listenAddrs,
		BootstrapPeers: bootstrapPeers,
		DataDir:        dataDir,
	}
	n, err := node.New(ctx, id, st, cfg, log)
	if err != nil {
		return fmt.Errorf("start node: %w", err)
	}
	defer n.Close()

	fmt.Println("Indra node running.")
	fmt.Println("Peer ID:", id.PeerID)
	fmt.Println("Addresses:")
	for _, a := range n.Addrs() {
		fmt.Println(" ", a)
	}
	fmt.Println()

	// Load existing conversations for the TUI.
	convos := buildConversations(st, id.PeerID)

	// Build the TUI send functions.
	sendFn := func(ctx context.Context, recipientID libp2ppeer.ID, text []byte) error {
		_, err := n.SendMessage(ctx, recipientID, text)
		return err
	}
	groupSendFn := func(ctx context.Context, groupID string, text []byte) error {
		return n.SendGroupMessage(ctx, groupID, text)
	}

	app := indratui.NewApp(id.PeerID.String(), convos, n.InboundMessages, sendFn, groupSendFn)
	p := bubbletea.NewProgram(app, bubbletea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		log.Error("TUI error", zap.Error(err))
	}

	return nil
}

func buildConversations(st *store.Store, myID libp2ppeer.ID) []*indratypes.Conversation {
	contacts, err := st.ListContacts()
	if err != nil {
		return nil
	}
	convos := make([]*indratypes.Conversation, 0, len(contacts))
	for _, c := range contacts {
		convID := indratypes.ConversationID(myID, c.PeerID)
		msgs, _ := st.ListMessages(convID, 100, time.Time{})
		convos = append(convos, &indratypes.Conversation{
			ID:           convID,
			Name:         c.Alias,
			Participants: []libp2ppeer.ID{myID, c.PeerID},
			Messages:     msgs,
		})
	}

	// Load group conversations.
	groups, err := st.ListGroups()
	if err == nil {
		for _, g := range groups {
			msgs, _ := st.ListMessages(g.ID, 100, time.Time{})
			convos = append(convos, &indratypes.Conversation{
				ID:           g.ID,
				IsGroup:      true,
				Name:         g.Name,
				Participants: g.Members,
				Messages:     msgs,
			})
		}
	}

	return convos
}

func newLogger(debug bool) *zap.Logger {
	var cfg zap.Config
	if debug {
		cfg = zap.NewDevelopmentConfig()
	} else {
		cfg = zap.NewProductionConfig()
	}
	log, err := cfg.Build()
	if err != nil {
		panic(err)
	}
	return log
}
