# Integration Tests

These tests validate that Consul Dataplane correctly integrates with Envoy and
the Consul server to implement service mesh features.

They are intentionally not exhaustive. For full coverage of Consul's mesh
capabilities, we rely on the existing [suite of integration tests](https://github.com/hashicorp/consul/tree/main/test/integration/connect/envoy)
that do a good job of ensuring servers generate correct Envoy configuration in
different scenarios, but cannot currently accommodate Consul Dataplane without
significant structural changes.

## Running the tests

They're run automatically as part of our [build workflow](https://github.com/hashicorp/consul-dataplane/actions/workflows/build.yml).

If you have Docker, you can also run them locally with:

```bash
# From the project root
$ make integration-tests

# Additional options
$ make integration-tests \
  INTEGRATION_TESTS_OUTPUT_DIR=/path/to/output \
  INTEGRATION_TESTS_SERVER_IMAGE=hashicorp/consul:some-version \
  INTEGRATION_TESTS_DATAPLANE_IMAGE=hashicorp/consul-dataplane:some-version
```
