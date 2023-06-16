# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

# This Dockerfile contains multiple targets.
# Use 'docker build --target=<name> .' to build one.
#
# Every target has a BIN_NAME argument that must be provided via --build-arg=BIN_NAME=<name>
# when building.

# envoy-binary pulls in the latest Envoy binary, as Envoy don't publish
# prebuilt binaries in any other form.
FROM envoyproxy/envoy-distroless:v1.26.2 as envoy-binary

FROM hashicorp/envoy-fips:v1.26.2 as envoy-fips-binary

# go-discover builds the discover binary (which we don't currently publish
# either).
FROM golang:1.20.4-alpine as go-discover
RUN CGO_ENABLED=0 go install github.com/hashicorp/go-discover/cmd/discover@49f60c093101c9c5f6b04d5b1c80164251a761a6

# Pull in dumb-init from alpine, as our distroless release image doesn't have a
# package manager and there's no RPM package for UBI.
FROM alpine:latest AS dumb-init
RUN apk add dumb-init

# release-default release image
# -----------------------------------
FROM gcr.io/distroless/base-debian11 AS release-default

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

COPY --from=go-discover /go/bin/discover /usr/local/bin/
COPY --from=envoy-binary /usr/local/bin/envoy /usr/local/bin/
COPY --from=dumb-init /usr/bin/dumb-init /usr/local/bin/
COPY dist/$TARGETOS/$TARGETARCH/$BIN_NAME /usr/local/bin/

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

COPY --from=go-discover /go/bin/discover /usr/local/bin/
COPY --from=envoy-fips-binary /usr/local/bin/envoy /usr/local/bin/
COPY --from=dumb-init /usr/bin/dumb-init /usr/local/bin/
COPY dist/$TARGETOS/$TARGETARCH/$BIN_NAME /usr/local/bin/

USER 100

ENTRYPOINT ["/usr/local/bin/dumb-init", "/usr/local/bin/consul-dataplane"]

# Red Hat UBI-based image
# This image is based on the Red Hat UBI base image, and has the necessary
# labels, license file, and non-root user.
# -----------------------------------
FROM registry.access.redhat.com/ubi9-minimal:9.2 as release-ubi

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

COPY dist/$TARGETOS/$TARGETARCH/$BIN_NAME /usr/local/bin/
COPY --from=go-discover /go/bin/discover /usr/local/bin/
COPY --from=envoy-binary /usr/local/bin/envoy /usr/local/bin/envoy
COPY --from=dumb-init /usr/bin/dumb-init /usr/local/bin/
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

COPY dist/$TARGETOS/$TARGETARCH/$BIN_NAME /usr/local/bin/
COPY --from=go-discover /go/bin/discover /usr/local/bin/
COPY --from=envoy-fips-binary /usr/local/bin/envoy /usr/local/bin/envoy
COPY --from=dumb-init /usr/bin/dumb-init /usr/local/bin/
COPY LICENSE /licenses/copyright.txt

USER 100
ENTRYPOINT ["/usr/local/bin/dumb-init", "/usr/local/bin/consul-dataplane"]

# ===================================
#
#   Set default target to 'release-default'.
#
# ===================================
FROM release-default
