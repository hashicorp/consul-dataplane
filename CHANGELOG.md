## 1.0.6 (September 5 2023)

SECURITY:

* Update to Go 1.20.7 and Envoy 1.24.10 within the Dockerfile. [[GH-237](https://github.com/hashicorp/consul-dataplane/pull/237)]

BUG FIXES:

* Fix a bug where container user was unable to bind to privileged ports (< 1024). The consul-dataplane container now requires the NET_BIND_SERVICE capability. [[GH-238](https://github.com/hashicorp/consul-dataplane/pull/238)]

## 1.0.5 (August 9 2023)

SECURITY:

* Upgrade to use Go 1.20.6 and `x/net/http` 0.12.0.
  This resolves [CVE-2023-29406](https://github.com/advisories/GHSA-f8f7-69v5-w4vx)(`net/http`). [[GH-219](https://github.com/hashicorp/consul-dataplane/pull/219)]
* Upgrade to use Go 1.20.7 and `x/net` 0.13.0.
  This resolves [CVE-2023-29409](https://nvd.nist.gov/vuln/detail/CVE-2023-29409)(`crypto/tls`)
  and [CVE-2023-3978](https://nvd.nist.gov/vuln/detail/CVE-2023-3978)(`net/html`). [[GH-227](https://github.com/hashicorp/consul-dataplane/pull/227)]

IMPROVEMENTS:

* connect: Add capture group labels from Envoy cluster FQDNs to Envoy exported metric labels [[GH-184](https://github.com/hashicorp/consul-dataplane/pull/184)]

BUG FIXES:

* Fix a bug with Envoy potentially starting with incomplete configuration by not waiting enough for initial xDS configuration. [[GH-140](https://github.com/hashicorp/consul-dataplane/pull/140)]


## 1.0.4 (June 28 2023)

SECURITY:

* Update go-discover to 214571b6a5309addf3db7775f4ee8cf4d264fd5f within the Dockerfile. [[GH-153](https://github.com/hashicorp/consul-dataplane/pull/153)]

FEATURES:

* Add -shutdown-drain-listeners, -shutdown-grace-period, -graceful-shutdown-path and -graceful-port flags to configure proxy lifecycle management settings for the Envoy container. [[GH-100](https://github.com/hashicorp/consul-dataplane/pull/100)]
* Add HTTP server with configurable port and endpoint path for initiating graceful shutdown. [[GH-115](https://github.com/hashicorp/consul-dataplane/pull/115)]
* Catch SIGTERM and SIGINT to initate graceful shutdown in accordance with proxy lifecycle management configuration. [[GH-130](https://github.com/hashicorp/consul-dataplane/pull/130)]

BUG FIXES:

* Add support for envoy-extra-args. Fixes [Envoy extra-args annotation crashing consul-dataplane container](https://github.com/hashicorp/consul-k8s/issues/1846). [[GH-133](https://github.com/hashicorp/consul-dataplane/pull/133)]
* Fix a bug where exiting envoy would inadvertently throw an error [[GH-175](https://github.com/hashicorp/consul-dataplane/pull/175)]


## 1.0.3 (June 1, 2023)

SECURITY:

* Update to UBI base image to 9.2. [[GH-125](https://github.com/hashicorp/consul-dataplane/pull/125)]

IMPROVEMENTS:

* Update bootstrap configuration to rename envoy_hcp_metrics_bind_socket_dir to envoy_telemetry_collector_bind_socket_dir to remove HCP naming references. [[GH-122](https://github.com/hashicorp/consul-dataplane/pull/122)]

BUG FIXES:

* Reverts #104 fix that caused a downstream error for Ingress/Mesh/Terminating GWs [[GH-131](https://github.com/hashicorp/consul-dataplane/pull/131)]

## 1.0.2 (May 16, 2023)

SECURITY:

* Update to Go 1.20.4 and Envoy 1.24.7 within the Dockerfile. [[GH-99](https://github.com/hashicorp/consul-dataplane/pull/99)]
* Upgrade golang/x/net to 0.7.0
This resolves vulnerability [CVE-2022-41723](https://github.com/golang/go/issues/57855) in `x/net` [[GH-81](https://github.com/hashicorp/consul-dataplane/pull/81)]
* Upgrade to use Go 1.20.1. 
This resolves vulnerabilities [CVE-2022-41724](https://go.dev/issue/58001) in `crypto/tls` and [CVE-2022-41723](https://go.dev/issue/57855) in `net/http`. [[GH-78](https://github.com/hashicorp/consul-dataplane/pull/78)]
* Upgrade to use Go 1.20.4. 
This resolves vulnerabilities [CVE-2023-24537](https://github.com/advisories/GHSA-9f7g-gqwh-jpf5)(`go/scanner`), 
[CVE-2023-24538](https://github.com/advisories/GHSA-v4m2-x4rp-hv22)(`html/template`), 
[CVE-2023-24534](https://github.com/advisories/GHSA-8v5j-pwr7-w5f8)(`net/textproto`) and 
[CVE-2023-24536](https://github.com/advisories/GHSA-9f7g-gqwh-jpf5)(`mime/multipart`). [[GH-94](https://github.com/hashicorp/consul-dataplane/pull/94)]

FEATURES:

* Add envoy_hcp_metrics_bind_socket_dir flag to configure a directory where a unix socket is created. 
This enables Envoy metrics collection, which will be forwarded to a HCP metrics collector. [[GH-90](https://github.com/hashicorp/consul-dataplane/pull/90)]

IMPROVEMENTS:

* Update consul-server-connection-manager to version 0.1.2. [[GH-77](https://github.com/hashicorp/consul-dataplane/pull/77)]

BUG FIXES:

* Fix a bug that threw an error when trying to use `$HOST_IP` with metrics URLs. [[GH-106](https://github.com/hashicorp/consul-dataplane/pull/106)]
* Fix a bug with Envoy potentially starting with incomplete configuration by not waiting enough for initial xDS configuration. [[GH-104](https://github.com/hashicorp/consul-dataplane/pull/104)]

## 1.0.1 (January 27, 2023)

SECURITY:

* Update to Go 1.19.4 and Envoy 1.24.1 within the Dockerfile. [[GH-64](https://github.com/hashicorp/consul-dataplane/pull/64)]

IMPROVEMENTS:

* Update consul-server-connection-manager to version 0.1.1. [[GH-66](https://github.com/hashicorp/consul-dataplane/pull/66)]


## 1.0.0 (November 16, 2022)

Initial release.
