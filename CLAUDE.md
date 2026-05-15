# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
make setup           # First-time: install mise toolchain + go mod tidy/download
make build           # Compile to bin/indra
make test            # Run all tests with race detector (2-minute timeout)
make run             # Build and run with --debug
make lint            # golangci-lint
make relay           # Compile relay server to bin/relay
make docker-up       # Start 3-node Docker Compose cluster + relay
make docker-down     # Stop cluster
make mobile-setup    # Install gomobile + gobind
make mobile-android  # Build mobile/build/indra.aar
make mobile-ios      # Build mobile/build/Indra.xcframework
```

Go is managed via [mise](https://mise.jdx.dev/) (`.mise.toml` pins Go 1.25). The Makefile resolves `go` through `mise which go` so it works with or without direnv active.

**GOROOT caveat**: mise may leave a stale GOROOT in the environment. When running Go commands manually, set `GOROOT` explicitly:
```bash
GOROOT=/Users/aryaman/.local/share/mise/installs/go/1.25.9 $(mise which go) test -race ./internal/protocol/...
```

**BadgerDB opens an exclusive lock** — only one process can open the same data directory at a time. The `whoami` and `add-contact` subcommands must complete before starting the node.

## Local two-node test

```bash
# Terminal 1
./bin/alice.sh

# Terminal 2
./bin/bob.sh
```

Alice writes her peer ID to `/tmp/indra-alice-id.txt`; Bob polls for it, then adds Alice as a contact and bootstraps to her address.

## Architecture

```
cmd/indra/main.go          CLI entry point (Cobra): run, whoami, add-contact, group create
pkg/types/types.go         Shared types: Message, Contact, Conversation, enums
internal/identity/         Ed25519 keypair + ML-KEM-768 keygen/load; Ed25519→Curve25519 conversion
internal/store/            BadgerDB wrapper: identity, contacts, messages, groups, nonces, PQC seeds
internal/crypto/           NaCl box + hybrid PQC (ML-KEM-768+NaCl) encrypt/decrypt
internal/protocol/         Stream handlers: dm.go (/indra/dm/1.0.0), group.go (/indra/group/1.0.0)
internal/protocol/pb/      Hand-written wire types (length-prefixed JSON, NOT protoc)
internal/discovery/        Kademlia DHT routing + mDNS peer discovery
internal/mailbox/          DHT store-and-forward for offline peers
internal/relay/            Push notification relay client (HTTP)
internal/node/             Wires everything together; SendMessage, SendGroupMessage, relay notify
internal/tui/              Bubble Tea TUI: app.go, contacts.go, chat.go, compose.go, styles.go
relay/                     Push notification relay server (standalone Go binary, separate go.mod)
mobile/                    gomobile-compatible API for Android/iOS (Client struct, JSON I/O)
app/                       React Native iOS app
app/ios/IndraApp/          Native Swift bridge (IndraModule.swift, AppDelegate.swift)
app/src/native/            TypeScript bridge (IndraModule.ts)
app/src/screens/           UI screens (ConversationList, Chat, AddContact, Settings)
```

### Data flow — sending a message

1. TUI `compose.go` fires `SendMsg` → routes to `node.SendMessage` (DM) or `node.SendGroupMessage` (group)
2. `node.SendMessage` fetches the contact's Curve25519 + PQC pubkeys from BadgerDB
3. Tries `protocol.DMHandler.Send` (direct libp2p stream, 10 s timeout)
4. On failure, falls back to `mailbox.PutOfflinePlaintext` (DHT store-and-forward)
5. After mailbox put, notifies the push relay (`POST /notify`) to send a silent push
6. Inbound messages arrive on `node.InboundMessages chan types.Message` → TUI/mobile polls
7. Offline mailbox envelopes are delivered via `DMHandler.DeliverEnvelope` (same decrypt/verify/save path)

### Mobile API (`mobile/indra.go`)

The `mobile` package wraps the full stack behind a `Client` struct with gomobile-compatible types (primitives, strings, `[]byte`, interfaces). Complex data (conversations, messages) is returned as JSON strings. Inbound messages are pushed via the `InboundHandler` callback interface.

Key methods: `Start`, `Stop`, `Whoami` (JSON), `SendMessage`, `ParseAndAddContact`, `SetRelayURL`, `RegisterPushToken`, `FetchMailbox`.

### Encryption

- Identity keys: Ed25519 (libp2p standard)
- Classical encryption: NaCl box (Curve25519 + XSalsa20-Poly1305) — `CryptoVersion=0`
- Hybrid PQC encryption: ML-KEM-768 KEM + NaCl DH → HKDF-SHA256 → secretbox — `CryptoVersion=2`
- Key conversion: Ed25519 seed → SHA-512 → clamp → edwards→Montgomery birational map (`filippo.io/edwards25519`)
- Envelope authentication: Ed25519 signature over `(messageID || ciphertext || nonce)`
- Automatic upgrade: sender uses hybrid when contact has `PQCPubKey`; silent fallback to NaCl otherwise

**Key subtlety**: hybrid mode uses `secretbox.Seal/Open` (symmetric), NOT `box.Seal` (which does its own DH). The symmetric key is derived via HKDF from the concatenation of classical DH shared secret and KEM shared secret.

### Storage key layout (BadgerDB)

```
identity:privkey            — marshalled libp2p private key
identity:boxpubkey          — Curve25519 public key bytes
identity:pqc_decap_seed     — 64-byte ML-KEM-768 seed
contact:<peerID>            — JSON-encoded Contact (includes PQCPubKey if known)
msg:<convID>:<ts_ns>:<id>   — JSON-encoded Message (prefix-ordered for range scans)
group:<groupID>             — JSON-encoded Group
nonce:<convID>              — 8-byte big-endian counter
delivered:<msgID>           — tombstone for deduplication
```

### Push notification relay

The relay (`relay/`) is a standalone Go binary (separate `go.mod`) that maps `peerID → push token`. It stores **zero message content**. Flow:

1. Device registers push token with relay on launch (`POST /register`)
2. When sender stores a message in the DHT mailbox, it POSTs `{from, to}` to relay (`POST /notify`)
3. Relay sends a silent APNs push (`content-available: 1`) to wake the recipient
4. Device wakes, calls `FetchMailbox()`, retrieves and decrypts the message from DHT

### TUI focus model

`internal/tui/app.go` maintains a `focus` value (`focusContacts` / `focusCompose`). Key events are routed **exclusively** to the focused component — Tab toggles focus. This prevents the contacts list from consuming Enter when the user is typing a message.

### Wire format

`internal/protocol/pb/messages.pb.go` is hand-written (not protoc-generated). Messages are length-prefixed JSON. The `.proto` file is kept for documentation only; running `make proto` would overwrite this file. The `Envelope` struct includes `CryptoVersion` and `PQCKEMCiphertext` fields for hybrid encryption.

## Key gotchas

- `dht.DefaultBootstrapPeers` is `[]multiaddr.Multiaddr`, not `[]peer.AddrInfo` — convert via `peer.AddrInfoFromP2pAddr` before connecting.
- Tests that create a libp2p host must pass `libp2p.Identity(id.PrivKey)` so `host.ID() == id.PeerID`; otherwise `ConversationID` won't match between sender and receiver.
- `ConversationID(a, b)` sorts the two peer IDs lexicographically to produce a stable key regardless of direction.
- `Whoami()` returns JSON: `{"peer_id":..., "box_pubkey":..., "pqc_pubkey":...}`. The CLI's `whoami --json` flag outputs the same format.
- `crypto/mlkem` and `crypto/hkdf` require Go 1.25+ (stdlib). GOROOT must point to the 1.25 install.
- `hkdf.Key()` in Go 1.25 takes `info string`, not `info []byte`.
