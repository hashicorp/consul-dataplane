# Investigation Status — Readiness Probe Issue

## Current State

**Branch:** `sujay/fix-readiness-probe` (based on `v1.4.6`, the buggy release)
**consul-k8s:** reverted to clean `main` (no changes)
**Changes:** 4 files modified in consul-dataplane, NOT committed
**Cluster:** KIND cluster UP with Consul Enterprise 1.18.15, nginx at 1/2

---

## Root Cause Analysis: Two Distinct Problems

### Problem 1: bootstrap.go ordering bug — FIX APPLIED, VERIFIED CORRECT

**Root cause:** In `pkg/consuldp/bootstrap.go`, `ReadyBindAddr` was set BEFORE
`mapstructure.WeakDecode()` / `bootstrapConfigFromCfg()`, which overwrites it.

**Fix (applied, uncommitted):** Move `ReadyBindAddr` assignment AFTER the decode.
Also default the bind address to `0.0.0.0` when only port is specified.

**Verification:** After our fix, the Envoy bootstrap config correctly contains the
`envoy_ready_listener` static listener on `<podIP>:21000`. Confirmed via:
```
curl http://127.0.0.1:19000/config_dump   # from nginx container in pod
curl http://127.0.0.1:19000/listeners      # shows: envoy_ready_listener::<podIP>:21000
```

### Problem 2: Consul Server 1.18.15 regression — xDS never delivers CDS

**Root cause: Consul Enterprise 1.18.15 server-side regression.** The catalog
`ConfigSource.Watch()` succeeds but `startSync()` is never called, so proxycfg
never produces a snapshot, and xDS never responds to Envoy's CDS request.

---

## Version Bisection Results (DEFINITIVE)

Tested with stock `consul-dataplane:1.4.6` (no code changes) against different
Consul Enterprise server versions:

| Consul Server Version | consul-dataplane | Pod Status | CDS | Result |
|----------------------|------------------|------------|-----|--------|
| 1.18.5               | 1.4.6            | 2/2 Running | ✅ completes | **WORKS** |
| 1.18.10              | 1.4.6            | 2/2 Running | ✅ completes | **WORKS** |
| 1.18.13              | 1.4.6            | 2/2 Running | ✅ completes | **WORKS** |
| 1.18.14              | 1.4.6            | 2/2 Running | ✅ completes | **WORKS** |
| **1.18.15**          | 1.4.6            | **1/2 Running** | ❌ stuck | **BROKEN** |

**Conclusion:** The regression was introduced in Consul Enterprise 1.18.15.
Consul 1.18.14 is the last working version in the 1.18.x line.

---

## TRACE Log Comparison

### Working (1.18.5) — Key sequence:
```
server-catalog: syncing catalog service    # <-- startSync() called
proxycfg: register                         # Manager.Register() called
proxycfg: updating snapshot                # snapshot produced
xds: sending CDS response                  # CDS delivered to Envoy
```

### Broken (1.18.15) — Key sequence:
```
xds: watching proxy, pending initial proxycfg snapshot for xDS   # Watch() returns OK
                                                                  # ... silence ...
                                                                  # startSync() NEVER called
                                                                  # No "syncing catalog service" log
                                                                  # No proxycfg snapshot
                                                                  # CDS never responds
```

---

## Server-Side Code Path Analysis

The xDS flow for catalog-registered proxies:

```
Envoy → consul-dataplane (gRPC proxy) → Consul Server xDS handler
  → delta.go: process() extracts proxyID from Envoy node metadata
  → delta.go: s.ProxyWatcher.Watch(proxyID, nodeName, token)
  → catalog ConfigSource.Watch():
      1. Check if service is local → NO (registered via catalog)
      2. Begin session with SessionLimiter → OK (unlimited capacity)
      3. startSync() → SHOULD fetch proxy from state store and register with proxycfg Manager
         ↑ THIS STEP FAILS TO EXECUTE ON 1.18.15
```

### Eliminated causes:

| Hypothesis | Status | Evidence |
|-----------|--------|---------|
| Session limiter blocking | ❌ Eliminated | Defaults to `max: Unlimited`, capacity controller correctly sets `max_sessions=2` |
| Enterprise license expired | ❌ Eliminated | Valid until 2026-05-16, server running fine |
| Namespace-related | ❌ Eliminated | Fails in both "nginx" and "default" namespaces |
| Consul client agents | ❌ Eliminated | Fails with `client.enabled=false` too |
| Missing config entries | ❌ Eliminated | Fails even with proxy-defaults and mesh config entries |
| xDS capacity controller | ❌ Eliminated | Running correctly, `max_sessions=2 num_servers=1 num_proxies=1` |
| consul-dataplane bug | ❌ Eliminated | Fails with stock `hashicorp/consul-dataplane:1.4.6` image too |
| `enable_xds_load_balancing` | ❌ N/A | PR #22299 was NOT backported to 1.18.x; config key is invalid |

### Most likely cause:

The Enterprise build of Consul 1.18.15 has ENT-specific modifications to the catalog
`ConfigSource` or `ProxyWatcher` that are not visible in the OSS repo. Something
changed between 1.18.14 and 1.18.15 that prevents `startSync()` from executing after
`Watch()` returns.

---

## Evidence from Live Debugging

### Listening ports inside the pod (1.18.15)
```
Port       Local Addr                State
--------------------------------------------------
80         0.0.0.0                   LISTEN   # nginx app
41777      127.0.0.1                 LISTEN   # consul-dataplane xDS server
20600      127.0.0.1                 LISTEN   # lifecycle/graceful port
8600       127.0.0.1                 LISTEN   # DNS proxy
19000      127.0.0.1                 LISTEN   # Envoy admin

# NOT listening:
# 20000 — inbound proxy (needs xDS)
# 21000 — envoy_ready_listener (needs workers)
```

### xDS connection status (Envoy → consul-dataplane)
```
consul-dataplane::127.0.0.1:41777::cx_active::1       # Connection established
consul-dataplane::127.0.0.1:41777::rq_active::1       # Streaming request active
consul-dataplane::127.0.0.1:41777::rq_success::0      # ZERO successful responses
consul-dataplane::127.0.0.1:41777::health_flags::healthy
```

### Envoy state on 1.18.15
```
state: PRE_INITIALIZING
listener_manager.workers_started: 0      # Workers never started
cluster_manager.cds.update_success: 0    # CDS never succeeded
cluster_manager.cds.init_fetch_timeout: 0 # No timeout (waits forever)
```

### Consul server confirms proxy is registered
```json
{
  "ServiceProxy": {
    "DestinationServiceName": "nginx",
    "LocalServiceAddress": "127.0.0.1",
    "LocalServicePort": 80,
    "Mode": "transparent"
  }
}
```

---

## Code Changes Applied (on `sujay/fix-readiness-probe` branch)

All changes are uncommitted, on branch `sujay/fix-readiness-probe` based on `v1.4.6`.

### 1. `pkg/consuldp/bootstrap.go` (CRITICAL FIX)

Moved `ReadyBindAddr` assignment AFTER `WeakDecode`/`bootstrapConfigFromCfg` so
central config doesn't overwrite it. Also defaults address to `0.0.0.0`.

**Before:**
```go
var bootstrapConfig bootstrap.BootstrapConfig
if envoy.ReadyBindAddress != "" && envoy.ReadyBindPort != 0 {
    bootstrapConfig.ReadyBindAddr = net.JoinHostPort(envoy.ReadyBindAddress, strconv.Itoa(envoy.ReadyBindPort))
}
if cdp.cfg.Telemetry.UseCentralConfig {
    // WeakDecode or bootstrapConfigFromCfg here overwrites ReadyBindAddr
    ...
}
```

**After:**
```go
var bootstrapConfig bootstrap.BootstrapConfig
if cdp.cfg.Telemetry.UseCentralConfig {
    // WeakDecode or bootstrapConfigFromCfg runs first
    ...
}
// ReadyBindAddr set AFTER decode to prevent overwrite
if envoy.ReadyBindPort != 0 {
    addr := envoy.ReadyBindAddress
    if addr == "" {
        addr = "0.0.0.0"
    }
    bootstrapConfig.ReadyBindAddr = net.JoinHostPort(addr, strconv.Itoa(envoy.ReadyBindPort))
}
```

### 2. `pkg/consuldp/lifecycle.go`

Changed `defaultLifecycleBindPort = "20300"` to `"20600"` to align with
consul-k8s's `DefaultGracefulPort`.

### 3. `cmd/consul-dataplane/config.go`

Changed default `"gracefulPort": 20300` to `20600`.

### 4. `cmd/consul-dataplane/config_test.go`

Updated 8 test expectations from `GracefulPort: 20300` to `GracefulPort: 20600`.

---

## consul-k8s Changes: REVERTED

All consul-k8s changes have been reverted. The repo is on clean `main`.

The workaround is to use the `consul.hashicorp.com/use-proxy-health-check: "true"`
annotation, which makes consul-k8s pass the correct flags/env vars to
consul-dataplane.

---

## Current Assessment

### Bootstrap fix: VERIFIED CORRECT
The `envoy_ready_listener` static listener appears in Envoy's bootstrap config
when using our patched binary with the annotation. This is the consul-dataplane
side fix and is correct.

### xDS regression: SERVER-SIDE BUG IN 1.18.15
The readiness probe fails because Envoy is stuck at `PRE_INITIALIZING` — the Consul
server never delivers CDS responses. This is a regression introduced in Consul
Enterprise 1.18.15 (1.18.14 works). The root cause is in ENT-specific server code
that is not visible in the OSS repository.

### Next Steps

1. **Try Consul 1.18.16 or 1.18.17** to see if the regression was already fixed
   in a later release

2. **Report the server regression** to the Consul server team with the version
   bisection results and TRACE log evidence

3. **Investigate potential consul-dataplane workaround** — e.g., setting Envoy
   `initial_fetch_timeout` to allow workers to start even without CDS completing

4. **Test the bootstrap.go fix end-to-end** against a working Consul version
   (1.18.14 or earlier) to fully validate the readiness probe fix

---

## Repository State Summary

```
consul-dataplane:
  Branch: sujay/fix-readiness-probe (from v1.4.6)
  Status: 4 files modified, NOT committed
  Files:
    - pkg/consuldp/bootstrap.go         (+12/-3)  [critical fix]
    - pkg/consuldp/lifecycle.go          (+1/-1)   [port alignment]
    - cmd/consul-dataplane/config.go     (+1/-1)   [port alignment]
    - cmd/consul-dataplane/config_test.go (+8/-8)  [test updates]

consul-k8s:
  Branch: main (clean)
  Status: no changes
```

## Cluster State

KIND cluster `consul` is UP with Consul Enterprise 1.18.15.
nginx deployment at 1/2 Running (consul-dataplane sidecar stuck).
