<h1>
  <img src="./_doc/logo.svg" align="left" height="46px" alt="Consul logo"/>
  <span>Consul Dataplane</span>
</h1>

Test doc change

Consul Dataplane is a lightweight process that manages Envoy for Consul service mesh workloads.

Consul Dataplane's design removes the need to run Consul client agents. Removing Consul client
agents results in the following benefits:

- **Fewer networking requirements**: Without client agents, Consul does not require bidirectional
  network connectivity across multiple protocols to enable gossip communication. Instead, it
  requires a single gRPC connection to the Consul servers, which significantly simplifies
  requirements for the operator.
- **Simplified set up**: Because there are no client agents to engage in gossip, you do not have to
  generate and distribute a gossip encryption key to agents during the initial bootstrapping
  process. Securing agent communication also becomes simpler, with fewer tokens to track,
  distribute, and rotate.
- **Additional environment and runtime support**: Current Consul on Kubernetes deployments require
  using hostPorts and DaemonSets for client agents, which limits Consul’s ability to be deployed in
  environments where those features are not supported. As a result, Consul Dataplane supports AWS
  Fargate and GKE Autopilot.
- **Easier upgrades**: With Consul Dataplane, updating Consul to a new version no longer requires
  upgrading client agents. Consul Dataplane also has better compatibility across Consul server
  versions, so the process to upgrade Consul servers becomes easier.

Refer to the [Documentation](https://developer.hashicorp.com/consul/docs/connect/dataplane) for more
information on Consul Dataplane.

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

## Releasing

See: engineering docs
