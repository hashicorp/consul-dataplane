# Starting from Consul 1.18.15 consul-dataplane is stuck Unhealthy not able to pass readiness probes

## Description

Starting from Consul 1.18.15 consul-dataplane is stuck Unhealthy not able to pass readiness probes:



Events:
  Type     Reason     Age                  From               Message
  Normal   Scheduled  2m14s                default-scheduler  Successfully assigned nginx/nginx-7c6fd947b4-rbw54 to consul-worker
  Normal   Pulled     2m14s                kubelet            Container image "hashicorp/consul-k8s-control-plane:1.4.10" already present on machine
  Normal   Created    2m14s                kubelet            Created container consul-connect-inject-init
  Normal   Started    2m13s                kubelet            Started container consul-connect-inject-init
  Normal   Pulling    2m9s                 kubelet            Pulling image "hashicorp/consul-dataplane:1.4.6"
  Normal   Pulled     2m5s                 kubelet            Successfully pulled image "hashicorp/consul-dataplane:1.4.6" in 4.32973771s (4.329758085s including waiting)
  Normal   Created    2m5s                 kubelet            Created container consul-dataplane
  Normal   Started    2m5s                 kubelet            Started container consul-dataplane
  Normal   Pulling    2m5s                 kubelet            Pulling image "nginxdemos/hello:0.3"
  Normal   Pulled     2m2s                 kubelet            Successfully pulled image "nginxdemos/hello:0.3" in 3.104498251s (3.104510502s including waiting)
  Normal   Created    2m2s                 kubelet            Created container nginx
  Normal   Started    2m2s                 kubelet            Started container nginx
  Warning  Unhealthy  14s (x14 over 2m1s)  kubelet            Readiness probe failed: dial tcp 10.244.3.4:20000: connect: connection refused
 

There aren’t any special errors in server, connect-injector, connect-inject-init or consul-dataplane logs. I’ve checked 1.18.11-16. 1.18.11-14 aren’t affected. 1.18.15-1.18.16 are affected. I wonder what changed between 1.18.14 and 1.18.15?

## Timeline Summary

This work item addressed the issue of consul-dataplane being stuck in an Unhealthy state starting from Consul 1.18.15, preventing it from passing readiness probes.

The issue was confirmed to be reproducible across multiple versions, including 1.18.21+ent and 1.22.5+ent, with no specific errors in logs.

Local testing with supported versions (e.g., 1.18.5+ent with helm chart 1.4.5) did not reproduce the problem, indicating potential version-specific or configuration-related causes.

The investigation revealed a possible mismatch between the default ports used for readiness probes and envoy startup, linked to changes in default values introduced in consul-k8s and consul-dataplane.

The team is tracing release histories and code changes to identify the root cause of port mismatches and default behavior changes, with ongoing efforts to fix the issue in upcoming releases.

## Reproduction Steps

Provided in `ai/REPRO_KIND/*`

## Comments

> Himanshu Sharma

I tried to run the setup locally with Consul 1.18.5+ent and helm chart version 1.4.5. Everything runs fine.

% helm list -A
NAME  	NAMESPACE	REVISION	UPDATED                             	STATUS  	CHART       	APP VERSION
consul	consul   	2       	2026-03-13 10:21:37.546761 +0530 IST	deployed	consul-1.4.5	1.18.2
% k get po -n consul
NAME                                           READY   STATUS    RESTARTS      AGE
consul-client-p2kvn                            1/1     Running   0             28m
consul-connect-injector-5bd88545d4-zrnz9       1/1     Running   1 (27m ago)   28m
consul-server-0                                1/1     Running   0             28m
consul-webhook-cert-manager-5b9969788b-f7nf6   1/1     Running   0             28m
I didn't find consul-dataplane (for sample nginx pod) getting stuck into readiness probe.


Events:
  Type    Reason     Age   From               Message
  ----    ------     ----  ----               -------
  Normal  Scheduled  77s   default-scheduler  Successfully assigned nginx/nginx-8688f7679f-sm5cw to tool-cluster-control-plane
  Normal  Pulled     77s   kubelet            Container image "hashicorp/consul-k8s-control-plane:1.4.5" already present on machine
  Normal  Created    77s   kubelet            Created container: consul-connect-inject-init
  Normal  Started    76s   kubelet            Started container consul-connect-inject-init
  Normal  Pulling    74s   kubelet            Pulling image "hashicorp/consul-dataplane:1.4.3"
  Normal  Pulled     54s   kubelet            Successfully pulled image "hashicorp/consul-dataplane:1.4.3" in 20.049s (20.049s including waiting). Image size: 57500016 bytes.
  Normal  Created    54s   kubelet            Created container: consul-dataplane
  Normal  Started    54s   kubelet            Started container consul-dataplane
  Normal  Pulling    54s   kubelet            Pulling image "nginxdemos/hello:0.3"
  Normal  Pulled     43s   kubelet            Successfully pulled image "nginxdemos/hello:0.3" in 10.938s (10.938s including waiting). Image size: 17615401 bytes.
  Normal  Created    43s   kubelet            Created container: nginx
  Normal  Started    43s   kubelet            Started container nginx
Note: However, I’ve observed consul-dataplane readiness probe issue with 1.18.21+ent and helm chart 1.4.10. 

I couldn’t find which helm chart version customer used. @Maksim Nosal could you please share which helm chart version is having this issue. 

Lastly, I've also validated the setup with latest Consul 1.22.5+ent and helm-chart 1.9.5, and the issue persist.




% helm list -A
NAME  	NAMESPACE	REVISION	UPDATED                             	STATUS  	CHART       	APP VERSION
consul	consul   	5       	2026-03-13 10:57:44.201952 +0530 IST	deployed	consul-1.9.5	1.22.5
 



 % k get po -n nginx -w
NAME                     READY   STATUS    RESTARTS   AGE
nginx-8688f7679f-hsl78   2/2     Running   0          3m11s
 



Events:
  Type     Reason     Age    From               Message
  ----     ------     ----   ----               -------
  Normal   Scheduled  3m23s  default-scheduler  Successfully assigned nginx/nginx-8688f7679f-hsl78 to tool-cluster-control-plane
  Normal   Pulled     3m23s  kubelet            Container image "hashicorp/consul-k8s-control-plane:1.9.5" already present on machine
  Normal   Created    3m23s  kubelet            Created container: consul-connect-inject-init
  Normal   Started    3m23s  kubelet            Started container consul-connect-inject-init
  Normal   Pulling    3m21s  kubelet            Pulling image "hashicorp/consul-dataplane:1.9.5"
  Normal   Pulled     2m44s  kubelet            Successfully pulled image "hashicorp/consul-dataplane:1.9.5" in 37.569s (37.569s including waiting). Image size: 105616042 bytes.
  Normal   Created    2m44s  kubelet            Created container: consul-dataplane
  Normal   Started    2m44s  kubelet            Started container consul-dataplane
  Normal   Pulled     2m44s  kubelet            Container image "nginxdemos/hello:0.3" already present on machine
  Normal   Created    2m44s  kubelet            Created container: nginx
  Normal   Started    2m43s  kubelet            Started container nginx
  Warning  Unhealthy  2m43s  kubelet            Readiness probe failed: dial tcp 10.244.0.53:20000: connect: connection refused

> Sujay

Was involved in testing out few fixes based on the issue found earlier. Tested following annotations:
        consul.hashicorp.com/use-proxy-health-check: "true"
        consul.hashicorp.com/sidecar-proxy-lifecycle-graceful-port: "20600"

with hope of pinning graceful port. However multiple different issue came up with sidecar envoy startup which was stuck, which I’m still investigating. Going through code activity by following logs and code flow to trace the issue. Should be able to conclude soon.

> Sujay

Based on initial investigation, it seems consul-k8s introduced default values some time ago for DefaultGracefulPorts which was set to 20600 (helm: add configuration for proxy lifecycle management by mikemorris · Pull Request #2233 · hashicorp/consul-k8s ).

However, redinessProbe still checks on port 20000. Meanwhile, consul-dataplane 1.6.5 introduced a gracefulStartup phase (consul-dataplane: Triggering graceful startup if gracefulStartupSeconds is grtr than 0
[closed]) which checks for readiness of envoy on proxy addr, I suppose this is where there is a mismatch in ports.
I still need to trace its origin and why its failing (need to look across consul, consul-dataplane and consul-k8s repo releases) which I’m doing right now. Will update as soon as I’m done tracing releases and commits where this default behaviour was introduced.

