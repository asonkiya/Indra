#!/usr/bin/env bash
set -e
cd "$(dirname "$0")/.."

rm -rf /tmp/indra-alice /tmp/indra-alice-id.txt /tmp/indra-bob-id.txt

# Generate alice's identity and save it
./bin/indra whoami --data /tmp/indra-alice > /tmp/indra-alice-id.txt
cat /tmp/indra-alice-id.txt

echo "Waiting for bob to generate his identity..."
for i in $(seq 1 30); do
  [ -f /tmp/indra-bob-id.txt ] && break
  sleep 1
done

if [ ! -f /tmp/indra-bob-id.txt ]; then
  echo "Timed out waiting for bob. Run bob.sh in another tab first or in parallel."
  exit 1
fi

BOB_ID=$(awk '/Peer ID/{print $3}' /tmp/indra-bob-id.txt)
BOB_KEY=$(awk '/Box pubkey/{print $3}' /tmp/indra-bob-id.txt)
./bin/indra add-contact "$BOB_ID" "$BOB_KEY" --alias bob --data /tmp/indra-alice
echo "Added bob as contact. Starting node..."

./bin/indra --data /tmp/indra-alice
