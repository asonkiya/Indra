#!/usr/bin/env bash
set -e
cd "$(dirname "$0")/.."

rm -rf /tmp/indra-bob

# Generate bob's identity and save it
./bin/indra whoami --data /tmp/indra-bob > /tmp/indra-bob-id.txt
cat /tmp/indra-bob-id.txt

echo "Waiting for alice to generate her identity..."
for i in $(seq 1 30); do
  [ -f /tmp/indra-alice-id.txt ] && break
  sleep 1
done

if [ ! -f /tmp/indra-alice-id.txt ]; then
  echo "Timed out waiting for alice. Run alice.sh in another tab."
  exit 1
fi

ALICE_ID=$(awk '/Peer ID/{print $3}' /tmp/indra-alice-id.txt)
ALICE_KEY=$(awk '/Box pubkey/{print $3}' /tmp/indra-alice-id.txt)
./bin/indra add-contact "$ALICE_ID" "$ALICE_KEY" --alias alice --data /tmp/indra-bob
echo "Added alice as contact. Starting node..."

# Give alice's node a moment to bind port 4001
sleep 2
./bin/indra --data /tmp/indra-bob --bootstrap "/ip4/127.0.0.1/tcp/4001/p2p/$ALICE_ID"
