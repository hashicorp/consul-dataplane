# Reproducing and Fixing the Readiness Probe Issue

Step-by-step guide for macOS. Uses a local KIND cluster.

## Prerequisites

Install these if not already present:

```bash
# Docker Desktop (must be running)
# https://www.docker.com/products/docker-desktop/

# kind
brew install kind

# kubectl
brew install kubectl

# helm
brew install helm

# envsubst (part of gettext)
brew install gettext
```

Add the HashiCorp Helm repo:

```bash
helm repo add hashicorp https://helm.releases.hashicorp.com
helm repo update
```

---

## Part 0: Clean Up Any Previous Runs

Run this before starting (safe to run even if nothing exists):

```bash
kubectl delete namespace nginx --ignore-not-found --wait
helm uninstall consul -n consul 2>/dev/null || true
kubectl delete namespace consul --ignore-not-found --wait
kind delete cluster --name consul 2>/dev/null || true
```

---

## Part 1: Reproduce the Issue (Broken Versions — Stock Images)

### 1.1 Create the KIND cluster

```bash
cd ai/REPRO_KIND
kind create cluster --config kind_cluster.yaml
```

Wait for the cluster to be ready:

```bash
kubectl cluster-info --context kind-consul
```

### 1.2 Set broken version variables

```bash
export CONSUL_ENTERPRISE_VERSION=1.18.15
export CONSUL_K8S_VERSION=1.4.10
export APIGW_VERSION=0.5.4
```

### 1.3 Install Consul

```bash
kubectl create namespace consul
kubectl apply -f license.yaml
kubectl apply -f bootstrap-token.yaml
envsubst < consul_values.yaml | helm -n consul install -f - consul hashicorp/consul --version $CONSUL_K8S_VERSION
```

Wait for all Consul pods to be ready (takes 2-3 minutes):

```bash
kubectl get pods -n consul -w
```

Expected: all pods `Running` and `READY` (1/1).

### 1.4 Deploy the test workload (no annotation)

```bash
kubectl create namespace nginx
kubectl apply -f nginx-deployment.yaml
```

### 1.5 Observe the failure

Watch the pod status:

```bash
kubectl get pods -n nginx -w
```

Expected: the pod stays at `1/2 Running` — the consul-dataplane sidecar never becomes ready.

Confirm readiness probe failure:

```bash
kubectl describe pod -n nginx -l app=nginx | tail -20
```

You should see:

```
Warning  Unhealthy  ...  kubelet  Readiness probe failed: dial tcp <POD_IP>:20000: connect: connection refused
```

This confirms the issue: the readiness probe uses `tcpSocket` on port 20000, which
is the xDS inbound listener that doesn't reliably become available quickly enough.

### 1.6 Try the annotation workaround (still fails without code fix)

Delete the first deployment and try with `use-proxy-health-check`:

```bash
kubectl delete -f nginx-deployment.yaml
kubectl delete namespace nginx --wait
kubectl create namespace nginx

cat <<'EOF' | kubectl apply -f -
apiVersion: v1
kind: ServiceAccount
metadata:
  name: nginx
  namespace: nginx
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx
  namespace: nginx
spec:
  selector:
    matchLabels:
      app: nginx
  replicas: 1
  template:
    metadata:
      labels:
        app: nginx
      annotations:
        consul.hashicorp.com/connect-inject: "true"
        consul.hashicorp.com/use-proxy-health-check: "true"
    spec:
      serviceAccountName: nginx
      containers:
      - name: nginx
        image: nginxdemos/hello:0.3
        ports:
        - containerPort: 80
---
apiVersion: v1
kind: Service
metadata:
  name: nginx
  namespace: nginx
spec:
  type: ClusterIP
  selector:
    app: nginx
  ports:
    - port: 80
      targetPort: 80
EOF
```

Watch pod status:

```bash
kubectl get pods -n nginx -w
```

**Expected: STILL fails at `1/2 Running`** — even though consul-k8s now passes
`-envoy-ready-bind-port=21000` and sets `DP_ENVOY_READY_BIND_ADDRESS`, the
**bootstrap.go bug** in consul-dataplane 1.4.6 prevents the ready listener from
being created. You can verify:

```bash
# Readiness probe is now httpGet on 21000 (correct)
kubectl get pod -n nginx -l app=nginx -o json | jq '.items[0].spec.containers[] | select(.name=="consul-dataplane") | .readinessProbe'

# But Envoy only has 1 listener (missing the ready listener)
kubectl logs -n nginx -l app=nginx -c consul-dataplane | grep "loading.*listener"
```

### 1.7 Clean up the workload (keep the cluster and Consul)

```bash
kubectl delete namespace nginx --wait
```

---

## Part 2: Build the Patched consul-dataplane

We keep the KIND cluster and Consul running from Part 1.

### 2.1 Verify you're on the fix branch

From the consul-dataplane repo root:

```bash
cd /Users/sujaykumar/go/src/github.com/hashicorp/consul-dataplane
git branch --show-current
# Should show: sujay/fix-readiness-probe
```

Verify the branch is based on v1.4.6 (the buggy version):

```bash
git log --oneline -1
# Should show: 87c8069 Prep release 1.4.6 (#728)
```

### 2.2 Review the uncommitted changes

```bash
git diff --stat
```

Expected output:

```
 cmd/consul-dataplane/config.go      |  2 +-
 cmd/consul-dataplane/config_test.go | 16 ++++++++--------
 pkg/consuldp/bootstrap.go           | 15 ++++++++++++---
 pkg/consuldp/lifecycle.go           |  2 +-
 4 files changed, 22 insertions(+), 13 deletions(-)
```

The three fixes are:

1. **`pkg/consuldp/bootstrap.go`** (critical fix): Moves `ReadyBindAddr` assignment
   AFTER `WeakDecode`/`bootstrapConfigFromCfg` so central config doesn't overwrite
   it. Also defaults address to `0.0.0.0` when only port is set.

2. **`pkg/consuldp/lifecycle.go`**: Aligns default lifecycle port from `20300` to
   `20600` (matches consul-k8s `DefaultGracefulPort`).

3. **`cmd/consul-dataplane/config.go`** + **`config_test.go`**: Same port alignment
   in JSON defaults and test expectations.

### 2.3 Run unit tests

```bash
go test ./...
```

All tests should pass.

### 2.4 Build the patched Docker image

```bash
make dev-docker
```

This builds a linux binary and produces the Docker image `consul-dataplane:1.4.6`
then tags it as `consul-dataplane:local`.

Verify the image exists:

```bash
docker images | grep consul-dataplane
```

You should see both `consul-dataplane:1.4.6` and `consul-dataplane:local`.

### 2.5 Load the patched image into KIND

```bash
kind load docker-image consul-dataplane:local --name consul
```

---

## Part 3: Deploy with the Patched Image and Validate the Fix

### 3.1 Reinstall Consul with custom dataplane image

We need to point the Helm chart at our patched image. Uninstall the current Consul
and reinstall with the image override:

```bash
helm uninstall consul -n consul
kubectl delete namespace consul --wait

export CONSUL_ENTERPRISE_VERSION=1.18.15
export CONSUL_K8S_VERSION=1.4.10
export APIGW_VERSION=0.5.4

kubectl create namespace consul
kubectl apply -f ai/REPRO_KIND/license.yaml
kubectl apply -f ai/REPRO_KIND/bootstrap-token.yaml

envsubst < ai/REPRO_KIND/consul_values.yaml | helm -n consul install -f - consul hashicorp/consul \
  --version $CONSUL_K8S_VERSION \
  --set global.imageConsulDataplane=consul-dataplane:local
```

Wait for all Consul pods to be ready:

```bash
kubectl get pods -n consul -w
```

### 3.2 Deploy the workload with the annotation

The `use-proxy-health-check` annotation tells consul-k8s to pass the
`-envoy-ready-bind-port=21000` flag and `DP_ENVOY_READY_BIND_ADDRESS` env var.
With our patched bootstrap.go, this now correctly creates the envoy ready listener.

```bash
kubectl create namespace nginx

cat <<'EOF' | kubectl apply -f -
apiVersion: v1
kind: ServiceAccount
metadata:
  name: nginx
  namespace: nginx
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx
  namespace: nginx
spec:
  selector:
    matchLabels:
      app: nginx
  replicas: 1
  template:
    metadata:
      labels:
        app: nginx
      annotations:
        consul.hashicorp.com/connect-inject: "true"
        consul.hashicorp.com/use-proxy-health-check: "true"
    spec:
      serviceAccountName: nginx
      containers:
      - name: nginx
        image: nginxdemos/hello:0.3
        ports:
        - containerPort: 80
---
apiVersion: v1
kind: Service
metadata:
  name: nginx
  namespace: nginx
spec:
  type: ClusterIP
  selector:
    app: nginx
  ports:
    - port: 80
      targetPort: 80
EOF
```

### 3.3 Validate the fix

Watch pod status:

```bash
kubectl get pods -n nginx -w
```

**Expected: pod reaches `2/2 Running` within ~1-2 minutes.** No Unhealthy warnings.

Confirm no probe failures:

```bash
kubectl describe pod -n nginx -l app=nginx | grep -E "Warning|Unhealthy"
```

Expected: no output (no warnings).

Verify the readiness probe is httpGet on port 21000:

```bash
kubectl get pod -n nginx -l app=nginx -o json | \
  jq '.items[0].spec.containers[] | select(.name=="consul-dataplane") | .readinessProbe'
```

Expected:

```json
{
  "httpGet": {
    "path": "/ready",
    "port": 21000
  },
  "initialDelaySeconds": 1
}
```

Verify the flags and env vars are set:

```bash
# Check args
kubectl get pod -n nginx -l app=nginx -o json | \
  jq -r '.items[0].spec.containers[] | select(.name=="consul-dataplane") | .args[]' | grep envoy-ready

# Check env
kubectl get pod -n nginx -l app=nginx -o json | \
  jq '.items[0].spec.containers[] | select(.name=="consul-dataplane") | .env[] | select(.name | startswith("DP_ENVOY_READY"))'
```

Verify Envoy has 2 listeners (inbound + ready):

```bash
kubectl logs -n nginx -l app=nginx -c consul-dataplane | grep "loading.*listener"
```

Expected: `loading 2 listener(s)` (was `1` before the fix).

Verify the ready listener is actually responding:

```bash
POD=$(kubectl get pod -n nginx -l app=nginx -o jsonpath='{.items[0].metadata.name}')
kubectl exec -n nginx $POD -c consul-dataplane -- wget -qO- http://localhost:21000/ready 2>&1 || true
```

---

## Part 4: Full Cleanup

```bash
kubectl delete namespace nginx --ignore-not-found --wait
helm uninstall consul -n consul 2>/dev/null || true
kubectl delete namespace consul --ignore-not-found --wait
kind delete cluster --name consul 2>/dev/null || true
```

---

## Summary

| Step | What happens | Pod status |
|------|-------------|------------|
| Part 1.5 — Stock images, no annotation | tcpSocket:20000 probe, nothing listens there | `1/2 Running` ❌ |
| Part 1.6 — Stock images, with annotation | httpGet:21000 probe, but bootstrap.go bug prevents ready listener | `1/2 Running` ❌ |
| Part 3.3 — Patched dataplane, with annotation | httpGet:21000 probe, bootstrap.go fix creates ready listener | `2/2 Running` ✅ |

The fix is **consul-dataplane only** — no consul-k8s code changes needed. Users
add the `consul.hashicorp.com/use-proxy-health-check: "true"` annotation to their
pod spec, and the patched bootstrap.go correctly creates the Envoy ready listener.
