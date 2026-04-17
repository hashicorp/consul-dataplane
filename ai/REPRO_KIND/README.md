### Create K8s cluster
```
kind create cluster --config kind_cluster.yaml
```

### Install Consul 
```
alias k=kubectl
export CONSUL_ENTERPRISE_VERSION=1.18.15
export CONSUL_K8S_VERSION=1.4.10
k create namespace consul
k apply -f license.yaml
k apply -f bootstrap-token.yaml
envsubst < consul_values.yaml | helm -n consul install -f - consul hashicorp/consul --version $CONSUL_K8S_VERSION
```

### Access Consul UI (optional)
```
https://localhost:30001
```
Token
```
61f69a27-028d-ad76-e4e5-b538334caf3e
```
### Install NGINX
```
k create namespace nginx
k apply -f nginx-deployment.yaml
```
### See the issue
```
k get pod -n nginx
```
Example:
% k get pod -n nginx
NAME                     READY   STATUS    RESTARTS   AGE
nginx-7c6fd947b4-rbw54   1/2     Running   0          9m46s

```
k describe pod -n nginx | grep Events -A 50
```
Example:
Events:
  Type     Reason     Age                 From               Message
  ----     ------     ----                ----               -------
  Normal   Scheduled  11m                 default-scheduler  Successfully assigned nginx/nginx-7c6fd947b4-rbw54 to consul-worker
  Normal   Pulled     11m                 kubelet            Container image "hashicorp/consul-k8s-control-plane:1.4.10" already present on machine
  Normal   Created    11m                 kubelet            Created container consul-connect-inject-init
  Normal   Started    11m                 kubelet            Started container consul-connect-inject-init
  Normal   Pulling    11m                 kubelet            Pulling image "hashicorp/consul-dataplane:1.4.6"
  Normal   Pulled     11m                 kubelet            Successfully pulled image "hashicorp/consul-dataplane:1.4.6" in 4.32973771s (4.329758085s including waiting)
  Normal   Created    11m                 kubelet            Created container consul-dataplane
  Normal   Started    11m                 kubelet            Started container consul-dataplane
  Normal   Pulling    11m                 kubelet            Pulling image "nginxdemos/hello:0.3"
  Normal   Pulled     10m                 kubelet            Successfully pulled image "nginxdemos/hello:0.3" in 3.104498251s (3.104510502s including waiting)
  Normal   Created    10m                 kubelet            Created container nginx
  Normal   Started    10m                 kubelet            Started container nginx
  Warning  Unhealthy  61s (x70 over 10m)  kubelet            Readiness probe failed: dial tcp 10.244.3.4:20000: connect: connection refused
