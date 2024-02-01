SHELL := /usr/bin/env bash -euo pipefail -c

PRODUCT_NAME ?= consul-dataplane
BIN_NAME     ?= $(PRODUCT_NAME)
GOPATH       ?= $(shell go env GOPATH)
GOBIN        ?= $(GOPATH)/bin

# Get local ARCH; on Intel Mac, 'uname -m' returns x86_64 which we turn into amd64.
# Not using 'go env GOOS/GOARCH' here so 'make docker' will work without local Go install.
ARCH     ?= $(shell A=$$(uname -m); [ $$A = x86_64 ] && A=amd64; echo $$A)
# Only build for linux so that building the docker image works on M1 Macs.
OS       ?= linux
PLATFORM = $(OS)/$(ARCH)
DIST     = dist/$(PLATFORM)
BIN      = $(DIST)/$(BIN_NAME)

VERSION = $(shell ./build-scripts/version.sh pkg/version/version.go)
GOLANG_VERSION ?= $(shell head -n 1 .go-version)
BOOTSTRAP_PACKAGE_DIR=internal/bootstrap
INTEGRATION_TESTS_SERVER_IMAGE    ?= hashicorppreview/consul:1.15-dev
INTEGRATION_TESTS_DATAPLANE_IMAGE ?= $(PRODUCT_NAME)/release-default:$(VERSION)

GIT_COMMIT?=$(shell git rev-parse --short HEAD)
GIT_DIRTY?=$(shell test -n "`git status --porcelain`" && echo "+CHANGES" || true)
GOLDFLAGS=-X github.com/hashicorp/consul-dataplane/pkg/version.GitCommit=$(GIT_COMMIT)$(GIT_DIRTY)

# Get latest revision (no dirty check for now).
REVISION = $(shell git rev-parse HEAD)

# Docker Stuff.
export DOCKER_BUILDKIT=1
BUILD_ARGS = BIN_NAME=$(BIN_NAME) PRODUCT_VERSION=$(VERSION) PRODUCT_REVISION=$(REVISION) GOLANG_VERSION=$(GOLANG_VERSION)
TAG        = $(PRODUCT_NAME):$(VERSION)
BA_FLAGS   = $(addprefix --build-arg=,$(BUILD_ARGS))
FLAGS      = --target $(TARGET) --platform $(PLATFORM) --tag $(TAG) $(BA_FLAGS)

##@ Build

dist: ## make dist directory and ignore everything
	mkdir -p $(DIST)
	echo '*' > dist/.gitignore

.PHONY: bin
bin: dist ## Build the binary
	GOARCH=$(ARCH) GOOS=$(OS) CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags="$(GOLDFLAGS)" -o $(BIN) ./cmd/$(BIN_NAME)

.PHONY: dev
dev: bin ## Build binary and copy to the destination
	cp $(BIN) $(GOBIN)/$(BIN_NAME)

# DANGER: this target is experimental and could be modified/removed at any time.
.PHONY: skaffold
skaffold: dev ## Build consul-dataplane dev Docker image for use with skaffold or local development.
	@docker build -t '$(DEV_IMAGE)' \
       --build-arg 'GOLANG_VERSION=$(GOLANG_VERSION)' \
       --build-arg 'TARGETARCH=$(ARCH)' \
       -f $(CURDIR)/Dockerfile.dev .

.PHONY: docker
docker: bin ## build the release-target docker image
	$(eval TARGET := release-default) # there are many targets in the Dockerfile, add more build if you need to customize the target
	docker build $(FLAGS) .
	@echo 'Image built; run "docker run --rm $(TAG)" to try it out.'

docker-run: docker ## run the image of $(TAG)
	docker run --rm $(TAG)

.PHONY: dev-docker
dev-docker: docker ## build docker image and tag the image to local
	docker tag '$(PRODUCT_NAME):$(VERSION)'  '$(PRODUCT_NAME):local'

##@ Testing

.PHONY: unit-tests
unit-tests: ## unit tests
	go test ./...

.PHONY: expand-integration-tests-output-dir
expand-integration-tests-output-dir: ## create directory to support integration tests
# make's built-in realpath function doesn't support non-existent directories
# and intermittently has issues finding newly created ones (so preemptively
# creating it with mkdir isn't an option) so we'll rely on the realpath bin.
ifdef INTEGRATION_TESTS_OUTPUT_DIR
ifeq (, $(shell which realpath))
 $(error "GNU Coreutils are required to run the integration-tests target with INTEGRATION_TESTS_OUTPUT_DIR.")
else
EXPANDED_INTEGRATION_TESTS_OUTPUT_DIR = $(shell realpath $(INTEGRATION_TESTS_OUTPUT_DIR))
endif
endif

.PHONY: integration-tests
integration-tests: docker/release-default expand-integration-tests-output-dir ## integration tests
	cd integration-tests && go test -v ./ -output-dir="$(EXPANDED_INTEGRATION_TESTS_OUTPUT_DIR)" -dataplane-image="$(INTEGRATION_TESTS_DATAPLANE_IMAGE)" -server-image="$(INTEGRATION_TESTS_SERVER_IMAGE)"

##@ Release

.PHONY: version
version: ## display version
	@echo $(VERSION)

##@ Tools

# Our Envoy bootstrap config generation contains a fair amount of logic that
# was already implemented for the `consul connect envoy` command. Eventually,
# this command will go away and be replaced by consul-dataplane, but for now
# we copy the files from the Consul repo, and do a small amount of processing
# to rename the package and remove a dependency on the Consul api module.
.PHONY: copy-bootstrap-config
copy-bootstrap-config: ## copy bootstrap config
	for file in bootstrap_config.go bootstrap_config_test.go bootstrap_tpl.go; do \
		curl --fail https://raw.githubusercontent.com/hashicorp/consul/main/command/connect/envoy/$$file | \
		sed 's/package envoy/package bootstrap/' | \
		sed '/github.com\/hashicorp\/consul\/api/d' | \
		sed 's/api.IntentionDefaultNamespace/"default"/g' | \
		sed '1s:^:// Code generated by make copy-bootstrap-config. DO NOT EDIT.\n:' | \
		sed '/"initial_metadata": \[/,/\]/d' | \
		gofmt \
		> $(BOOTSTRAP_PACKAGE_DIR)/$$file; \
	done

.PHONY: changelog
changelog: ## build change log
ifdef DP_LAST_RELEASE_GIT_TAG
	@changelog-build \
		-last-release $(DP_LAST_RELEASE_GIT_TAG) \
		-entries-dir .changelog/ \
		-changelog-template .changelog/changelog.tmpl \
		-note-template .changelog/note.tmpl \
		-this-release $(REVISION)
else
	$(error Cannot generate changelog without DP_LAST_RELEASE_GIT_TAG)
endif

.PHONY: check-env
check-env: ## check env
	@printenv | grep "DP"

.PHONY: prepare-release
prepare-release:
ifndef DP_RELEASE_VERSION
	$(error DP_RELEASE_VERSION is required)
endif
	@$(CURDIR)/build-scripts/prepare-release.sh $(CURDIR)/pkg/version/version.go $(DP_RELEASE_VERSION) ""

.PHONY: prepare-dev
prepare-dev:
ifndef DP_NEXT_RELEASE_VERSION
	$(error DP_NEXT_RELEASE_VERSION is required)
endif
	@$(CURDIR)/build-scripts/prepare-release.sh $(CURDIR)/pkg/version/version.go $(DP_NEXT_RELEASE_VERSION) "dev"

# This generates mocks against public proto packages in consul. At the time of writing,
# only the dns and resource packages are used in consul-dataplane so only mocks for their
# interfaces are generated here.
.PHONY: mocks
mocks:
	for pkg in pbdns pbresource; do \
		mockery --srcpkg=github.com/hashicorp/consul/proto-public/$$pkg --output ./internal/mocks/$${pkg}mock --outpkg $${pkg}mock --case underscore --all; \
	done

##@ Help

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php
.PHONY: help
help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)
