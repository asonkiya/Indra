# Indra

A fully decentralized, end-to-end encrypted messenger. No servers. No phone numbers. No accounts.

Every device is a peer. You add contacts by scanning a QR code. Messages travel directly between devices over libp2p — the same stack behind IPFS. If the recipient is offline, the encrypted message is stored in a distributed hash table until they come back online; a silent push wakes their phone to pick it up. No central server ever touches your data.

Indra is the only open-source P2P messenger with **hybrid post-quantum encryption**. Every message is protected by both classical cryptography (NaCl box / Curve25519) and a NIST-standardized post-quantum algorithm (ML-KEM-768 / FIPS 203), making messages secure against both today's attacks and future quantum computers.

```
Alice ──── DHT ──── Bob
  \                  /
   └── Charlie ─────
   (each node routes & stores)
```

## Status

| Component | Status |
|---|---|
| Ed25519 identity + Curve25519 key derivation | Done |
| Hybrid PQC encryption (ML-KEM-768 + NaCl) | Done |
| BadgerDB local storage (messages, contacts, groups) | Done |
| libp2p host (TCP + QUIC, Noise, Yamux, NAT traversal) | Done |
| Kademlia DHT peer discovery + mDNS | Done |
| Direct messaging (`/indra/dm/1.0.0`) | Done |
| Ed25519 message signatures + verification | Done |
| Offline delivery via DHT mailbox | Done |
| Group chat fan-out (`/indra/group/1.0.0`) | Done |
| Bubble Tea TUI | Done |
| gomobile API layer (`mobile/`) | Done |
| React Native iOS app | Done |
| QR code / JSON contact exchange | Done |
| Push notification relay | Done |
| File sharing | Planned |
| Voice / video | Planned |
| Forward secrecy (Double Ratchet) | Planned |

## Quick start

**Prerequisites**: [mise](https://mise.jdx.dev/)

```bash
brew install mise

git clone https://github.com/aryaman/indra
cd indra
mise trust
make setup     # installs Go 1.25, downloads dependencies
make build     # compiles to bin/indra
./bin/indra    # starts the node + TUI
```

On first run, a fresh Ed25519 identity and ML-KEM-768 keypair are generated and saved. Your peer ID is derived from your public key — it's stable across restarts.

## Usage

```
./bin/indra [flags]

Flags:
  --listen     Multiaddrs to listen on (default: TCP 4001 + QUIC 4001)
  --data       Data directory (default: ~/.config/indra)
  --bootstrap  Extra bootstrap peer multiaddrs
  --debug      Enable verbose logging
```

### Adding a contact

```bash
# Print your identity for sharing
./bin/indra whoami --data /tmp/alice

# JSON output (for QR codes / automated exchange)
./bin/indra whoami --json --data /tmp/alice

# Add a contact using their peer ID and box pubkey
./bin/indra add-contact <PEER_ID> <BOX_PUBKEY_HEX> --alias bob --data /tmp/alice
```

### Running a local two-node test

```bash
# Terminal 1
./bin/alice.sh

# Terminal 2
./bin/bob.sh
```

## Mobile app

The React Native iOS app lives in `app/`. It uses a gomobile-generated xcframework that wraps the full Go stack.

```bash
make mobile-setup  # install gomobile + gobind
make mobile-ios    # build Indra.xcframework
cd app && npm install
cd app/ios && pod install
# Open app/ios/IndraApp.xcworkspace in Xcode
```

Two-simulator test: `./bin/sim2.sh`

## Push relay

The push notification relay (`relay/`) sends silent APNs/FCM pushes to wake offline devices. It stores **zero message content** — only maps `peerID -> push token`.

```bash
make relay                    # build to bin/relay
./bin/relay --apns-p8 key.p8  # see relay/.env.example for config
```

## Docker

```bash
cp .env.example .env
docker compose up bootstrap    # boot the bootstrap node, copy its peer ID into .env
docker compose up --build -d   # bring all nodes + relay up
```

## Architecture

```
cmd/indra/              CLI entry point (Cobra: run, whoami, add-contact, group create)
internal/
  identity/             Ed25519 keypair + ML-KEM-768 keygen/load + Curve25519 conversion
  node/                 libp2p host, DHT, SendMessage, SendGroupMessage, relay integration
  crypto/               NaCl box + hybrid PQC encrypt/decrypt, nonce management
  protocol/
    dm.go               /indra/dm/1.0.0 stream handler + DeliverEnvelope
    group.go            /indra/group/1.0.0 group fan-out handler
    pb/                 Wire format (length-prefixed JSON; .proto for schema)
  store/                BadgerDB: messages, contacts, groups, nonce counters, PQC seeds
  mailbox/              DHT store-and-forward for offline delivery
  relay/                Push notification relay client
  discovery/            DHT routing discovery + mDNS
  tui/                  Bubble Tea TUI (DM + group chat)
relay/                  Push notification relay server (standalone binary)
mobile/                 gomobile-compatible API (Android .aar / iOS .xcframework)
app/                    React Native iOS app
pkg/types/              Shared types: Message, Contact, Conversation, Group
```

### Encryption

| Layer | Algorithm | Purpose |
|---|---|---|
| Identity | Ed25519 | Peer authentication, message signing |
| Classical | NaCl box (Curve25519 + XSalsa20-Poly1305) | End-to-end encryption |
| Post-quantum | ML-KEM-768 (NIST FIPS 203) | Quantum-safe key encapsulation |
| Hybrid | HKDF-SHA256(DH shared \|\| KEM shared) -> secretbox | Combined classical + PQ security |
| Transport | libp2p Noise | Authenticated stream encryption |

The hybrid construction (`CryptoVersion=2`) automatically activates when both peers have PQC keys. Legacy peers (`CryptoVersion=0`) use NaCl-only encryption with silent fallback.

### Security model

| Threat | Mitigation |
|---|---|
| Harvest-now-decrypt-later (quantum) | ML-KEM-768 hybrid encryption |
| DHT node reads offline messages | NaCl box — only the recipient's private key can decrypt |
| Man-in-the-middle on stream | libp2p Noise protocol (authenticated key exchange) |
| Sender impersonation | Ed25519 signature over `(messageID \|\| ciphertext \|\| nonce)` |
| Replay attacks | UUID + delivered-tombstone in BadgerDB |
| Push relay reads content | Relay only receives `{from, to}` — zero message content |

## Development

```bash
make build           # compile CLI to bin/indra
make test            # go test -race ./...
make lint            # golangci-lint
make relay           # compile relay to bin/relay
make mobile-setup    # install gomobile + gobind
make mobile-ios      # build Indra.xcframework
make mobile-android  # build indra.aar
make docker-up       # start 3-node cluster + relay
make docker-down     # stop cluster
```

## License

MIT — see [LICENSE](LICENSE).
