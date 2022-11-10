<h1>
  <img src="./_doc/logo.svg" align="left" height="46px" alt="Consul logo"/>
  <span>Consul Dataplane<sup>BETA</sup></span>
</h1>

Consul Dataplane is a lightweight process that manages Envoy for Consul service mesh workloads.

Consul Dataplane's design removes the need to run Consul client agents. Removing Consul client
agents results in the following benefits:

- **Fewer network requirements**: Consul client agents use multiple network protocols and require
  bi-directional communication between Consul client and server agents for the gossip protocol.
  Consul Dataplane does not use gossip and instead uses a single gRPC connection out to the Consul servers.
- **Simplified set up**: Consul Dataplane does not need to be configured with a gossip encryption key
  and operators do not need to distribute separate ACL tokens for Consul client agents.
- **Additional runtime support**: Consul Dataplane runs as a sidecar alongside your workload, making
  it easier to support various runtimes. For example, it runs on serverless platforms when you do not have access
  to the host machine where the Consul client agent can be run.
- **Easier upgrades**: Deploying new Consul versions no longer requires  upgrading Consul
  client agents. Consul Dataplane has better compatibility across Consul server versions, so
  you only need to upgrade the Consul servers to take advantage of new Consul features.

Refer to the [Documentation](#documentation) section for more information on Consul Dataplane.

**Note**: We take Consul's security and our users' trust seriously. If you believe you have
found a security issue in Consul, please responsibly disclose by contacting us at
security@hashicorp.com.

## Development

### Build

#### Binary

```
make dev
```

#### Docker Image

```
make docker
```

### Testing

#### Unit Tests

```
make unit-tests
```

## Documentation

Consul Dataplane is currently in beta. It currently supports the following features:

- **Consul server discovery**: Consul Dataplane discovers Consul server addresses using DNS or by
  running a script, and is quickly notified of new Consul servers using a ServerWatch gRPC stream.
- **Consul server connection**: Consul Dataplane maintains a gRPC connection to a Consul server and
  automatically switches to another Consul server as needed.
- **Feature discovery**: Consul Dataplane checks Consul server feature support to facilitate version
  compatibility.
- **Envoy management**: Consul Dataplane configures, starts, and manages an Envoy sub-process.
- **Envoy ADS proxy**: Consul Dataplane proxies Envoy's Aggregated Discovery Service (ADS) to a Consul
  server.
- **Consul DNS proxy**: Consul Dataplane runs a local DNS server and proxy DNS requests over a
  gRPC connection to a Consul server to enable service discovery.
- **Merged metrics**: Consul Dataplane supports Prometheus, StatsD, and DogstatsD metrics. Prometheus metrics
  for Consul Dataplane, Envoy, and your application are merged and served from a single endpoint.

We plan to add the following features in a subsequent release:

- Envoy SDS: Consul Dataplane will implement a Secret Discovery Service (SDS) for Envoy to generate
  secret keys and certificate signing requests in order to offload cryptographic operations from the
  Consul servers.
- Config files: Consul Dataplane currently supports CLI flags and environment variables and will
  support configuration files.

### Requirements

- Consul server version 1.14+
- A [compatible version](https://www.consul.io/docs/connect/proxies/envoy#supported-versions) of
  Envoy. The `envoy` binary must be found on the PATH.

### Usage

You should run the `consul-dataplane` binary as a sidecar alongside your service mesh workloads in
place of Envoy. You should not run Envoy directly. Instead, use `consul-dataplane` to configure and
start Envoy for you.

In containerized environments, use the `hashicorp/consul-dataplane` image in place of an Envoy
image. This image includes the `consul-dataplane`, `envoy` and `go-discover` binaries.

The following minimal example shows how to start `consul-dataplane`:

```
consul-dataplane \
    -addresses "exec=./get-addresses.sh" \
    -service-node-name my-test-node \
    -proxy-service-id my-svc-id-sidecar-proxy \
    ...
```

Consul Dataplane connects to Consul servers specified in `-addresses`. Then it starts an Envoy
sub-process configured to run as a sidecar proxy for the service specified by `-proxy-service-id` on the
node specified by `-service-node-name` in the Consul service catalog.

### Server Discovery

Consul Dataplane connects directly to your Consul servers. It supports two forms of address
discovery in the `-addresses` field which uses the syntax supported by
[go-netaddrs](https://github.com/hashicorp/go-netaddrs).

- **DNS**: Consul Dataplane resolves a domain name to discover Consul server IP addresses.

  ```
  consul-dataplane -addresses my.consul.example.com
  ```

- **Executable Command**: Consul Dataplane runs a script that, on success, returns one
  or more IP addresses separate by whitespace:

  ```
  $ ./my-script.sh
  172.20.0.1
  172.20.0.2
  172.20.0.3
  $ consul-dataplane -addresses "exec=./my-script.sh"
  ```

  The [`go-discover`](https://github.com/hashicorp/go-discover) binary is included in the
  `hashicorp/consul-dataplane` image for use with this mode of server discovery, which functions in
  a way similar to [Cloud Auto-join](https://www.consul.io/docs/install/cloud-auto-join). The
  following example demonstrates how to use the `go-discover` binary with Consul Dataplane.

  ```
  consul-dataplane -addresses "exec=discover -q addrs provider=aws region=us-west-2 tag_key=consul-server tag_value=true"
  ```

### Credentials

Consul Dataplane requires an ACL token when ACLs are enabled on the Consul servers. An ACL token
can be specified one of two ways.

- **Static token**: A static ACL token is passed to Consul Dataplane.

  ```
  consul-dataplane -credential-type "static"` -static-token "12345678-90ab-cdef-0000-12345678abcd"
  ```

- **Auth method login**: Consul Dataplane logs in to one of Consul's supported [auth
  methods](https://www.consul.io/docs/security/acl/auth-methods).

  ```
  consul-dataplane -credential-type "login"
    -login-auth-method <method> \
    -login-bearer-token <token> \  # Or, -login-bearer-token-path
    -login-datacenter <datacenter> \
    -login-meta key1=val1 -login-meta key2=val2 \
    -login-namespace <namespace> \
    -login-partition <partition>
  ```

Refer to the [Configuration Reference](#configuration-reference) for a description of each option
passed to `consul-dataplane`.

### Consul Servers Behind a Load Balancer

When Consul servers are behind a load balancer, you must pass `-server-watch-disabled` to Consul
Dataplane:

```
consul-dataplane -server-watch-disabled
```

By default, Consul Dataplane opens a server watch stream to a Consul server, which enables the server
to inform Consul Dataplane of new or different Consul server addresses. However, if Consul Dataplane
is connecting through a load balancer, then it must ignore the Consul server addresses that are
returned from the server watch stream.

### Configuration Reference

The `consul-dataplane` binary supports the following flags.

| Flag                                    | Environment Variable                    | Type            | Default       | Description                                                                                                                                                                                                                                |
| --------------------------------------- | --------------------------------------- | --------------- | ------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `-addresses`                            | `DP_CONSUL_ADDRESSES`                   | string          |               | Consul server gRPC addresses. This can be a DNS name or an executable command in the format, `exec=<executable with optional args>`.<br>Refer to [go-netaddrs](https://github.com/hashicorp/go-netaddrs#summary) for details and examples. |
| `-ca-certs`                             | `DP_CA_CERTS`                           | string          |               | The path to a file or directory containing CA certificates used to verify the server's certificate.                                                                                                                                        |
| `-consul-dns-bind-addr`                 | `DP_CONSUL_DNS_BIND_ADDR`               | string          | `"127.0.0.1"` | The address that will be bound to the consul dns proxy.                                                                                                                                                                                    |
| `-consul-dns-bind-port`                 | `DP_CONSUL_DNS_BIND_PORT`               | int             | `-1`          | The port the consul dns proxy will listen on. By default -1 disables the dns proxy                                                                                                                                                         |
| `-credential-type`                      | `DP_CREDENTIAL_TYPE`                    | string          |               | The type of credentials, either static or login, used to authenticate with Consul servers.                                                                                                                                                 |
| `-envoy-admin-bind-address`             | `DP_ENVOY_ADMIN_BIND_ADDRESS`           | string          | `"127.0.0.1"` | The address on which the Envoy admin server is available.                                                                                                                                                                                  |
| `-envoy-admin-bind-port`                | `DP_ENVOY_ADMIN_BIND_PORT`              | int             | `19000`       | The port on which the Envoy admin server is available.                                                                                                                                                                                     |
| `-envoy-concurrency`                    | `DP_ENVOY_CONCURRENCY`                  | int             | `2`           | The number of worker threads that Envoy uses.                                                                                                                                                                                              |
| `-envoy-ready-bind-address`             | `DP_ENVOY_READY_BIND_ADDRESS`           | string          |               | The address on which Envoy's readiness probe is available.                                                                                                                                                                                 |
| `-envoy-ready-bind-port`                | `DP_ENVOY_READY_BIND_PORT`              | int             |               | The port on which Envoy's readiness probe is available.                                                                                                                                                                                    |
| `-grpc-port`                            | `DP_CONSUL_GRPC_PORT`                   | int             | `8502`        | The Consul server gRPC port to which consul-dataplane connects.                                                                                                                                                                            |
| `-log-json`                             | `DP_LOG_JSON`                           | bool            | `false`       | Enables log messages in JSON format.                                                                                                                                                                                                       |
| `-log-level`                            | `DP_LOG_LEVEL`                          | string          | `"info"`      | Log level of the messages to print. Available log levels are "trace", "debug", "info", "warn", and "error".                                                                                                                                |
| `-login-auth-method`                    | `DP_CREDENTIAL_LOGIN_AUTH_METHOD`       | string          |               | The auth method used to log in.                                                                                                                                                                                                            |
| `-login-bearer-token`                   | `DP_CREDENTIAL_LOGIN_BEARER_TOKEN`      | string          |               | The bearer token presented to the auth method.                                                                                                                                                                                             |
| `-login-bearer-token-path`              | `DP_CREDENTIAL_LOGIN_BEARER_TOKEN_PATH` | string          |               | The path to a file containing the bearer token presented to the auth method.                                                                                                                                                               |
| `-login-datacenter`                     | `DP_CREDENTIAL_LOGIN_DATACENTER`        | string          |               | The datacenter containing the auth method.                                                                                                                                                                                                 |
| `-login-meta`                           | `DP_CREDENTIAL_LOGIN_META{1,9}`         | string          |               | A set of key/value pairs to attach to the ACL token. Each pair is formatted as `<key>=<value>`. This flag may be passed multiple times.                                                                                                    |
| `-login-namespace`                      | `DP_CREDENTIAL_LOGIN_NAMESPACE`         | string          |               | The Consul Enterprise namespace containing the auth method.                                                                                                                                                                                |
| `-login-partition`                      | `DP_CREDENTIAL_LOGIN_PARTITION`         | string          |               | The Consul Enterprise partition containing the auth method.                                                                                                                                                                                |
| `-proxy-service-id`                     | `DP_PROXY_SERVICE_ID`                   | string          |               | The proxy service instance's ID.                                                                                                                                                                                                           |
| `-proxy-service-id-path`                | `DP_PROXY_SERVICE_ID_PATH`              | string          |               | The path to a file containing the proxy service instance's ID.                                                                                                                                                                             |
| `-server-watch-disabled`                | `DP_SERVER_WATCH_DISABLED`              | bool            | `false`       | Setting this prevents consul-dataplane from consuming the server update stream. This is useful for situations where Consul servers are behind a load balancer.                                                                             |
| `-service-namespace`                    | `DP_SERVICE_NAMESPACE`                  | string          |               | The Consul Enterprise namespace in which the proxy service instance is registered.                                                                                                                                                         |
| `-service-node-id`                      | `DP_SERVICE_NODE_ID`                    | string          |               | The ID of the Consul node to which the proxy service instance is registered.                                                                                                                                                               |
| `-service-node-name`                    | `DP_SERVICE_NODE_NAME`                  | string          |               | The name of the Consul node to which the proxy service instance is registered.                                                                                                                                                             |
| `-service-partition`                    | `DP_SERVICE_PARTITION`                  | string          |               | The Consul Enterprise partition in which the proxy service instance is registered.                                                                                                                                                         |
| `-static-token`                         | `DP_CREDENTIAL_STATIC_TOKEN`            | string          |               | The ACL token used to authenticate requests to Consul servers when `-credential-type` is set to static.                                                                                                                                    |
| `-telemetry-prom-ca-certs-path`         | `DP_TELEMETRY_PROM_CA_CERTS_PATH`       | string          |               | The path to a file or directory containing CA certificates used to verify the Prometheus server's certificate.                                                                                                                             |
| `-telemetry-prom-cert-file`             | `DP_TELEMETRY_PROM_CERT_FILE`           | string          |               | The path to the client certificate used to serve Prometheus metrics.                                                                                                                                                                       |
| `-telemetry-prom-key-file`              | `DP_TELEMETRY_PROM_KEY_FILE`            | string          |               | The path to the client private key used to serve Prometheus metrics.                                                                                                                                                                       |
| `-telemetry-prom-merge-port`            | `DP_TELEMETRY_PROM_MERGE_PORT`          | int             | `20100`       | The port to serve merged Prometheus metrics.                                                                                                                                                                                               |
| `-telemetry-prom-retention-time`        | `DP_TELEMETRY_PROM_RETENTION_TIME`      | duration        | `1m0s`        | The duration for Prometheus metrics aggregation.                                                                                                                                                                                           |
| `-telemetry-prom-scrape-path`           | `DP_TELEMETRY_PROM_SCRAPE_PATH`         | string          | `"/metrics"`  | The URL path where Envoy serves Prometheus metrics.                                                                                                                                                                                        |
| `-telemetry-prom-service-metrics-url`   | `DP_TELEMETRY_PROM_SERVICE_METRICS_URL` | string          |               | Prometheus metrics at this URL are scraped and included in Consul Dataplane's main Prometheus metrics.                                                                                                                                     |
| `-telemetry-use-central-config`         | `DP_TELEMETRY_USE_CENTRAL_CONFIG`       | bool            | `true`        | Controls whether the proxy applies the central telemetry configuration.                                                                                                                                                                    |
| `-tls-cert`                             | `DP_TLS_CERT`                           | string          |               | The path to a client certificate file. This is required if `tls.grpc.verify_incoming` is enabled on the server.                                                                                                                            |
| `-tls-disabled`                         | `DP_TLS_DISABLED`                       | bool            | `false`       | Communicate with Consul servers over a plaintext connection. Useful for testing, but not recommended for production.                                                                                                                       |
| `-tls-insecure-skip-verify`             | `DP_TLS_INSECURE_SKIP_VERIFY`           | bool            | `false`       | Do not verify the server's certificate. Useful for testing, but not recommended for production.                                                                                                                                            |
| `-tls-key`                              | `DP_TLS_KEY`                            | string          |               | The path to a client private key file. This is required if `tls.grpc.verify_incoming` is enabled on the server.                                                                                                                            |
| `-tls-server-name`                      | `DP_TLS_SERVER_NAME`                    | string          |               | The hostname to expect in the server certificate's subject. This is required if `-addresses` is not a DNS name.                                                                                                                            |
| `-version`                              |                                         | bool            | `false`       | Prints the current version of consul-dataplane.                                                                                                                                                                                            |
| `-xds-bind-addr`                        | `DP_XDS_BIND_ADDR`                      | string          | `"127.0.0.1"` | The address on which the Envoy xDS server is available.                                                                                                                                                                                    |
| `-xds-bind-port`                        | `DP_XDS_BIND_PORT`                      | int             |               | The address on which the Envoy xDS server is available.                                                                                                                                                                                    |

### Extending the Container Image

The official `hashicorp/consul-dataplane` container image is ["distroless"](https://github.com/GoogleContainerTools/distroless)
and only includes the bare-minimum runtime dependencies, for greater security.

You may want to add a shell that can be used by the `-addresses exec=...` flag
to resolve Consul servers with a custom script.

Here's an example of how you might do that, copying `sh` from the busybox image:

```Dockerfile
FROM hashicorp/consul-dataplane:latest
COPY --from=busybox:uclibc /bin/sh /bin/sh
```
