#!/usr/bin/env bash
set -e
cd "$(dirname "$0")"

if [ ! -f config.json ]; then
  echo "[*] config.json not found, copying from example."
  cp config.example.json config.json
fi

if [ ! -f go.mod ]; then
  echo "[!] go.mod missing. Abort."
  exit 1
fi

echo "[*] go mod tidy"
go mod tidy

echo "[*] Building universal video proxy..."
go build -o proxy-server .

echo "[*] Starting..."
exec ./proxy-server -config config.json