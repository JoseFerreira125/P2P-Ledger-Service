#!/usr/bin/env bash

set -ex # Enable exit on error and print commands

echo "Starting entrypoint.sh for ledger-2/3"
pwd
ls -l

# Wait for ledger-1's HTTP API (internal port 8080) to be available
/root/wait-for-it.sh ledger-1 8080 -- echo "ledger-1 API is up"

echo "Fetching node info from ledger-1..."
NODE_INFO=$(curl -s http://ledger-1:8080/node/info)
echo "RAW NODE_INFO: $NODE_INFO"

PEER_ID=$(echo "$NODE_INFO" | jq -r '.peer_id')

# FIX: Use the /dns4/ protocol which supports hostnames like 'ledger-1'.
# The underlying libp2p stack will resolve this DNS name within the Docker network.
LISTEN_ADDR="/dns4/ledger-1/tcp/4000"

echo "Extracted PEER_ID: $PEER_ID"
echo "Assumed LISTEN_ADDR: $LISTEN_ADDR"

if [ -z "$PEER_ID" ]; then
  echo "Error: Could not retrieve Peer ID from ledger-1. NODE_INFO was: $NODE_INFO" >&2
  exit 1
fi

# Construct the full multiaddress
FULL_MULTIADDR="${LISTEN_ADDR}/p2p/${PEER_ID}"

echo "Constructed FULL_MULTIADDR: $FULL_MULTIADDR"

# Execute the ledger binary with the connect argument
exec ./ledger --connect "$FULL_MULTIADDR"