<h1>
  <img src="./_doc/logo.svg" align="left" height="46px" alt="Consul logo"/>
  <span>Consul Dataplane<sup>BETA</sup></span>
</h1>

Consul Dataplane is a lightweight process that manages Envoy for Consul service mesh workloads.

Consul Dataplane was designed to remove the need to run Consul client agents workloads. Removing
Consul client agents results in the following benefits:

* **Fewer network requirements**: Consul client agents use multiple network protocols and required
  bi-directional communication between Consul client and server agents for the gossip protocol.
  Consul Dataplane does not use gossip and uses a single gRPC connection out to the Consul servers.
* **Simplified set up**: Consul Dataplane does not need to be configured with a gossip encryption key
  and operators do not need to distribute separate ACL tokens for Consul client agents.
* **Additional runtime support**: Consul Dataplane runs as a sidecar alongside your workload making
  it easier to support various runtimes, such as serverless platforms where we do not have access to
  host machine where the Consul client agent can be run.
* **Easier upgrades**: Deploying new Consul versions no longer requires the step of upgrading Consul
  client agents. Consul Dataplane has better compatibility across Consul server versions so that
  you only need to upgrade the Consul servers to take advantage of new Consul features.

See the [Documentation](#documentation) section for more information on Consul Dataplane.

Please note: We take Consul's security and our users' trust very seriously. If you believe you have
found a security issue in Consul, please responsibly disclose by contacting us at
security@hashicorp.com.

## Development

### Testing

#### Unit Tests

`make unit-tests`

### Generate Go code from Consul proto

`make consul-proto`

## Documentation

Consul Dataplane is currently in beta. It currently supports the following:

* **Consul Server Discovery**: Consul Dataplane discovers Consul server addresses using DNS or by
  running a script, and is quickly notified of new Consul servers using a ServerWatch gRPC stream.
* **Consul Server Connnection**: Consul Dataplane maintains a gRPC connection to a Consul server and
  automatically switches to another Consul server as-needed.
* **Feature Discovery**: Consul Dataplane checks Consul server feature support to facilitate version
  compatibility.
* **Envoy Management**: Consul Dataplane configures, starts, and manages an Envoy sub-process.
* **Envoy xDS Proxy**: Consul Dataplane proxies Envoy's Aggregated Discovery Service (SDS) to a Consul
  server.

The following features will be added in the near future:

* Envoy SDS Proxying: Consul Dataplane will proxy Envoy's Aggregated Discovery Service (SDS) to a
  Consul server.
* Consul DNS Proxy: Consul Dataplane will run a local DNS server and proxies DNS requests over a
  gRPC connection to a Consul server.
* Merged Metrics: Consul Dataplane will expose metrics for both Consul Dataplane and Envoy through a
  single endpoint.

### Requirements

* Consul server version 1.14+
* A [compatible version](https://www.consul.io/docs/connect/proxies/envoy#supported-versions) of
  Envoy. The `envoy` binary must be found on the PATH.

### Usage

The `consul-dataplane` binary should be run as a sidecar alongside your service mesh workloads in
place of Envoy. You should not run Envoy directly. Instead, `consul-dataplane` will configure and
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

Consul Dataplane will connect to Consul servers specified in `-addresses`. It will start an Envoy
sub-process configured to run as a sidecar proxy for service specified by `-proxy-service-id` on the
node specified by `-service-node-name` in the Consul service catalog.

### Server Discovery

Consul Dataplane connects directly to your Consul servers. It supports two forms of address
discovery in the `-addresses` field:

* **DNS**: Consul Dataplane will resolve a domain name to discover Consul server IP addresses.

  ```
  consul-dataplane -addresses my.consul.example.com
  ```

* **Executable Command**: Consul Dataplane will run a script that, on success, should return one
  or more IP addresses separate by whitespace:

  ```
  $ ./my-script.sh
  172.20.0.1
  172.20.0.2
  172.20.0.3
  $ consul-dataplane -addresses "exec=./my-script.sh"
  ```

  The `go-discover` binary is included in the `hashicorp/consul-dataplane` image for use with this
  mode of server discovery (similar to [Cloud
  Auto-join](https://www.consul.io/docs/install/cloud-auto-join). The following shows how to use the
  `go-discover` binary with Consul Dataplane.

  ```
  consul-dataplane -addresses "exec=discover -q addrs provider=aws region=us-west-2 tag_key=consul-server tag_value=true"
  ```

### Credentials

Consul Dataplane requires an ACL token when ACLs are enabled on the Consul servers. An ACL token
can be specified one of two ways.

* **Static Token**: A static ACL token is passed to Consul Dataplane

  ```
  consul-dataplane -credential-type "static"` -static-token "12345678-90ab-cdef-0000-12345678abcd"
  ```

* **Auth Method Login**: Consul Dataplane logs in to one of Consul support [auth
  methods](https://www.consul.io/docs/security/acl/auth-methods)

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

| Flag                            | Type   | Default       | Description                                                                                                                                                                                                                                |
|---------------------------------|--------|---------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `-addresses`                    | string |               | Consul server gRPC addresses. This can be a DNS name or an executable command in the format, `exec=<executable with optional args>`.<br>Refer to [go-netaddrs](https://github.com/hashicorp/go-netaddrs#summary) for details and examples. |
| `-ca-certs`                     | string |               | The path to a file or directory containing CA certificates that will be used to verify the server's certificate.                                                                                                                           |
| `-credential-type`              | string |               | The type of credentials that will be used to authenticate with Consul servers (static or login).                                                                                                                                           |
| `-envoy-admin-bind-address`     | string | `"127.0.0.1"` | The address on which the Envoy admin server will be available.                                                                                                                                                                             |
| `-envoy-admin-bind-port`        | int    | `19000`       | The port on which the Envoy admin server will be available.                                                                                                                                                                                |
| `-envoy-concurrency`            | int    | `2`           | The number of worker threads that Envoy will use.                                                                                                                                                                                          |
| `-envoy-ready-bind-address`     | string |               | The address on which Envoy's readiness probe will be available.                                                                                                                                                                            |
| `-envoy-ready-bind-port`        | int    |               | The port on which Envoy's readiness probe will be available.                                                                                                                                                                               |
| `-grpc-port`                    | int    | `8502`        | The Consul server gRPC port to which consul-dataplane connects.                                                                                                                                                                            |
| `-log-json`                     | bool   | `false`       | If this flag is passed, consul-dataplane will log in JSON format.                                                                                                                                                                          |
| `-log-level`                    | string | `"info"`      | Log level of the messages to print. Available log levels are "trace", "debug", "info", "warn", and "error".                                                                                                                                |
| `-login-auth-method`            | string |               | The auth method that will be used to log in.                                                                                                                                                                                               |
| `-login-bearer-token`           | string |               | The bearer token that will be presented to the auth method.                                                                                                                                                                                |
| `-login-bearer-token-path`      | string |               | The path to a file containing the bearer token that will be presented to the auth method.                                                                                                                                                  |
| `-login-datacenter`             | string |               | The datacenter containing the auth method.                                                                                                                                                                                                 |
| `-login-meta`                   | value  |               | An arbitrary set of key/value pairs that will be attached to the ACL token (formatted as key=value, may be given multiple times).                                                                                                          |
| `-login-namespace`              | string |               | The Consul Enterprise namespace containing the auth method.                                                                                                                                                                                |
| `-login-partition`              | string |               | The Consul Enterprise partition containing the auth method.                                                                                                                                                                                |
| `-proxy-service-id`             | string |               | The proxy service instance's ID.                                                                                                                                                                                                           |
| `-server-watch-disabled`        | bool   | `false`       | Setting this prevents consul-dataplane from consuming the server update stream. This is useful for situations where Consul servers are behind a load balancer.                                                                             |
| `-service-namespace`            | string |               | The Consul Enterprise namespace in which the proxy service instance is registered.                                                                                                                                                         |
| `-service-node-id`              | string |               | The ID of the Consul node to which the proxy service instance is registered.                                                                                                                                                               |
| `-service-node-name`            | string |               | The name of the Consul node to which the proxy service instance is registered.                                                                                                                                                             |
| `-service-partition`            | string |               | The Consul Enterprise partition in which the proxy service instance is registered.                                                                                                                                                         |
| `-static-token`                 | string |               | The ACL token used to authenticate requests to Consul servers (when `-credential-type` is set to static).                                                                                                                                  |
| `-telemetry-use-central-config` | bool   | `true`        | Controls whether the proxy will apply the central telemetry configuration.                                                                                                                                                                 |
| `-tls-cert`                     | string |               | The path to a client certificate file (only required if `tls.grpc.verify_incoming` is enabled on the server).                                                                                                                              |
| `-tls-disabled`                 | bool   | `false`       | Communicate with Consul servers over a plaintext connection. Useful for testing, but not recommended for production.                                                                                                                       |
| `-tls-insecure-skip-verify`     | bool   | `false`       | Do not verify the server's certificate. Useful for testing, but not recommended for production.                                                                                                                                            |
| `-tls-key`                      | string |               | The path to a client private key file (only required if `tls.grpc.verify_incoming` is enabled on the server).                                                                                                                              |
| `-tls-server-name`              | string |               | The hostname to expect in the server certificate's subject (required if `-addresses` isn't a DNS name).                                                                                                                                    |
| `-version`                      | bool   | `false`       | Prints the current version of consul-dataplane.                                                                                                                                                                                            |
| `-xds-bind-addr`                | string | `"127.0.0.1"` | The address on which the Envoy xDS server will be available.                                                                                                                                                                               |
