#!/usr/bin/env bash
set -euo pipefail

# Local verifier for consul-dataplane FIPS 140-3 migration.
# It validates:
# 1) unit tests pass
# 2) linux fips binaries compile for amd64 + arm64
# 3) binary metadata includes GOFIPS140=v1.0.0 and fips tag
# 4) fips docker image builds locally
# 5) --version output includes +fips1403

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

BIN_NAME="consul-dataplane"
DIST_AMD64="dist/linux/amd64"
DIST_ARM64="dist/linux/arm64"
BIN_AMD64="$DIST_AMD64/$BIN_NAME"
GOFIPS140_VERSION="${GOFIPS140_VERSION:-v1.0.0}"
ENVOY_FIPS_SUFFIX="${ENVOY_FIPS_SUFFIX:-fips1402}"
IMAGE_TAG="${IMAGE_TAG:-consul-dataplane-fips-local}"
GO_VERSION="$(cat .go-version)"

info() {
  echo "[verify-fips1403] $*"
}

fail() {
  echo "[verify-fips1403] ERROR: $*" >&2
  exit 1
}

assert_contains() {
  local haystack="$1"
  local needle="$2"
  if ! grep -Fq "$needle" <<<"$haystack"; then
    fail "Expected to find '$needle'"
  fi
}

info "Running unit tests"
go test ./...

info "Building linux/amd64 FIPS binary"
mkdir -p "$DIST_AMD64"
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 GOFIPS140="$GOFIPS140_VERSION" \
  go build -tags=fips -trimpath -buildvcs=false -o "$BIN_AMD64" ./cmd/$BIN_NAME

info "Building linux/arm64 FIPS binary"
mkdir -p "$DIST_ARM64"
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 GOFIPS140="$GOFIPS140_VERSION" \
  go build -tags=fips -trimpath -buildvcs=false -o "$DIST_ARM64/$BIN_NAME" ./cmd/$BIN_NAME

info "Checking build metadata on linux/amd64 binary"
METADATA="$(go version -m "$BIN_AMD64")"
assert_contains "$METADATA" "GOFIPS140=$GOFIPS140_VERSION"
assert_contains "$METADATA" "-tags=fips"

info "Building local FIPS container image ($IMAGE_TAG)"
docker build \
  --target release-fips-default \
  --platform linux/amd64 \
  --build-arg BIN_NAME="$BIN_NAME" \
  --build-arg PRODUCT_VERSION="local" \
  --build-arg PRODUCT_REVISION="dev" \
  --build-arg GOLANG_VERSION="$GO_VERSION" \
  --build-arg ENVOY_FIPS_SUFFIX="$ENVOY_FIPS_SUFFIX" \
  -t "$IMAGE_TAG" .

info "Checking --version output from container"
VERSION_OUTPUT="$(docker run --rm "$IMAGE_TAG" --version)"
assert_contains "$VERSION_OUTPUT" "+fips1403"

info "All local FIPS 140-3 verification checks passed"
