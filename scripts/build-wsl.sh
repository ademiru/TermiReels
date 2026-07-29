#!/usr/bin/env bash
set -euo pipefail

with_creator=false
if [[ "${1:-}" == "--with-creator-provider" ]]; then
  with_creator=true
elif [[ $# -ne 0 ]]; then
  echo "usage: $0 [--with-creator-provider]" >&2
  exit 2
fi

if ! grep -qiE 'microsoft|wsl' /proc/sys/kernel/osrelease 2>/dev/null; then
  echo "This build helper is intended to run inside WSL2." >&2
  exit 1
fi

if ! command -v docker >/dev/null 2>&1; then
  echo "Docker is unavailable. Enable Docker Desktop's WSL integration first." >&2
  exit 1
fi
if ! docker info >/dev/null 2>&1; then
  echo "Docker is installed but its daemon is unavailable. Start Docker Desktop and enable this WSL distribution." >&2
  exit 1
fi

repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_dir"
mkdir -p .build-cache/go-build .build-cache/go-mod

image="termireels-wsl-builder:local"
docker build --file Dockerfile.build --tag "$image" .
docker run --rm \
  --user "$(id -u):$(id -g)" \
  --env GOCACHE=/app/.build-cache/go-build \
  --env GOMODCACHE=/app/.build-cache/go-mod \
  --volume "$repo_dir:/app" \
  --workdir /app \
  "$image" \
  go build -buildvcs=false -o termireels .

if $with_creator; then
  if ! command -v node >/dev/null 2>&1 || ! command -v npm >/dev/null 2>&1; then
    echo "Node.js 20+ and npm are required for --with-creator-provider." >&2
    exit 1
  fi
  node_major="$(node -p 'Number(process.versions.node.split(".")[0])')"
  if (( node_major < 20 )); then
    echo "Node.js 20 or newer is required; found $(node --version)." >&2
    exit 1
  fi
  npm --prefix creator-provider ci
  npm --prefix creator-provider run build
fi

echo
echo "Built: $repo_dir/termireels"
if $with_creator; then
  echo "Run:   ./termireels --creator-provider"
else
  echo "Run:   ./termireels"
fi
