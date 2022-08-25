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

.PHONY: docker-build
docker-build:
	docker build --no-cache . -t consul-dataplane