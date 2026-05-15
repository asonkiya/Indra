# Testing Indra — Two Machine Setup

## Prerequisites

Both machines need Go 1.25+ via mise and the binary built from source.

```bash
brew install mise
git clone https://github.com/aryaman/indra && cd indra
mise trust && make setup && make build
```

---

## Step 1 — Get your identity

Run this on **each machine** to generate (or load) your identity:

```bash
./bin/indra whoami
```

Output:
```
Peer ID:         12D3KooW...
Box pubkey:      a3f8c2...
PQC pubkey:      7b2e91...
```

For JSON output (used by the mobile app's QR code exchange):
```bash
./bin/indra whoami --json
```

Send your identity info to the other person (text, email, anything).

---

## Step 2 — Add each other as contacts

On **Machine A**, paste Machine B's values:
```bash
./bin/indra add-contact <B_PEER_ID> <B_BOX_PUBKEY> --alias bob
```

On **Machine B**, paste Machine A's values:
```bash
./bin/indra add-contact <A_PEER_ID> <A_BOX_PUBKEY> --alias alice
```

You should see: `Contact saved: bob (12D3KooW...)`

---

## Step 3 — Start the nodes

**Machine A** goes first. Note your public IP address (`curl ifconfig.me` if unsure).

```bash
./bin/indra
```

It will print something like:
```
Indra node running.
Peer ID: 12D3KooWxxx
Addresses:
  /ip4/1.2.3.4/tcp/4001/p2p/12D3KooWxxx
```

> Make sure **port 4001 TCP and UDP** is open on Machine A's firewall or router.

**Machine B** connects to A using that address:
```bash
./bin/indra --bootstrap /ip4/<MACHINE_A_PUBLIC_IP>/tcp/4001/p2p/<A_PEER_ID>
```

---

## Step 4 — Send messages

Both terminals open the Indra TUI. You should see your contact in the left panel.

- **Arrow keys** — navigate the contacts list
- **Enter** — open a conversation
- **Type** — compose a message
- **Enter** — send
- **Ctrl+C** — quit

Messages are encrypted with hybrid PQC (ML-KEM-768 + NaCl) when both peers have PQC keys (default for new identities).

---

## Mobile testing (iOS simulators)

```bash
make mobile-ios              # build xcframework
cd app && npm install
cd app/ios && pod install
./bin/sim2.sh                # launches two simulators with the app
```

In the app:
1. Open **Settings** on Simulator A — copy the Whoami JSON
2. On Simulator B, tap **Add Contact** → paste the JSON → **Add Contact**
3. Repeat in reverse
4. Send messages between the two simulators

---

## Troubleshooting

**Machine B can't connect to A**
- Confirm port 4001 is open: `nc -zv <MACHINE_A_IP> 4001`
- Try the QUIC address instead: `--bootstrap /ip4/<IP>/udp/4001/quic-v1/p2p/<PEER_ID>`

**Both machines are behind NAT**
Use a random port and let libp2p attempt hole-punching:
```bash
./bin/indra --listen /ip4/0.0.0.0/tcp/0,/ip4/0.0.0.0/udp/0/quic-v1
```
If hole-punching fails, one person needs to temporarily open port 4001.

**Contact doesn't appear in TUI**
Make sure you ran `add-contact` before starting the node — contacts are loaded at startup.

**Debug mode**
Add `--debug` to either node to see verbose logs including DHT activity, connection attempts, and message delivery:
```bash
./bin/indra --debug
```
