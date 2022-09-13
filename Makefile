SHELL := /usr/bin/env bash -euo pipefail -c

REPO_NAME    ?= $(shell basename "$(CURDIR)")
PRODUCT_NAME ?= $(REPO_NAME)
BIN_NAME     ?= $(PRODUCT_NAME)

# Get local ARCH; on Intel Mac, 'uname -m' returns x86_64 which we turn into amd64.
# Not using 'go env GOOS/GOARCH' here so 'make docker' will work without local Go install.
ARCH     = $(shell A=$$(uname -m); [ $$A = x86_64 ] && A=amd64; echo $$A)
OS       = $(shell uname | tr [[:upper:]] [[:lower:]])
PLATFORM = $(OS)/$(ARCH)
DIST     = dist/$(PLATFORM)
BIN      = $(DIST)/$(BIN_NAME)

VERSION = $(shell ./build-scripts/version.sh pkg/version/version.go)

# Get latest revision (no dirty check for now).
REVISION = $(shell git rev-parse HEAD)

.PHONY: version
version:
	@echo $(VERSION)

dist:
	mkdir -p $(DIST)
	echo '*' > dist/.gitignore

.PHONY: bin
bin: dist
	GOARCH=$(ARCH) GOOS=$(OS) CGO_ENABLED=0 go build -trimpath -buildvcs=false -o $(BIN) ./cmd/$(BIN_NAME)

# Docker Stuff.
export DOCKER_BUILDKIT=1
BUILD_ARGS = BIN_NAME=$(BIN_NAME) PRODUCT_VERSION=$(VERSION) PRODUCT_REVISION=$(REVISION)
TAG        = $(PRODUCT_NAME)/$(TARGET):$(VERSION)
BA_FLAGS   = $(addprefix --build-arg=,$(BUILD_ARGS))
FLAGS      = --target $(TARGET) --platform $(PLATFORM) --tag $(TAG) $(BA_FLAGS)

# Set OS to linux for all docker/* targets.
docker/%: OS = linux

# DOCKER_TARGET is a macro that generates the build and run make targets
# for a given Dockerfile target.
# Args: 1) Dockerfile target name (required).
#       2) Build prerequisites (optional).
define DOCKER_TARGET
.PHONY: docker/$(1)
docker/$(1): TARGET=$(1)
docker/$(1): $(2)
	docker build $$(FLAGS) .
	@echo 'Image built; run "docker run --rm $$(TAG)" to try it out.'

.PHONY: docker/$(1)/run
docker/$(1)/run: TARGET=$(1)
docker/$(1)/run: docker/$(1)
	docker run --rm $$(TAG)
endef

# Create docker/<target>[/run] targets.
$(eval $(call DOCKER_TARGET,release-default,bin))
$(eval $(call DOCKER_TARGET,release-ubi,bin))

.PHONY: docker
docker: docker/release-default

BOOTSTRAP_PACKAGE_DIR=internal/bootstrap

.PHONY: copy-bootstrap-config

# Our Envoy bootstrap config generation contains a fair amount of logic that
# was already implemented for the `consul connect envoy` command. Eventually,
# this command will go away and be replaced by consul-dataplane, but for now
# we copy the files from the Consul repo, and do a small amount of processing
# to rename the package and remove a dependency on the Consul api module.
copy-bootstrap-config:
	for file in bootstrap_config.go bootstrap_config_test.go bootstrap_tpl.go; do \
		curl --fail https://raw.githubusercontent.com/hashicorp/consul/main/command/connect/envoy/$$file | \
		sed 's/package envoy/package bootstrap/' | \
		sed '/github.com\/hashicorp\/consul\/api/d' | \
		sed 's/api.IntentionDefaultNamespace/"default"/g' | \
		sed '1s:^:// Code generated by make copy-bootstrap-config. DO NOT EDIT.\n:' | \
		gofmt \
		> $(BOOTSTRAP_PACKAGE_DIR)/$$file; \
	done

.PHONY: unit-tests
unit-tests:
	go test ./...

# TODO: Install dependencies before running this target
.PHONY: consul-proto
consul-proto:
	buf generate "https://github.com/hashicorp/consul.git#branch=main,subdir=proto-public"
