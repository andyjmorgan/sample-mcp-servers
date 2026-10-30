#!/usr/bin/env bash
# Build and push every server image to the lab registry.
#   ./build.sh            # tag = short git sha
#   TAG=v1 ./build.sh
set -euo pipefail
REGISTRY=${REGISTRY:-192.168.0.140:5555/sample-mcp-servers}
TAG=${TAG:-$(git rev-parse --short HEAD)}
for s in timeserver dateserver uuidserver diceserver countserver; do
  docker build --build-arg SERVER=$s -t "$REGISTRY/$s:$TAG" -t "$REGISTRY/$s:latest" .
  docker push -q "$REGISTRY/$s:$TAG"
  docker push -q "$REGISTRY/$s:latest"
done
