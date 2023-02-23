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
