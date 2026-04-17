# Investigation Report: consul-dataplane Stuck Unhealthy (Readiness Probe Failure on Port 20000)

## Summary

The readiness probe is a `tcpSocket` check on port 20000, which is the Envoy
**inbound proxy listener** port (configured via xDS by the Consul server). The
probe fails because the Envoy inbound listener is not created on port 20000 in
affected versions. This is caused by changes in both consul-k8s and
consul-dataplane affecting the proxy lifecycle and startup sequence.

## Affected Versions

| Component | Working | Broken |
|-----------|---------|--------|
| Consul Server | 1.18.5 - 1.18.14 | **1.18.15+** (confirmed through 1.22.5+ent) |
| Helm Chart (consul-k8s) | 1.4.5 | **1.4.10+** (confirmed through 1.9.5) |
| consul-dataplane | 1.4.3 | **1.4.5+** (GH-687 backport) |

## How the Readiness Probe Works (from consul-k8s)

consul-k8s connect-injector (`consul_dataplane_sidecar.go:56-78`) configures
the readiness probe in two modes:

**Default (no annotation):**
```go
// tcpSocket check on the Envoy INBOUND listener (xDS-configured)
readinessProbe = &corev1.Probe{
    ProbeHandler: corev1.ProbeHandler{
        TCPSocket: &corev1.TCPSocketAction{
            Port: intstr.FromInt(constants.ProxyDefaultInboundPort), // 20000
        },
    },
    InitialDelaySeconds: 1,
}
```
- Does NOT pass `-envoy-ready-bind-port` to consul-dataplane
- Does NOT set `DP_ENVOY_READY_BIND_ADDRESS`
- Relies entirely on the Envoy xDS inbound listener being on port 20000

**With `consul.hashicorp.com/use-proxy-health-check: "true"`:**
```go
// httpGet check on a DEDICATED ready listener (bootstrap-configured)
readinessProbe = &corev1.Probe{
    ProbeHandler: corev1.ProbeHandler{
        HTTPGet: &corev1.HTTPGetAction{
            Port: intstr.FromInt(constants.ProxyDefaultHealthPort), // 21000
            Path: "/ready",
        },
    },
    InitialDelaySeconds: 1,
}
```
- Passes `-envoy-ready-bind-port=21000` to consul-dataplane
- Sets `DP_ENVOY_READY_BIND_ADDRESS` to pod IP (via `status.podIP`)
- Creates a separate Envoy ready listener on port 21000 (no port conflict)

### Key consul-k8s Constants

Source: `control-plane/connect-inject/constants/constants.go`

| Constant | Value | Purpose |
|----------|-------|---------|
| `ProxyDefaultInboundPort` | **20000** | Envoy inbound listener (xDS); default readiness probe port |
| `ProxyDefaultHealthPort` | **21000** | Envoy ready listener (bootstrap); used with use-proxy-health-check |
| `DefaultGracefulPort` | **20600** | Lifecycle HTTP server for graceful startup/shutdown |

## Root Cause

### 1. consul-dataplane v1.4.5 added a blocking `gracefulStartup()` call

In commit `88e4000` (GH-687 backport to release/1.4.x), a synchronous call was
added to `pkg/consuldp/consul_dataplane.go`:

```go
// v1.4.4 (working): no gracefulStartup() call
cdp.lifecycleConfig.startLifecycleManager(ctx)
// monitoring goroutine starts immediately

// v1.4.5+ (broken): gracefulStartup() blocks before monitoring starts
cdp.lifecycleConfig.startLifecycleManager(ctx)
cdp.lifecycleConfig.gracefulStartup()   // <-- NEW: blocks if startupGracePeriodSeconds > 0
// monitoring goroutine starts only AFTER gracefulStartup returns
```

When `startupGracePeriodSeconds > 0`, this blocks the main goroutine polling the
Envoy admin endpoint (`http://127.0.0.1:19000/ready`). During this period, the
monitoring goroutine has not started, so errors from Envoy/xDS are not detected.

### 2. Port default mismatch between consul-k8s and consul-dataplane

| Setting | consul-dataplane default | consul-k8s default |
|---------|--------------------------|---------------------|
| GracefulPort | **20300** (`lifecycle.go:24`) | **20600** (`constants.go:62`) |

consul-k8s always passes `-graceful-port=20600` to consul-dataplane. But
consul-dataplane's own default is 20300. If the flag is not passed for any
reason (e.g., older consul-k8s version), the lifecycle server binds to the
wrong port.

### 3. `mapstructure.WeakDecode` overwrites `ReadyBindAddr` in bootstrap.go (THE ROOT CAUSE)

**Confirmed via live debugging**: Envoy logs show `loading 1 listener(s)` — the
`envoy_ready_listener` is missing from the bootstrap even though both
`-envoy-ready-bind-port=21000` and `DP_ENVOY_READY_BIND_ADDRESS=<podIP>` are
correctly set.

The bug is in `pkg/consuldp/bootstrap.go:115-123`. The `ReadyBindAddr` is set
**before** `mapstructure.WeakDecode`, which then overwrites it:

```go
// Line 116-118: ReadyBindAddr is set correctly
var bootstrapConfig bootstrap.BootstrapConfig
if envoy.ReadyBindAddress != "" && envoy.ReadyBindPort != 0 {
    bootstrapConfig.ReadyBindAddr = net.JoinHostPort(envoy.ReadyBindAddress, strconv.Itoa(envoy.ReadyBindPort))
}

// Line 120-123: WeakDecode OVERWRITES ReadyBindAddr back to "" !!!
if cdp.cfg.Telemetry.UseCentralConfig {
    if err := mapstructure.WeakDecode(bootstrapParams.Config.AsMap(), &bootstrapConfig); err != nil {
        ...
    }
}

// Line 138: GenerateJSON uses the now-empty ReadyBindAddr → no ready listener
cfg, err := bootstrapConfig.GenerateJSON(args, true)
```

`UseCentralConfig` defaults to `true`. The Consul server's proxy config map
decoded by `WeakDecode` resets `ReadyBindAddr` to empty, erasing the value set
on line 117. **No combination of flags, env vars, or annotations can survive
this overwrite.**

## Why Common Workaround Approaches Fail

### Pod annotation `envoy-ready-bind-address` / `envoy-ready-bind-port`
consul-k8s has **no such annotations**. The connect-injector silently ignores
unknown annotations.

### Strategic merge patch to inject env vars
The `consul-dataplane` container is injected by the connect-injector **webhook**
at pod creation time -- it is not in the Deployment spec. The webhook overwrites
any container definition you add via patch.

### Setting `DP_ENVOY_READY_BIND_PORT=20000` via env vars
Even if injected, this creates an Envoy `envoy_ready_listener` on port 20000 in
the bootstrap config. This **conflicts** with the xDS inbound listener that also
wants port 20000, potentially causing Envoy binding errors.

## Workaround

Use the `consul.hashicorp.com/use-proxy-health-check` annotation. This switches
the readiness probe from `tcpSocket:20000` (inbound listener) to
`httpGet:/ready` on port `21000` (dedicated ready listener), avoiding port 20000
entirely:

```yaml
metadata:
  annotations:
    consul.hashicorp.com/connect-inject: "true"
    consul.hashicorp.com/use-proxy-health-check: "true"
```

This causes consul-k8s to:
1. Set the readiness probe to `httpGet` on port **21000** `/ready`
2. Pass `-envoy-ready-bind-port=21000` to consul-dataplane
3. Set `DP_ENVOY_READY_BIND_ADDRESS` to the pod IP
4. Create a dedicated Envoy ready listener on `<podIP>:21000` (no port conflict)

> **Note:** The original ISSUE.md mentions that `use-proxy-health-check: "true"`
> had additional issues with sidecar startup being stuck. If this occurs, it may
> be related to the `gracefulStartup()` blocking behavior. Ensure
> `startupGracePeriodSeconds` is 0 (the default) or try the code fix below.

## Concrete Fix

### Fix 1 - `pkg/consuldp/bootstrap.go:115-123` (consul-dataplane) — CRITICAL

Move the `ReadyBindAddr` assignment to **after** `mapstructure.WeakDecode` so it
is not overwritten. Also default address to `0.0.0.0` when only port is set:

```go
// Before (BROKEN — ReadyBindAddr set, then overwritten by WeakDecode):
var bootstrapConfig bootstrap.BootstrapConfig
if envoy.ReadyBindAddress != "" && envoy.ReadyBindPort != 0 {
    bootstrapConfig.ReadyBindAddr = net.JoinHostPort(...)
}
if cdp.cfg.Telemetry.UseCentralConfig {
    mapstructure.WeakDecode(bootstrapParams.Config.AsMap(), &bootstrapConfig) // overwrites!
}

// After (FIXED — ReadyBindAddr set after WeakDecode, cannot be overwritten):
var bootstrapConfig bootstrap.BootstrapConfig
if cdp.cfg.Telemetry.UseCentralConfig {
    if err := mapstructure.WeakDecode(bootstrapParams.Config.AsMap(), &bootstrapConfig); err != nil {
        return nil, nil, fmt.Errorf("failed parsing Proxy.Config: %w", err)
    }
    args.PrometheusBackendPort = strconv.Itoa(prom.MergePort)
}
if envoy.ReadyBindPort != 0 {
    addr := envoy.ReadyBindAddress
    if addr == "" {
        addr = "0.0.0.0"
    }
    bootstrapConfig.ReadyBindAddr = net.JoinHostPort(addr, strconv.Itoa(envoy.ReadyBindPort))
}
```

### Fix 2 - `pkg/consuldp/lifecycle.go:24` (consul-dataplane)

Align the default lifecycle bind port with consul-k8s `DefaultGracefulPort`:

```go
// Before:
defaultLifecycleBindPort = "20300"

// After:
defaultLifecycleBindPort = "20600"
```

And update `cmd/consul-dataplane/config.go:243`:

```go
// Before:
"gracefulPort": 20300,

// After:
"gracefulPort": 20600,
```

### Fix 3 - `consul_dataplane_sidecar.go` (consul-k8s)

The connect-injector should set `DP_ENVOY_READY_BIND_ADDRESS` and pass
`-envoy-ready-bind-port` **even in the default case** (not just when
`use-proxy-health-check=true`). This creates a dedicated ready listener that
the readiness probe can check without relying on the xDS inbound listener:

```go
// In the default (non-health-check) branch, add:
container.Env = append(container.Env, corev1.EnvVar{
    Name: "DP_ENVOY_READY_BIND_ADDRESS",
    ValueFrom: &corev1.EnvVarSource{
        FieldRef: &corev1.ObjectFieldSelector{FieldPath: "status.podIP"},
    },
})
args = append(args, fmt.Sprintf("-envoy-ready-bind-port=%d", constants.ProxyDefaultInboundPort+mpi.serviceIndex))
```

And change the readiness probe from `tcpSocket` to `httpGet`:

```go
readinessProbe = &corev1.Probe{
    ProbeHandler: corev1.ProbeHandler{
        HTTPGet: &corev1.HTTPGetAction{
            Port: intstr.FromInt(constants.ProxyDefaultInboundPort + mpi.serviceIndex),
            Path: "/ready",
        },
    },
    InitialDelaySeconds: 1,
}
```

**Note:** This changes port 20000 from the xDS inbound listener to the Envoy
ready listener. The inbound listener port would need to move, or a separate port
should be used (like the existing `ProxyDefaultHealthPort = 21000`).

## Key Code Locations

### consul-dataplane

| File | Lines | Role |
|------|-------|------|
| `pkg/consuldp/bootstrap.go` | 116-118 | Gating condition for Envoy ready listener creation |
| `pkg/consuldp/lifecycle.go` | 24 | Default lifecycle port (20300, should be 20600) |
| `pkg/consuldp/lifecycle.go` | 228-261 | `gracefulStartup()` - blocks until Envoy ready |
| `pkg/consuldp/consul_dataplane.go` | 247-251 | Lifecycle manager + gracefulStartup() call sequence |
| `cmd/consul-dataplane/config.go` | 236, 243 | Default values for ReadyBindPort (0) and GracefulPort (20300) |
| `cmd/consul-dataplane/main.go` | 102-103 | Env vars: `DP_ENVOY_READY_BIND_ADDRESS`, `DP_ENVOY_READY_BIND_PORT` |

### consul-k8s

| File | Lines | Role |
|------|-------|------|
| `control-plane/connect-inject/webhook/consul_dataplane_sidecar.go` | 56-78 | Readiness probe configuration (tcpSocket vs httpGet) |
| `control-plane/connect-inject/webhook/consul_dataplane_sidecar.go` | 182-195 | `DP_ENVOY_READY_BIND_ADDRESS` env var (only when use-proxy-health-check) |
| `control-plane/connect-inject/webhook/consul_dataplane_sidecar.go` | 384-387 | `-envoy-ready-bind-port` flag (only when use-proxy-health-check) |
| `control-plane/connect-inject/webhook/consul_dataplane_sidecar.go` | 619-630 | `useProxyHealthCheck()` annotation check |
| `control-plane/connect-inject/constants/constants.go` | 27, 30, 62 | Port constants (20000, 21000, 20600) |
| `control-plane/connect-inject/webhook/redirect_traffic.go` | 54 | iptables redirect to ProxyDefaultInboundPort (20000) |
