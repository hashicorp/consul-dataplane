## 1.1.1 (May 31, 2023)

SECURITY:

* Update to Go 1.20.4 and Envoy 1.25.6 within the Dockerfile. [[GH-98](https://github.com/hashicorp/consul-dataplane/pull/98)]
* Update to UBI base image to 9.2. [[GH-125](https://github.com/hashicorp/consul-dataplane/pull/125)]
* Upgrade to use Go 1.20.4. 
This resolves vulnerabilities [CVE-2023-24537](https://github.com/advisories/GHSA-9f7g-gqwh-jpf5)(`go/scanner`), 
[CVE-2023-24538](https://github.com/advisories/GHSA-v4m2-x4rp-hv22)(`html/template`), 
[CVE-2023-24534](https://github.com/advisories/GHSA-8v5j-pwr7-w5f8)(`net/textproto`) and 
[CVE-2023-24536](https://github.com/advisories/GHSA-9f7g-gqwh-jpf5)(`mime/multipart`). [[GH-94](https://github.com/hashicorp/consul-dataplane/pull/94)]

FEATURES:

* Add envoy_hcp_metrics_bind_socket_dir flag to configure a directory where a unix socket is created. 
This enables Envoy metrics collection, which will be forwarded to a HCP metrics collector. [[GH-90](https://github.com/hashicorp/consul-dataplane/pull/90)]

IMPROVEMENTS:

* Update bootstrap configuration to rename envoy_hcp_metrics_bind_socket_dir to envoy_telemetry_collector_bind_socket_dir to remove HCP naming references. [[GH-122](https://github.com/hashicorp/consul-dataplane/pull/122)]

BUG FIXES:

* Fix a bug that threw an error when trying to use `$HOST_IP` with metrics URLs. [[GH-106](https://github.com/hashicorp/consul-dataplane/pull/106)]
* Fix a bug with Envoy potentially starting with incomplete configuration by not waiting enough for initial xDS configuration. [[GH-104](https://github.com/hashicorp/consul-dataplane/pull/104)]

## 1.1.0 (February 23, 2023)

SECURITY:

* Update Envoy to 1.25.1 within the Dockerfile. [[GH-71](https://github.com/hashicorp/consul-dataplane/pull/71)]
* Upgrade golang/x/net to 0.7.0
This resolves vulnerability [CVE-2022-41723](https://github.com/golang/go/issues/57855) in `x/net` [[GH-81](https://github.com/hashicorp/consul-dataplane/pull/81)]
* Upgrade to use Go 1.20.1. 
This resolves vulnerabilities [CVE-2022-41724](https://go.dev/issue/58001) in `crypto/tls` and [CVE-2022-41723](https://go.dev/issue/57855) in `net/http`. [[GH-78](https://github.com/hashicorp/consul-dataplane/pull/78)]

FEATURES:

* support Envoy admin [access logs](https://developer.hashicorp.com/consul/docs/connect/observability/access-logs). [[GH-65](https://github.com/hashicorp/consul-dataplane/pull/65)]

IMPROVEMENTS:

* Update consul-server-connection-manager to version 0.1.2. [[GH-74](https://github.com/hashicorp/consul-dataplane/pull/74)]

## 1.0.1 (January 27, 2023)

SECURITY:

* Update to Go 1.19.4 and Envoy 1.24.1 within the Dockerfile. [[GH-64](https://github.com/hashicorp/consul-dataplane/pull/64)]

IMPROVEMENTS:

* Update consul-server-connection-manager to version 0.1.1. [[GH-66](https://github.com/hashicorp/consul-dataplane/pull/66)]


## 1.0.0 (November 16, 2022)

Initial release.
