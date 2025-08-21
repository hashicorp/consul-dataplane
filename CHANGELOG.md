## 1.8.1 (August 14, 2025)

SECURITY:

* go: upgrade go version to 1.24.5 [[GH-800](https://github.com/hashicorp/consul-dataplane/pull/800)]
* go: upgrade go discover version to 40c38fd658f0fd07ce74f2ee51b8abd3bfed01b3 [[GH-807](https://github.com/hashicorp/consul-dataplane/pull/807)]

IMPROVEMENTS:

* Update Envoy version to 1.34.4 [[GH-808](https://github.com/hashicorp/consul-dataplane/pull/808)]

## 1.7.4 (August 14, 2025)

SECURITY:

* go: upgrade go version to 1.24.5 [[GH-800](https://github.com/hashicorp/consul-dataplane/pull/800)]
* go: upgrade go discover version to 40c38fd658f0fd07ce74f2ee51b8abd3bfed01b3 [[GH-807](https://github.com/hashicorp/consul-dataplane/pull/807)]
* update: envoy to 1.33.6 [[GH-822](https://github.com/hashicorp/consul-dataplane/pull/822)]

## 1.6.8 (August 14, 2025)

SECURITY:

* go: upgrade go version to 1.24.5 [[GH-800](https://github.com/hashicorp/consul-dataplane/pull/800)]
* go: upgrade go version to 40c38fd658f0fd07ce74f2ee51b8abd3bfed01b3 [[GH-807](https://github.com/hashicorp/consul-dataplane/pull/807)]
* update: envoy to 1.32.9 [[GH-823](https://github.com/hashicorp/consul-dataplane/pull/823)]

## 1.8.0 (July 21, 2025)

IMPROVEMENTS:

* `Update `golang.org/x/net` to v0.37.0.`
* `Update `golang.org/x/sys` to v0.31.0.`
* `Update `golang.org/x/text` to v0.23.0.` [[GH-702](https://github.com/hashicorp/consul-dataplane/pull/702)]

## 1.7.3 (July 18, 2025)

SECURITY:

* Upgraded `envoy` to 1.33.5

## 1.6.7 (July 18, 2025)

SECURITY:

* Upgraded `envoy` to 1.32.8

## 1.7.2 (June 25, 2025)

SECURITY:

* cve: upgrade golang.org/x/net package to address CVE:
- [GO-2025-3595](https://pkg.go.dev/vuln/GO-2025-3595)
- [GHSA-vvgc-356p-c3xw](https://osv.dev/vulnerability/GHSA-vvgc-356p-c3xw) [[GH-764](https://github.com/hashicorp/consul-dataplane/pull/764)]
* security: upgraded go version to 1.23.10 [[GH-759](https://github.com/hashicorp/consul-dataplane/pull/759)]

## 1.6.6 (June 25, 2025)

SECURITY:

* cve: upgrade golang.org/x/net package to address CVE:
- [GO-2025-3595](https://pkg.go.dev/vuln/GO-2025-3595)
- [GHSA-vvgc-356p-c3xw](https://osv.dev/vulnerability/GHSA-vvgc-356p-c3xw) [[GH-764](https://github.com/hashicorp/consul-dataplane/pull/764)]
* security: upgraded go version to 1.23.10 [[GH-759](https://github.com/hashicorp/consul-dataplane/pull/759)]

## 1.7.1 (May 22, 2025)

SECURITY:

* CVE: update tj-actions/changed-files to fix CVE-2025-30066 [[GH-738](https://github.com/hashicorp/consul-dataplane/pull/738)]

## 1.6.5 (May 22, 2025)

SECURITY:

* CVE: update tj-actions/changed-files to fix CVE-2025-30066 [[GH-738](https://github.com/hashicorp/consul-dataplane/pull/738)]

## 1.5.8 (May 22, 2025)

SECURITY:

* CVE: update tj-actions/changed-files to fix CVE-2025-30066 [[GH-738](https://github.com/hashicorp/consul-dataplane/pull/738)]

## 1.7.0 (May 6, 2025)

SECURITY:

* Upgraded `x/net` to 0.38.0. This resolves [GO-2025-3595](https://pkg.go.dev/vuln/GO-2025-3595)
* Upgraded `envoy` to 1.33.2
* Upgraded `Go` to 1.23.8

IMPROVEMENTS:

* Triggering graceful startup if startup-grace-period-seconds is greater than 0 [[GH-687](https://github.com/hashicorp/consul-dataplane/pull/687)]
* Update Envoy version from 1.32.1 to 1.33.0 [[GH-685](https://github.com/hashicorp/consul-dataplane/pull/685)]
* Upgrade to use Go 1.23.6 [[GH-696](https://github.com/hashicorp/consul-dataplane/pull/696)]

## 1.7.0-rc2 (April 24, 2025)

* Upgraded `x/net` to 0.38.0. This resolves [GO-2025-3595](https://pkg.go.dev/vuln/GO-2025-3595)
* Upgraded `envoy` to 1.33.2
* Upgraded `Go` to 1.23.8

## 1.6.4 (April 24, 2025)

* Upgraded `x/net` to 0.38.0. This resolves [GO-2025-3595](https://pkg.go.dev/vuln/GO-2025-3595)
* Upgraded `envoy` to 1.33.2
* Upgraded `Go` to 1.23.8

## 1.6.3 (March 15, 2025)

IMPROVEMENTS:

* Triggering graceful startup if startup-grace-period-seconds is greater than 0 [[GH-687](https://github.com/hashicorp/consul-dataplane/pull/687)]

## 1.5.6 (March 15, 2025)

IMPROVEMENTS:

* Triggering graceful startup if startup-grace-period-seconds is greater than 0 [[GH-687](https://github.com/hashicorp/consul-dataplane/pull/687)]

## 1.7.0-rc1 (March 10, 2025)

IMPROVEMENTS:

* Triggering graceful startup if startup-grace-period-seconds is greater than 0 [[GH-687](https://github.com/hashicorp/consul-dataplane/pull/687)]
* Update Envoy version from 1.32.1 to 1.33.0 [[GH-685](https://github.com/hashicorp/consul-dataplane/pull/685)]
* Upgrade to use Go 1.23.6 [[GH-696](https://github.com/hashicorp/consul-dataplane/pull/696)]

## 1.5.5 (January 10, 2025)

SECURITY:

* Upgrade to support Envoy `1.29.12`. [[GH-676](https://github.com/hashicorp/consul-dataplane/pull/676)]
* Updated golang.org/x/net dependency to 0.34.0 to fix vulnerability [[GO-2024-3333](https://pkg.go.dev/vuln/GO-2024-3333)]

## 1.6.2 (January 7, 2024)

SECURITY:

* Upgrade to support Envoy `1.32.3`. [[GH-672](https://github.com/hashicorp/consul-dataplane/pull/672)]

## 1.6.1 (November 1, 2024)

SECURITY:

* Upgrade to support Envoy `1.32.1`. [[GH-661](https://github.com/hashicorp/consul-dataplane/pull/661)]

## 1.6.0 (October 15, 2024)

SECURITY:

* Upgrade Go to use 1.22.7. This addresses CVE
  [CVE-2024-34155](https://nvd.nist.gov/vuln/detail/CVE-2024-34155) [[GH-608](https://github.com/hashicorp/consul-dataplane/pull/608)]
* Upgrade envoy version to 1.31.2 to address [CVE-2024-45807](https://cve.mitre.org/cgi-bin/cvename.cgi?name=CVE-2024-45807),[CVE-2024-45808](https://cve.mitre.org/cgi-bin/cvename.cgi?name=CVE-2024-45808),[CVE-2024-45806](https://cve.mitre.org/cgi-bin/cvename.cgi?name=CVE-2024-45806),[CVE-2024-45809](https://cve.mitre.org/cgi-bin/cvename.cgi?name=CVE-2024-45809) and [CVE-2024-45810](https://cve.mitre.org/cgi-bin/cvename.cgi?name=CVE-2024-45810) [[GH-624](https://github.com/hashicorp/consul-dataplane/pull/624)]
* Upgrade to support Envoy `1.31.0`. [[GH-609](https://github.com/hashicorp/consul-dataplane/pull/609)]

FEATURES:

* Added the ability to set the `-mode` flag. Options available are `sidecar` and `dns-proxy`. The system defaults to `sidecar`.
  When set to `sidecar`:
- DNS Server, xDS Server, and Envoy are enabled.
- The system validates that `-consul-dns-bind-addr` and equivalent environment variable must be set to the loopback address.
  When set to `dns-proxy`:
- Only DNS Server is enabled. xDS Server and Envoy are disabled.
- `consul-dns-bind-addr` and equivalent environment variable can be set to other values besides the loopback address. [[GH-571](https://github.com/hashicorp/consul-dataplane/pull/571)]
* Removes the dependence on the v2 catalog and "resource-apis" experiment. [[GH-565](https://github.com/hashicorp/consul-dataplane/pull/565)]

IMPROVEMENTS:

* Update `github.com/hashicorp/consul-server-connection-manager` to v0.1.9. [[GH-595](https://github.com/hashicorp/consul-dataplane/pull/595)]
* Update `github.com/hashicorp/go-hclog` to v1.5.0. [[GH-595](https://github.com/hashicorp/consul-dataplane/pull/595)]

## 1.6.0-rc1 (September 20, 2024)

SECURITY:

* Upgrade Go to use 1.22.7. This addresses CVE
  [CVE-2024-34155](https://nvd.nist.gov/vuln/detail/CVE-2024-34155) [[GH-608](https://github.com/hashicorp/consul-dataplane/pull/608)]
* Upgrade envoy version to 1.31.2 to address [CVE-2024-45807](https://cve.mitre.org/cgi-bin/cvename.cgi?name=CVE-2024-45807),[CVE-2024-45808](https://cve.mitre.org/cgi-bin/cvename.cgi?name=CVE-2024-45808),[CVE-2024-45806](https://cve.mitre.org/cgi-bin/cvename.cgi?name=CVE-2024-45806),[CVE-2024-45809](https://cve.mitre.org/cgi-bin/cvename.cgi?name=CVE-2024-45809) and [CVE-2024-45810](https://cve.mitre.org/cgi-bin/cvename.cgi?name=CVE-2024-45810) [[GH-624](https://github.com/hashicorp/consul-dataplane/pull/624)]
* Upgrade to support Envoy `1.31.0`. [[GH-609](https://github.com/hashicorp/consul-dataplane/pull/609)]

FEATURES:

* Added the ability to set the `-mode` flag. Options available are `sidecar` and `dns-proxy`. The system defaults to `sidecar`.
  When set to `sidecar`:
- DNS Server, xDS Server, and Envoy are enabled.
- The system validates that `-consul-dns-bind-addr` and equivalent environment variable must be set to the loopback address.
  When set to `dns-proxy`:
- Only DNS Server is enabled. xDS Server and Envoy are disabled.
- `consul-dns-bind-addr` and equivalent environment variable can be set to other values besides the loopback address. [[GH-571](https://github.com/hashicorp/consul-dataplane/pull/571)]
* Removes the dependence on the v2 catalog and "resource-apis" experiment. [[GH-565](https://github.com/hashicorp/consul-dataplane/pull/565)]

IMPROVEMENTS:

* Update `github.com/hashicorp/consul-server-connection-manager` to v0.1.9. [[GH-595](https://github.com/hashicorp/consul-dataplane/pull/595)]
* Update `github.com/hashicorp/go-hclog` to v1.5.0. [[GH-595](https://github.com/hashicorp/consul-dataplane/pull/595)]

## 1.5.0 (June 12, 2024)

IMPROVEMENTS:

* Upgrade Go to use 1.22.4. [[GH-529](https://github.com/hashicorp/consul-dataplane/pull/529)]
* Upgrade to support Envoy `1.29.5`. [[GH-533](https://github.com/hashicorp/consul-dataplane/pull/533)]
* dns: queries proxied by consul-dataplane now assume the same namespace/partition/ACL token as the service registered to the dataplane instance. [[GH-172](https://github.com/hashicorp/consul-dataplane/pull/172)]

## 1.4.2 (May 21, 2024)

SECURITY:

* Upgrade Go to use 1.21.10. This addresses CVEs
  [CVE-2024-24787](https://nvd.nist.gov/vuln/detail/CVE-2024-24787) and
  [CVE-2024-24788](https://nvd.nist.gov/vuln/detail/CVE-2024-24788) [[GH-487](https://github.com/hashicorp/consul-dataplane/pull/487)]
* Upgrade to support Envoy `1.28.2`. This resolves CVE
  [CVE-2024-27919](https://nvd.nist.gov/vuln/detail/CVE-2024-27919) (`http2`). [[GH-474](https://github.com/hashicorp/consul-dataplane/pull/474)]
* Upgrade to support Envoy `1.28.3`. This resolves CVE
  [CVE-2024-32475](https://nvd.nist.gov/vuln/detail/CVE-2024-32475). [[GH-496](https://github.com/hashicorp/consul-dataplane/pull/496)]
* Upgrade to use Go `1.21.9`. This resolves CVE
  [CVE-2023-45288](https://nvd.nist.gov/vuln/detail/CVE-2023-45288) (`http2`). [[GH-474](https://github.com/hashicorp/consul-dataplane/pull/474)]
* Upgrade to use golang.org/x/net `v0.24.0`. This resolves CVE
  [CVE-2023-45288](https://nvd.nist.gov/vuln/detail/CVE-2023-45288) (`x/net`). [[GH-474](https://github.com/hashicorp/consul-dataplane/pull/474)]

IMPROVEMENTS:

* Upgrade Go to use 1.22.3. [[GH-501](https://github.com/hashicorp/consul-dataplane/pull/501)]

## 1.3.5 (May 24, 2024)
SECURITY:

* Upgrade Go to use 1.21.10. This addresses CVEs 
[CVE-2024-24787](https://nvd.nist.gov/vuln/detail/CVE-2024-24787) and
[CVE-2024-24788](https://nvd.nist.gov/vuln/detail/CVE-2024-24788) [[GH-487](https://github.com/hashicorp/consul-dataplane/pull/487)]
* Upgrade to support Envoy `1.27.4`. This resolves CVE
[CVE-2024-27919](https://nvd.nist.gov/vuln/detail/CVE-2024-27919) (`http2`). [[GH-477](https://github.com/hashicorp/consul-dataplane/pull/477)]
* Upgrade to support Envoy `1.27.5`. This resolves CVE
[CVE-2024-32475](https://nvd.nist.gov/vuln/detail/CVE-2024-32475). [[GH-497](https://github.com/hashicorp/consul-dataplane/pull/497)]
* Upgrade to use Go `1.21.9`. This resolves CVE
[CVE-2023-45288](https://nvd.nist.gov/vuln/detail/CVE-2023-45288) (`http2`). [[GH-477](https://github.com/hashicorp/consul-dataplane/pull/477)]
* Upgrade to use golang.org/x/net `v0.24.0`. This resolves CVE
[CVE-2023-45288](https://nvd.nist.gov/vuln/detail/CVE-2023-45288) (`x/net`). [[GH-477](https://github.com/hashicorp/consul-dataplane/pull/477)]

IMPROVEMENTS:

* Upgrade Go to use 1.22.3. [[GH-501](https://github.com/hashicorp/consul-dataplane/pull/501)]

## 1.2.8 (May 24, 2024)
SECURITY:

* Upgrade Go to use 1.21.10. This addresses CVEs 
[CVE-2024-24787](https://nvd.nist.gov/vuln/detail/CVE-2024-24787) and
[CVE-2024-24788](https://nvd.nist.gov/vuln/detail/CVE-2024-24788) [[GH-487](https://github.com/hashicorp/consul-dataplane/pull/487)]
* Upgrade to support Envoy `1.26.8`. This resolves CVE
[CVE-2024-27919](https://nvd.nist.gov/vuln/detail/CVE-2024-27919) (`http2`). [[GH-476](https://github.com/hashicorp/consul-dataplane/pull/476)]
* Upgrade to support Envoy `1.27.5`. This resolves CVE
[CVE-2024-32475](https://nvd.nist.gov/vuln/detail/CVE-2024-32475). [[GH-498](https://github.com/hashicorp/consul-dataplane/pull/498)]
* Upgrade to use Go `1.21.9`. This resolves CVE
[CVE-2023-45288](https://nvd.nist.gov/vuln/detail/CVE-2023-45288) (`http2`). [[GH-476](https://github.com/hashicorp/consul-dataplane/pull/476)]
* Upgrade to use golang.org/x/net `v0.24.0`. This resolves CVE
[CVE-2023-45288](https://nvd.nist.gov/vuln/detail/CVE-2023-45288) (`x/net`). [[GH-476](https://github.com/hashicorp/consul-dataplane/pull/476)]

IMPROVEMENTS:

* Upgrade Go to use 1.22.3. [[GH-501](https://github.com/hashicorp/consul-dataplane/pull/501)]

## 1.1.11 (May 20, 2024)
SECURITY:

* Upgrade Go to use 1.21.10. This addresses CVEs 
[CVE-2024-24787](https://nvd.nist.gov/vuln/detail/CVE-2024-24787) and
[CVE-2024-24788](https://nvd.nist.gov/vuln/detail/CVE-2024-24788) [[GH-487](https://github.com/hashicorp/consul-dataplane/pull/487)]
* Upgrade to support Envoy `1.26.8`. This resolves CVE
[CVE-2024-27919](https://nvd.nist.gov/vuln/detail/CVE-2024-27919) (`http2`). [[GH-475](https://github.com/hashicorp/consul-dataplane/pull/475)]
* Upgrade to support Envoy `1.27.5`. This resolves CVE
[CVE-2024-32475](https://nvd.nist.gov/vuln/detail/CVE-2024-32475). [[GH-499](https://github.com/hashicorp/consul-dataplane/pull/499)]
* Upgrade to use Go `1.21.9`. This resolves CVE
[CVE-2023-45288](https://nvd.nist.gov/vuln/detail/CVE-2023-45288) (`http2`). [[GH-475](https://github.com/hashicorp/consul-dataplane/pull/475)]
* Upgrade to use golang.org/x/net `v0.24.0`. This resolves CVE
[CVE-2023-45288](https://nvd.nist.gov/vuln/detail/CVE-2023-45288) (`x/net`). [[GH-475](https://github.com/hashicorp/consul-dataplane/pull/475)]

IMPROVEMENTS:

* Upgrade Go to use 1.22.3. [[GH-501](https://github.com/hashicorp/consul-dataplane/pull/501)]

## 1.4.1 (March 28, 2024)

SECURITY:

* Update `google.golang.org/protobuf` to v1.33.0 to address [CVE-2024-24786](https://nvd.nist.gov/vuln/detail/CVE-2024-24786). [[GH-460](https://github.com/hashicorp/consul-dataplane/pull/460)]
* Upgrade to use Go `1.21.8`. This resolves CVEs
  [CVE-2024-24783](https://nvd.nist.gov/vuln/detail/CVE-2024-24783) (`crypto/x509`).
  [CVE-2023-45290](https://nvd.nist.gov/vuln/detail/CVE-2023-45290) (`net/http`).
  [CVE-2023-45289](https://nvd.nist.gov/vuln/detail/CVE-2023-45289) (`net/http`, `net/http/cookiejar`).
  [CVE-2024-24785](https://nvd.nist.gov/vuln/detail/CVE-2024-24785) (`html/template`).
  [CVE-2024-24784](https://nvd.nist.gov/vuln/detail/CVE-2024-24784) (`net/mail`). [[GH-465](https://github.com/hashicorp/consul-dataplane/pull/465)]

## 1.3.4 (March 28, 2024)

SECURITY:

* Update `google.golang.org/protobuf` to v1.33.0 to address [CVE-2024-24786](https://nvd.nist.gov/vuln/detail/CVE-2024-24786). [[GH-460](https://github.com/hashicorp/consul-dataplane/pull/460)]
* Upgrade `consul-dataplane-fips` OpenShift container image to use `ubi9-minimal:9.3` as the base image. [[GH-434](https://github.com/hashicorp/consul-dataplane/pull/434)]
* Upgrade to use Go `1.21.8`. This resolves CVEs
  [CVE-2024-24783](https://nvd.nist.gov/vuln/detail/CVE-2024-24783) (`crypto/x509`).
  [CVE-2023-45290](https://nvd.nist.gov/vuln/detail/CVE-2023-45290) (`net/http`).
  [CVE-2023-45289](https://nvd.nist.gov/vuln/detail/CVE-2023-45289) (`net/http`, `net/http/cookiejar`).
  [CVE-2024-24785](https://nvd.nist.gov/vuln/detail/CVE-2024-24785) (`html/template`).
  [CVE-2024-24784](https://nvd.nist.gov/vuln/detail/CVE-2024-24784) (`net/mail`). [[GH-465](https://github.com/hashicorp/consul-dataplane/pull/465)]

## 1.2.7 (March 28, 2024)

SECURITY:

* Update `google.golang.org/protobuf` to v1.33.0 to address [CVE-2024-24786](https://nvd.nist.gov/vuln/detail/CVE-2024-24786). [[GH-460](https://github.com/hashicorp/consul-dataplane/pull/460)]
* Upgrade `consul-dataplane-fips` OpenShift container image to use `ubi9-minimal:9.3` as the base image. [[GH-434](https://github.com/hashicorp/consul-dataplane/pull/434)]
* Upgrade to use Go `1.21.8`. This resolves CVEs
  [CVE-2024-24783](https://nvd.nist.gov/vuln/detail/CVE-2024-24783) (`crypto/x509`).
  [CVE-2023-45290](https://nvd.nist.gov/vuln/detail/CVE-2023-45290) (`net/http`).
  [CVE-2023-45289](https://nvd.nist.gov/vuln/detail/CVE-2023-45289) (`net/http`, `net/http/cookiejar`).
  [CVE-2024-24785](https://nvd.nist.gov/vuln/detail/CVE-2024-24785) (`html/template`).
  [CVE-2024-24784](https://nvd.nist.gov/vuln/detail/CVE-2024-24784) (`net/mail`). [[GH-465](https://github.com/hashicorp/consul-dataplane/pull/465)]

## 1.1.10 (March 28, 2024)

SECURITY:

* Update `google.golang.org/protobuf` to v1.33.0 to address [CVE-2024-24786](https://nvd.nist.gov/vuln/detail/CVE-2024-24786). [[GH-460](https://github.com/hashicorp/consul-dataplane/pull/460)]
* Upgrade to use Go `1.21.8`. This resolves CVEs
[CVE-2024-24783](https://nvd.nist.gov/vuln/detail/CVE-2024-24783) (`crypto/x509`).
[CVE-2023-45290](https://nvd.nist.gov/vuln/detail/CVE-2023-45290) (`net/http`).
[CVE-2023-45289](https://nvd.nist.gov/vuln/detail/CVE-2023-45289) (`net/http`, `net/http/cookiejar`).
[CVE-2024-24785](https://nvd.nist.gov/vuln/detail/CVE-2024-24785) (`html/template`).
[CVE-2024-24784](https://nvd.nist.gov/vuln/detail/CVE-2024-24784) (`net/mail`). [[GH-465](https://github.com/hashicorp/consul-dataplane/pull/465)]

## 1.4.0 (February 28, 2024)

SECURITY:

* Update Envoy version to 1.27.2 to address [CVE-2023-44487](https://github.com/envoyproxy/envoy/security/advisories/GHSA-jhv4-f7mr-xx76) [[GH-310](https://github.com/hashicorp/consul-dataplane/pull/310)]
* Update Envoy version to 1.28.1 to address [CVE-2024-23324](https://github.com/envoyproxy/envoy/security/advisories/GHSA-gq3v-vvhj-96j6), [CVE-2024-23325](https://github.com/envoyproxy/envoy/security/advisories/GHSA-5m7c-mrwr-pm26), [CVE-2024-23322](https://github.com/envoyproxy/envoy/security/advisories/GHSA-6p83-mfmh-qv38), [CVE-2024-23323](https://github.com/envoyproxy/envoy/security/advisories/GHSA-x278-4w4x-r7ch), [CVE-2024-23327](https://github.com/envoyproxy/envoy/security/advisories/GHSA-4h5x-x9vh-m29j), and [CVE-2023-44487](https://github.com/envoyproxy/envoy/security/advisories/GHSA-jhv4-f7mr-xx76) [[GH-416](https://github.com/hashicorp/consul-dataplane/pull/416)]
* Upgrade `consul-dataplane-fips` OpenShift container image to use `ubi9-minimal:9.3` as the base image. [[GH-434](https://github.com/hashicorp/consul-dataplane/pull/434)]

FEATURES:

* Add metrics exporting directly to HCP when configured in core. [[GH-370](https://github.com/hashicorp/consul-dataplane/pull/370)]

IMPROVEMENTS:

* Propagate merged metrics request query params to Envoy to enable metrics filtering. [[GH-372](https://github.com/hashicorp/consul-dataplane/pull/372)]
* Update Envoy version from 1.27 to 1.28 [[GH-416](https://github.com/hashicorp/consul-dataplane/pull/416)]

BUG FIXES:

* Exclude Prometheus scrape path query params from Envoy path match s.t. it does not break merged metrics request routing. [[GH-372](https://github.com/hashicorp/consul-dataplane/pull/372)]

## 1.3.3 (February 14, 2024)

SECURITY:

* Update Envoy version to 1.27.3 to address [CVE-2024-23324](https://github.com/envoyproxy/envoy/security/advisories/GHSA-gq3v-vvhj-96j6), [CVE-2024-23325](https://github.com/envoyproxy/envoy/security/advisories/GHSA-5m7c-mrwr-pm26), [CVE-2024-23322](https://github.com/envoyproxy/envoy/security/advisories/GHSA-6p83-mfmh-qv38), [CVE-2024-23323](https://github.com/envoyproxy/envoy/security/advisories/GHSA-x278-4w4x-r7ch), [CVE-2024-23327](https://github.com/envoyproxy/envoy/security/advisories/GHSA-4h5x-x9vh-m29j), and [CVE-2023-44487](https://github.com/envoyproxy/envoy/security/advisories/GHSA-jhv4-f7mr-xx76) [[GH-421](https://github.com/hashicorp/consul-dataplane/pull/421)]

IMPROVEMENTS:

* Upgrade to use Go 1.21.7. [[GH-411](https://github.com/hashicorp/consul-dataplane/pull/411)]

## 1.2.6 (February 14, 2024)

SECURITY:

* Update Envoy version to 1.26.7 to address [CVE-2024-23324](https://github.com/envoyproxy/envoy/security/advisories/GHSA-gq3v-vvhj-96j6), [CVE-2024-23325](https://github.com/envoyproxy/envoy/security/advisories/GHSA-5m7c-mrwr-pm26), [CVE-2024-23322](https://github.com/envoyproxy/envoy/security/advisories/GHSA-6p83-mfmh-qv38), [CVE-2024-23323](https://github.com/envoyproxy/envoy/security/advisories/GHSA-x278-4w4x-r7ch), [CVE-2024-23327](https://github.com/envoyproxy/envoy/security/advisories/GHSA-4h5x-x9vh-m29j), and [CVE-2023-44487](https://github.com/envoyproxy/envoy/security/advisories/GHSA-jhv4-f7mr-xx76) [[GH-417](https://github.com/hashicorp/consul-dataplane/pull/417)]

IMPROVEMENTS:

* Upgrade to use Go 1.21.7. [[GH-411](https://github.com/hashicorp/consul-dataplane/pull/411)]

## 1.1.9 (February 14, 2024)

SECURITY:

* Update Envoy version to 1.26.7 to address [CVE-2024-23324](https://github.com/envoyproxy/envoy/security/advisories/GHSA-gq3v-vvhj-96j6), [CVE-2024-23325](https://github.com/envoyproxy/envoy/security/advisories/GHSA-5m7c-mrwr-pm26), [CVE-2024-23322](https://github.com/envoyproxy/envoy/security/advisories/GHSA-6p83-mfmh-qv38), [CVE-2024-23323](https://github.com/envoyproxy/envoy/security/advisories/GHSA-x278-4w4x-r7ch), [CVE-2024-23327](https://github.com/envoyproxy/envoy/security/advisories/GHSA-4h5x-x9vh-m29j), and [CVE-2023-44487](https://github.com/envoyproxy/envoy/security/advisories/GHSA-jhv4-f7mr-xx76) (note: upgrades to Envoy 1.26 for security patches due to 1.25 EOL) [[GH-418](https://github.com/hashicorp/consul-dataplane/pull/418)]

IMPROVEMENTS:

* Upgrade to use Go 1.21.7. [[GH-411](https://github.com/hashicorp/consul-dataplane/pull/411)]

## 1.3.2 (January 24, 2024)

SECURITY:

* Upgrade OpenShift container images to use `ubi9-minimal:9.3` as the base image. [[GH-373](https://github.com/hashicorp/consul-dataplane/pull/373)]

IMPROVEMENTS:

* Upgrade to use Go 1.21.6. [[GH-384](https://github.com/hashicorp/consul-dataplane/pull/384)]

## 1.2.5 (January 24, 2024)

SECURITY:

* Upgrade OpenShift container images to use `ubi9-minimal:9.3` as the base image. [[GH-373](https://github.com/hashicorp/consul-dataplane/pull/373)]

IMPROVEMENTS:

* Upgrade to use Go 1.21.6. [[GH-384](https://github.com/hashicorp/consul-dataplane/pull/384)]

## 1.1.8 (January 24, 2024)

SECURITY:

* Upgrade OpenShift container images to use `ubi9-minimal:9.3` as the base image. [[GH-373](https://github.com/hashicorp/consul-dataplane/pull/373)]

IMPROVEMENTS:

* Upgrade to use Go 1.21.6. [[GH-384](https://github.com/hashicorp/consul-dataplane/pull/384)]

## 1.3.1 (December 18, 2023)

SECURITY:

* Update Envoy version to 1.27.2 to address [CVE-2023-44487](https://github.com/envoyproxy/envoy/security/advisories/GHSA-jhv4-f7mr-xx76) [[GH-314](https://github.com/hashicorp/consul-dataplane/pull/314)]
* Upgrade to use Go 1.20.12. This resolves CVEs
  [CVE-2023-45283](https://nvd.nist.gov/vuln/detail/CVE-2023-45283): (`path/filepath`) recognize \??\ as a Root Local Device path prefix (Windows)
  [CVE-2023-45284](https://nvd.nist.gov/vuln/detail/CVE-2023-45285): recognize device names with trailing spaces and superscripts (Windows)
  [CVE-2023-39326](https://nvd.nist.gov/vuln/detail/CVE-2023-39326): (`net/http`) limit chunked data overhead
  [CVE-2023-45285](https://nvd.nist.gov/vuln/detail/CVE-2023-45285): (`cmd/go`) go get may unexpectedly fallback to insecure git [[GH-353](https://github.com/hashicorp/consul-dataplane/pull/353)]

BUG FIXES:

* Fix issue where the internal grpc-proxy would hit the max message size limit for xDS streams with a large amount of configuration. [[GH-357](https://github.com/hashicorp/consul-dataplane/pull/357)]

## 1.2.4 (December 18, 2023)

SECURITY:

* Upgrade to use Go 1.20.12. This resolves CVEs
  [CVE-2023-45283](https://nvd.nist.gov/vuln/detail/CVE-2023-45283): (`path/filepath`) recognize \??\ as a Root Local Device path prefix (Windows)
  [CVE-2023-45284](https://nvd.nist.gov/vuln/detail/CVE-2023-45285): recognize device names with trailing spaces and superscripts (Windows)
  [CVE-2023-39326](https://nvd.nist.gov/vuln/detail/CVE-2023-39326): (`net/http`) limit chunked data overhead
  [CVE-2023-45285](https://nvd.nist.gov/vuln/detail/CVE-2023-45285): (`cmd/go`) go get may unexpectedly fallback to insecure git [[GH-353](https://github.com/hashicorp/consul-dataplane/pull/353)]

BUG FIXES:

* Fix issue where the internal grpc-proxy would hit the max message size limit for xDS streams with a large amount of configuration. [[GH-357](https://github.com/hashicorp/consul-dataplane/pull/357)]

## 1.1.7 (December 18, 2023)

SECURITY:

* Upgrade to use Go 1.20.12. This resolves CVEs
  [CVE-2023-45283](https://nvd.nist.gov/vuln/detail/CVE-2023-45283): (`path/filepath`) recognize \??\ as a Root Local Device path prefix (Windows)
  [CVE-2023-45284](https://nvd.nist.gov/vuln/detail/CVE-2023-45285): recognize device names with trailing spaces and superscripts (Windows)
  [CVE-2023-39326](https://nvd.nist.gov/vuln/detail/CVE-2023-39326): (`net/http`) limit chunked data overhead
  [CVE-2023-45285](https://nvd.nist.gov/vuln/detail/CVE-2023-45285): (`cmd/go`) go get may unexpectedly fallback to insecure git [[GH-353](https://github.com/hashicorp/consul-dataplane/pull/353)]

BUG FIXES:

* Fix issue where the internal grpc-proxy would hit the max message size limit for xDS streams with a large amount of configuration. [[GH-357](https://github.com/hashicorp/consul-dataplane/pull/357)]

## 1.3.0 (November 6, 2023)

SECURITY:

* Update Envoy version to 1.27.2 to address [CVE-2023-44487](https://github.com/envoyproxy/envoy/security/advisories/GHSA-jhv4-f7mr-xx76) [[GH-315](https://github.com/hashicorp/consul-dataplane/pull/315)]
* Upgrade `google.golang.org/grpc` to 1.56.3.
  This resolves vulnerability [CVE-2023-44487](https://nvd.nist.gov/vuln/detail/CVE-2023-44487). [[GH-323](https://github.com/hashicorp/consul-dataplane/pull/323)]
* Upgrade to use Go 1.20.10 and `x/net` 0.17.0.
  This resolves [CVE-2023-39325](https://nvd.nist.gov/vuln/detail/CVE-2023-39325)
  / [CVE-2023-44487](https://nvd.nist.gov/vuln/detail/CVE-2023-44487). [[GH-299](https://github.com/hashicorp/consul-dataplane/pull/299)]

## 1.2.3 (November 1, 2023)

SECURITY:

* Update Envoy version to 1.26.6 to address [CVE-2023-44487](https://github.com/envoyproxy/envoy/security/advisories/GHSA-jhv4-f7mr-xx76) [[GH-313](https://github.com/hashicorp/consul-dataplane/pull/313)]
* Upgrade `google.golang.org/grpc` to 1.56.3.
  This resolves vulnerability [CVE-2023-44487](https://nvd.nist.gov/vuln/detail/CVE-2023-44487). [[GH-323](https://github.com/hashicorp/consul-dataplane/pull/323)]
* Upgrade to use Go 1.20.10 and `x/net` 0.17.0.
  This resolves [CVE-2023-39325](https://nvd.nist.gov/vuln/detail/CVE-2023-39325)
  / [CVE-2023-44487](https://nvd.nist.gov/vuln/detail/CVE-2023-44487). [[GH-299](https://github.com/hashicorp/consul-dataplane/pull/299)]
* Upgrade to use Go 1.20.8. This resolves CVEs
  [CVE-2023-39320](https://github.com/advisories/GHSA-rxv8-v965-v333) (`cmd/go`),
  [CVE-2023-39318](https://github.com/advisories/GHSA-vq7j-gx56-rxjh) (`html/template`),
  [CVE-2023-39319](https://github.com/advisories/GHSA-vv9m-32rr-3g55) (`html/template`),
  [CVE-2023-39321](https://github.com/advisories/GHSA-9v7r-x7cv-v437) (`crypto/tls`), and
  [CVE-2023-39322](https://github.com/advisories/GHSA-892h-r6cr-53g4) (`crypto/tls`) [[GH-261](https://github.com/hashicorp/consul-dataplane/pull/261)]

## 1.1.6 (November 1, 2023)

SECURITY:

* Update Envoy version to 1.25.11 to address [CVE-2023-44487](https://github.com/envoyproxy/envoy/security/advisories/GHSA-jhv4-f7mr-xx76) [[GH-312](https://github.com/hashicorp/consul-dataplane/pull/312)]
* Upgrade `google.golang.org/grpc` to 1.56.3.
  This resolves vulnerability [CVE-2023-44487](https://nvd.nist.gov/vuln/detail/CVE-2023-44487). [[GH-323](https://github.com/hashicorp/consul-dataplane/pull/323)]
* Upgrade to use Go 1.20.10 and `x/net` 0.17.0.
  This resolves [CVE-2023-39325](https://nvd.nist.gov/vuln/detail/CVE-2023-39325)
  / [CVE-2023-44487](https://nvd.nist.gov/vuln/detail/CVE-2023-44487). [[GH-299](https://github.com/hashicorp/consul-dataplane/pull/299)]
* Upgrade to use Go 1.20.8. This resolves CVEs
  [CVE-2023-39320](https://github.com/advisories/GHSA-rxv8-v965-v333) (`cmd/go`),
  [CVE-2023-39318](https://github.com/advisories/GHSA-vq7j-gx56-rxjh) (`html/template`),
  [CVE-2023-39319](https://github.com/advisories/GHSA-vv9m-32rr-3g55) (`html/template`),
  [CVE-2023-39321](https://github.com/advisories/GHSA-9v7r-x7cv-v437) (`crypto/tls`), and
  [CVE-2023-39322](https://github.com/advisories/GHSA-892h-r6cr-53g4) (`crypto/tls`) [[GH-261](https://github.com/hashicorp/consul-dataplane/pull/261)]

## 1.0.7 (November 1, 2023)

SECURITY:

* Update Envoy version to 1.24.12 to address [CVE-2023-44487](https://github.com/envoyproxy/envoy/security/advisories/GHSA-jhv4-f7mr-xx76) [[GH-311](https://github.com/hashicorp/consul-dataplane/pull/311)]
* Upgrade `google.golang.org/grpc` to 1.56.3.
  This resolves vulnerability [CVE-2023-44487](https://nvd.nist.gov/vuln/detail/CVE-2023-44487). [[GH-323](https://github.com/hashicorp/consul-dataplane/pull/323)]
* Upgrade to use Go 1.20.10 and `x/net` 0.17.0.
  This resolves [CVE-2023-39325](https://nvd.nist.gov/vuln/detail/CVE-2023-39325)
  / [CVE-2023-44487](https://nvd.nist.gov/vuln/detail/CVE-2023-44487). [[GH-299](https://github.com/hashicorp/consul-dataplane/pull/299)]
* Upgrade to use Go 1.20.8. This resolves CVEs
  [CVE-2023-39320](https://github.com/advisories/GHSA-rxv8-v965-v333) (`cmd/go`),
  [CVE-2023-39318](https://github.com/advisories/GHSA-vq7j-gx56-rxjh) (`html/template`),
  [CVE-2023-39319](https://github.com/advisories/GHSA-vv9m-32rr-3g55) (`html/template`),
  [CVE-2023-39321](https://github.com/advisories/GHSA-9v7r-x7cv-v437) (`crypto/tls`), and
  [CVE-2023-39322](https://github.com/advisories/GHSA-892h-r6cr-53g4) (`crypto/tls`) [[GH-261](https://github.com/hashicorp/consul-dataplane/pull/261)]

## 1.3.0-rc1 (October 10, 2023)

SECURITY:

* Update to Go 1.20.7 and Envoy 1.26.4 within the Dockerfile. [[GH-235](https://github.com/hashicorp/consul-dataplane/pull/235)]
* Upgrade to use Go 1.20.6 and `x/net/http` 0.12.0.
  This resolves [CVE-2023-29406](https://github.com/advisories/GHSA-f8f7-69v5-w4vx)(`net/http`). [[GH-219](https://github.com/hashicorp/consul-dataplane/pull/219)]
* Upgrade to use Go 1.20.7 and `x/net` 0.13.0.
  This resolves [CVE-2023-29409](https://nvd.nist.gov/vuln/detail/CVE-2023-29409)(`crypto/tls`)
  and [CVE-2023-3978](https://nvd.nist.gov/vuln/detail/CVE-2023-3978)(`net/html`). [[GH-227](https://github.com/hashicorp/consul-dataplane/pull/227)]
* Upgrade to use Go 1.20.8. This resolves CVEs
  [CVE-2023-39320](https://github.com/advisories/GHSA-rxv8-v965-v333) (`cmd/go`),
  [CVE-2023-39318](https://github.com/advisories/GHSA-vq7j-gx56-rxjh) (`html/template`),
  [CVE-2023-39319](https://github.com/advisories/GHSA-vv9m-32rr-3g55) (`html/template`),
  [CVE-2023-39321](https://github.com/advisories/GHSA-9v7r-x7cv-v437) (`crypto/tls`), and
  [CVE-2023-39322](https://github.com/advisories/GHSA-892h-r6cr-53g4) (`crypto/tls`) [[GH-261](https://github.com/hashicorp/consul-dataplane/pull/261)]

FEATURES:

* Add -shutdown-drain-listeners, -shutdown-grace-period, -graceful-shutdown-path and -graceful-port flags to configure proxy lifecycle management settings for the Envoy container. [[GH-100](https://github.com/hashicorp/consul-dataplane/pull/100)]
* Add HTTP server with configurable port and endpoint path for initiating graceful shutdown. [[GH-115](https://github.com/hashicorp/consul-dataplane/pull/115)]
* Catch SIGTERM and SIGINT to initate graceful shutdown in accordance with proxy lifecycle management configuration. [[GH-130](https://github.com/hashicorp/consul-dataplane/pull/130)]
* Make consul dataplane handle bootstrap param response for Catalog and Mesh V2 resources [[GH-242](https://github.com/hashicorp/consul-dataplane/pull/242)]

IMPROVEMENTS:

* Add graceful_startup endpoint and postStart hook in order to guarantee that dataplane starts up before application container. [[GH-239](https://github.com/hashicorp/consul-dataplane/pull/239)]
* Add the `-config-file` flag to support reading configuration options from a JSON file. [[GH-164](https://github.com/hashicorp/consul-dataplane/pull/164)]
* In order to support Windows, write Envoy bootstrap configuration to a regular file instead of a named pipe. [[GH-188](https://github.com/hashicorp/consul-dataplane/pull/188)]
* connect: Add capture group labels from Envoy cluster FQDNs to Envoy exported metric labels [[GH-184](https://github.com/hashicorp/consul-dataplane/pull/184)]

BUG FIXES:

* Add support for envoy-extra-args. Fixes [Envoy extra-args annotation crashing consul-dataplane container](https://github.com/hashicorp/consul-k8s/issues/1846). [[GH-133](https://github.com/hashicorp/consul-dataplane/pull/133)]
* Fix a bug where container user was unable to bind to privileged ports (< 1024). The consul-dataplane container now requires the NET_BIND_SERVICE capability. [[GH-238](https://github.com/hashicorp/consul-dataplane/pull/238)]
* Fix a bug where exiting envoy would inadvertently throw an error [[GH-175](https://github.com/hashicorp/consul-dataplane/pull/175)]
* Fix a bug with Envoy potentially starting with incomplete configuration by not waiting enough for initial xDS configuration. [[GH-140](https://github.com/hashicorp/consul-dataplane/pull/140)]

## 1.2.2 (September 5, 2023)

SECURITY:

* Update to Go 1.20.7 and Envoy 1.26.4 within the Dockerfile. [[GH-235](https://github.com/hashicorp/consul-dataplane/pull/235)]

BUG FIXES:

* Fix a bug where container user was unable to bind to privileged ports (< 1024). The consul-dataplane container now requires the NET_BIND_SERVICE capability. [[GH-238](https://github.com/hashicorp/consul-dataplane/pull/238)]

## 1.1.5 (September 5, 2023)

SECURITY:

* Update to Go 1.20.7 and Envoy 1.25.9 within the Dockerfile. [[GH-236](https://github.com/hashicorp/consul-dataplane/pull/236)]

BUG FIXES:

* Fix a bug where container user was unable to bind to privileged ports (< 1024). The consul-dataplane container now requires the NET_BIND_SERVICE capability. [[GH-238](https://github.com/hashicorp/consul-dataplane/pull/238)]

## 1.0.6 (September 5, 2023)

SECURITY:

* Update to Go 1.20.7 and Envoy 1.24.10 within the Dockerfile. [[GH-237](https://github.com/hashicorp/consul-dataplane/pull/237)]

BUG FIXES:

* Fix a bug where container user was unable to bind to privileged ports (< 1024). The consul-dataplane container now requires the NET_BIND_SERVICE capability. [[GH-238](https://github.com/hashicorp/consul-dataplane/pull/238)]

## 1.2.1 (August 9, 2023)

SECURITY:

* Upgrade to use Go 1.20.7 and `x/net/http` 0.12.0.
  This resolves [CVE-2023-29406](https://github.com/advisories/GHSA-f8f7-69v5-w4vx)(`net/http`). [[GH-219](https://github.com/hashicorp/consul-dataplane/pull/219)]
* Upgrade to use Go 1.20.7 and `x/net` 0.13.0.
  This resolves [CVE-2023-29409](https://nvd.nist.gov/vuln/detail/CVE-2023-29409)(`crypto/tls`)
  and [CVE-2023-3978](https://nvd.nist.gov/vuln/detail/CVE-2023-3978)(`net/html`). [[GH-227](https://github.com/hashicorp/consul-dataplane/pull/227)]

FEATURES:

* Add -shutdown-drain-listeners, -shutdown-grace-period, -graceful-shutdown-path and -graceful-port flags to configure proxy lifecycle management settings for the Envoy container. [[GH-100](https://github.com/hashicorp/consul-dataplane/pull/100)]
* Add HTTP server with configurable port and endpoint path for initiating graceful shutdown. [[GH-115](https://github.com/hashicorp/consul-dataplane/pull/115)]
* Catch SIGTERM and SIGINT to initate graceful shutdown in accordance with proxy lifecycle management configuration. [[GH-130](https://github.com/hashicorp/consul-dataplane/pull/130)]

IMPROVEMENTS:

* connect: Add capture group labels from Envoy cluster FQDNs to Envoy exported metric labels [[GH-184](https://github.com/hashicorp/consul-dataplane/pull/184)]

BUG FIXES:

* Add support for envoy-extra-args. Fixes [Envoy extra-args annotation crashing consul-dataplane container](https://github.com/hashicorp/consul-k8s/issues/1846). [[GH-133](https://github.com/hashicorp/consul-dataplane/pull/133)]
* Fix a bug where exiting envoy would inadvertently throw an error [[GH-175](https://github.com/hashicorp/consul-dataplane/pull/175)]
* Fix a bug with Envoy potentially starting with incomplete configuration by not waiting enough for initial xDS configuration. [[GH-140](https://github.com/hashicorp/consul-dataplane/pull/140)]


## 1.1.4 (August 9, 2023)

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

## 1.0.5 (August 9, 2023)

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

## 1.2.0 (June 28, 2023)

SECURITY:

* Update go-discover to 214571b6a5309addf3db7775f4ee8cf4d264fd5f within the Dockerfile. [[GH-153](https://github.com/hashicorp/consul-dataplane/pull/153)]
* Update to Envoy 1.26.2 within the Dockerfile. [[GH-142](https://github.com/hashicorp/consul-dataplane/pull/142)]
* Update to Go 1.20.4 and Envoy 1.26.1 within the Dockerfile. [[GH-97](https://github.com/hashicorp/consul-dataplane/pull/97)]


## 1.1.3 (June 28, 2023)

SECURITY:

* Update go-discover to 214571b6a5309addf3db7775f4ee8cf4d264fd5f within the Dockerfile. [[GH-153](https://github.com/hashicorp/consul-dataplane/pull/153)]

FEATURES:

* Add -shutdown-drain-listeners, -shutdown-grace-period, -graceful-shutdown-path and -graceful-port flags to configure proxy lifecycle management settings for the Envoy container. [[GH-100](https://github.com/hashicorp/consul-dataplane/pull/100)]
* Add HTTP server with configurable port and endpoint path for initiating graceful shutdown. [[GH-115](https://github.com/hashicorp/consul-dataplane/pull/115)]
* Catch SIGTERM and SIGINT to initate graceful shutdown in accordance with proxy lifecycle management configuration. [[GH-130](https://github.com/hashicorp/consul-dataplane/pull/130)]

BUG FIXES:

* Add support for envoy-extra-args. Fixes [Envoy extra-args annotation crashing consul-dataplane container](https://github.com/hashicorp/consul-k8s/issues/1846). [[GH-133](https://github.com/hashicorp/consul-dataplane/pull/133)]
* Fix a bug where exiting envoy would inadvertently throw an error [[GH-175](https://github.com/hashicorp/consul-dataplane/pull/175)]

## 1.0.4 (June 28, 2023)

SECURITY:

* Update go-discover to 214571b6a5309addf3db7775f4ee8cf4d264fd5f within the Dockerfile. [[GH-153](https://github.com/hashicorp/consul-dataplane/pull/153)]

FEATURES:

* Add -shutdown-drain-listeners, -shutdown-grace-period, -graceful-shutdown-path and -graceful-port flags to configure proxy lifecycle management settings for the Envoy container. [[GH-100](https://github.com/hashicorp/consul-dataplane/pull/100)]
* Add HTTP server with configurable port and endpoint path for initiating graceful shutdown. [[GH-115](https://github.com/hashicorp/consul-dataplane/pull/115)]
* Catch SIGTERM and SIGINT to initate graceful shutdown in accordance with proxy lifecycle management configuration. [[GH-130](https://github.com/hashicorp/consul-dataplane/pull/130)]

BUG FIXES:

* Add support for envoy-extra-args. Fixes [Envoy extra-args annotation crashing consul-dataplane container](https://github.com/hashicorp/consul-k8s/issues/1846). [[GH-133](https://github.com/hashicorp/consul-dataplane/pull/133)]
* Fix a bug where exiting envoy would inadvertently throw an error [[GH-175](https://github.com/hashicorp/consul-dataplane/pull/175)]

## 1.1.2 (June 1, 2023)

BUG FIXES:

* Reverts #104 fix that caused a downstream error for Ingress/Mesh/Terminating GWs [[GH-131](https://github.com/hashicorp/consul-dataplane/pull/131)]

## 1.0.3 (June 1, 2023)

SECURITY:

* Update to UBI base image to 9.2. [[GH-125](https://github.com/hashicorp/consul-dataplane/pull/125)]

IMPROVEMENTS:

* Update bootstrap configuration to rename envoy_hcp_metrics_bind_socket_dir to envoy_telemetry_collector_bind_socket_dir to remove HCP naming references. [[GH-122](https://github.com/hashicorp/consul-dataplane/pull/122)]

BUG FIXES:

* Reverts #104 fix that caused a downstream error for Ingress/Mesh/Terminating GWs [[GH-131](https://github.com/hashicorp/consul-dataplane/pull/131)]

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
