# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

# This Dockerfile contains multiple targets.
# Use 'docker build --target=<name> .' to build one.
#
# Every target has a BIN_NAME argument that must be provided via --build-arg=BIN_NAME=<name>
# when building.

# envoy-binary pulls in the latest Envoy binary, as Envoy don't publish
# prebuilt binaries in any other form.
FROM hashicorp/envoy:1.26.4 as envoy-binary

# Modify the envoy binary to be able to bind to privileged ports (< 1024).
FROM debian:bullseye-slim AS setcap-envoy-binary

ARG BIN_NAME=consul-dataplane
ARG TARGETARCH
ARG TARGETOS

COPY --from=envoy-binary /usr/local/bin/envoy /usr/local/bin/
COPY dist/$TARGETOS/$TARGETARCH/$BIN_NAME /usr/local/bin/

RUN apt-get update && apt install -y libcap2-bin
RUN setcap CAP_NET_BIND_SERVICE=+ep /usr/local/bin/envoy
RUN setcap CAP_NET_BIND_SERVICE=+ep /usr/local/bin/$BIN_NAME

FROM hashicorp/envoy-fips:v1.26.4 as envoy-fips-binary

# Modify the envoy-fips binary to be able to bind to privileged ports (< 1024).
FROM debian:bullseye-slim AS setcap-envoy-fips-binary

ARG BIN_NAME=consul-dataplane
ARG TARGETARCH
ARG TARGETOS

COPY --from=envoy-fips-binary /usr/local/bin/envoy /usr/local/bin/
COPY dist/$TARGETOS/$TARGETARCH/$BIN_NAME /usr/local/bin/

RUN apt-get update && apt install -y libcap2-bin
RUN setcap CAP_NET_BIND_SERVICE=+ep /usr/local/bin/envoy
RUN setcap CAP_NET_BIND_SERVICE=+ep /usr/local/bin/$BIN_NAME

# go-discover builds the discover binary (which we don't currently publish
# either).
FROM golang:1.20.8-alpine as go-discover
RUN CGO_ENABLED=0 go install github.com/hashicorp/go-discover/cmd/discover@214571b6a5309addf3db7775f4ee8cf4d264fd5f

# Pull in dumb-init from alpine, as our distroless release image doesn't have a
# package manager and there's no RPM package for UBI.
FROM alpine:latest AS dumb-init
RUN apk add dumb-init

# release-default release image
# -----------------------------------
FROM gcr.io/distroless/base-debian11 AS release-default

ARG BIN_NAME=consul-dataplane
ENV BIN_NAME=$BIN_NAME
ARG PRODUCT_VERSION
ARG PRODUCT_REVISION
ARG PRODUCT_NAME=$BIN_NAME

# TARGETARCH and TARGETOS are set automatically when --platform is provided.
ARG TARGETOS TARGETARCH

LABEL name=${BIN_NAME}\
      maintainer="Consul Team <consul@hashicorp.com>" \
      vendor="HashiCorp" \
      version=${PRODUCT_VERSION} \
      release=${PRODUCT_REVISION} \
      revision=${PRODUCT_REVISION} \
      summary="Consul dataplane manages the proxy that runs within the data plane layer of Consul Service Mesh." \
      description="Consul dataplane manages the proxy that runs within the data plane layer of Consul Service Mesh."

COPY --from=dumb-init /usr/bin/dumb-init /usr/local/bin/
COPY --from=go-discover /go/bin/discover /usr/local/bin/
COPY --from=setcap-envoy-binary /usr/local/bin/envoy /usr/local/bin/
COPY --from=setcap-envoy-binary /usr/local/bin/$BIN_NAME /usr/local/bin/
COPY LICENSE /licenses/copyright.txt

USER 100

ENTRYPOINT ["/usr/local/bin/dumb-init", "/usr/local/bin/consul-dataplane"]

# FIPS release-default release image
# -----------------------------------
FROM gcr.io/distroless/base-debian11 AS release-fips-default

ARG BIN_NAME
ARG PRODUCT_VERSION
ARG PRODUCT_REVISION
ARG PRODUCT_NAME=$BIN_NAME

# TARGETARCH and TARGETOS are set automatically when --platform is provided.
ARG TARGETOS TARGETARCH

LABEL name=${BIN_NAME}\
      maintainer="Consul Team <consul@hashicorp.com>" \
      vendor="HashiCorp" \
      version=${PRODUCT_VERSION} \
      release=${PRODUCT_REVISION} \
      revision=${PRODUCT_REVISION} \
      summary="Consul dataplane manages the proxy that runs within the data plane layer of Consul Service Mesh." \
      description="Consul dataplane manages the proxy that runs within the data plane layer of Consul Service Mesh."

COPY --from=dumb-init /usr/bin/dumb-init /usr/local/bin/
COPY --from=go-discover /go/bin/discover /usr/local/bin/
COPY --from=setcap-envoy-fips-binary /usr/local/bin/envoy /usr/local/bin/
COPY --from=setcap-envoy-fips-binary /usr/local/bin/$BIN_NAME /usr/local/bin/
COPY LICENSE /licenses/copyright.txt

USER 100

ENTRYPOINT ["/usr/local/bin/dumb-init", "/usr/local/bin/consul-dataplane"]

# Red Hat UBI-based image
# This image is based on the Red Hat UBI base image, and has the necessary
# labels, license file, and non-root user.
# -----------------------------------
FROM registry.access.redhat.com/ubi9-minimal:9.2 as release-ubi

ARG BIN_NAME=consul-dataplane
ENV BIN_NAME=$BIN_NAME
ARG PRODUCT_VERSION
ARG PRODUCT_REVISION
ARG PRODUCT_NAME=$BIN_NAME
# TARGETARCH and TARGETOS are set automatically when --platform is provided.
ARG TARGETOS TARGETARCH

LABEL name=${BIN_NAME}\
      maintainer="Consul Team <consul@hashicorp.com>" \
      vendor="HashiCorp" \
      version=${PRODUCT_VERSION} \
      release=${PRODUCT_REVISION} \
      revision=${PRODUCT_REVISION} \
      summary="Consul dataplane connects an application to a Consul service mesh." \
      description="Consul dataplane connects an application to a Consul service mesh."

RUN microdnf install -y shadow-utils

# Create a non-root user to run the software.
RUN groupadd --gid 1000 $PRODUCT_NAME && \
    adduser --uid 100 --system -g $PRODUCT_NAME $PRODUCT_NAME && \
    usermod -a -G root $PRODUCT_NAME

COPY --from=dumb-init /usr/bin/dumb-init /usr/local/bin/
COPY --from=go-discover /go/bin/discover /usr/local/bin/
COPY --from=setcap-envoy-binary /usr/local/bin/envoy /usr/local/bin/
COPY --from=setcap-envoy-binary /usr/local/bin/$BIN_NAME /usr/local/bin/
COPY LICENSE /licenses/copyright.txt

USER 100
ENTRYPOINT ["/usr/local/bin/dumb-init", "/usr/local/bin/consul-dataplane"]

# FIPS Red Hat UBI-based image
# This image is based on the Red Hat UBI base image, and has the necessary
# labels, license file, and non-root user.
# -----------------------------------
FROM registry.access.redhat.com/ubi9-minimal:9.2 as release-fips-ubi

ARG BIN_NAME
ENV BIN_NAME=$BIN_NAME
ARG PRODUCT_VERSION
ARG PRODUCT_REVISION
ARG PRODUCT_NAME=$BIN_NAME
# TARGETARCH and TARGETOS are set automatically when --platform is provided.
ARG TARGETOS TARGETARCH

LABEL name=${BIN_NAME}\
      maintainer="Consul Team <consul@hashicorp.com>" \
      vendor="HashiCorp" \
      version=${PRODUCT_VERSION} \
      release=${PRODUCT_REVISION} \
      revision=${PRODUCT_REVISION} \
      summary="Consul dataplane connects an application to a Consul service mesh." \
      description="Consul dataplane connects an application to a Consul service mesh."

RUN microdnf install -y shadow-utils

# Create a non-root user to run the software.
RUN groupadd --gid 1000 $PRODUCT_NAME && \
    adduser --uid 100 --system -g $PRODUCT_NAME $PRODUCT_NAME && \
    usermod -a -G root $PRODUCT_NAME

COPY --from=dumb-init /usr/bin/dumb-init /usr/local/bin/
COPY --from=go-discover /go/bin/discover /usr/local/bin/
COPY --from=setcap-envoy-fips-binary /usr/local/bin/envoy /usr/local/bin/
COPY --from=setcap-envoy-fips-binary /usr/local/bin/$BIN_NAME /usr/local/bin/
COPY LICENSE /licenses/copyright.txt

USER 100
ENTRYPOINT ["/usr/local/bin/dumb-init", "/usr/local/bin/consul-dataplane"]

# ===================================
#
#   Set default target to 'release-default'.
#
# ===================================
FROM release-default
