# This Dockerfile contains multiple targets.
# Use 'docker build --target=<name> .' to build one.
#
# Every target has a BIN_NAME argument that must be provided via --build-arg=BIN_NAME=<name>
# when building.

# envoy-binary pulls in the latest Envoy binary, as Envoy don't publish
# prebuilt binaries in any other form.
FROM envoyproxy/envoy:v1.23.1 as envoy-binary

# go-discover builds the discover binary (which we don't currently publish
# either).
FROM golang:1.19.1-alpine as go-discover
RUN go install github.com/hashicorp/go-discover/cmd/discover@49f60c093101c9c5f6b04d5b1c80164251a761a6

# ===================================
#
#   Release images.
#
# ===================================

# release-default release image
# -----------------------------------
FROM alpine:3.16 AS release-default

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

# Create a non-root user to run the software.
RUN addgroup $PRODUCT_NAME && \
    adduser -S -G $PRODUCT_NAME 100

COPY dist/$TARGETOS/$TARGETARCH/$BIN_NAME /usr/local/bin/
COPY --from=go-discover /go/bin/discover /usr/local/bin/
COPY --from=envoy-binary /usr/local/bin/envoy /usr/local/bin/envoy
RUN apk add gcompat dumb-init

USER 100
ENTRYPOINT ["/usr/bin/dumb-init", "/usr/local/bin/consul-dataplane"]

# Red Hat UBI-based image
# This image is based on the Red Hat UBI base image, and has the necessary
# labels, license file, and non-root user.
# -----------------------------------
FROM registry.access.redhat.com/ubi8/ubi-minimal:8.6 as release-ubi

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

RUN microdnf install -y wget shadow-utils

# dumb-init is downloaded directly from GitHub because there's no RPM package.
# Its shasum is hardcoded. If you upgrade the dumb-init verion you'll need to
# also update the shasum.
RUN set -eux && \
    wget -O /usr/local/bin/dumb-init https://github.com/Yelp/dumb-init/releases/download/v1.2.5/dumb-init_1.2.5_x86_64 && \
    echo 'e874b55f3279ca41415d290c512a7ba9d08f98041b28ae7c2acb19a545f1c4df /usr/local/bin/dumb-init' > dumb-init-shasum && \
    sha256sum --check dumb-init-shasum && \
    chmod +x /usr/local/bin/dumb-init

# Create a non-root user to run the software.
RUN groupadd --gid 1000 $PRODUCT_NAME && \
    adduser --uid 100 --system -g $PRODUCT_NAME $PRODUCT_NAME && \
    usermod -a -G root $PRODUCT_NAME

COPY dist/$TARGETOS/$TARGETARCH/$BIN_NAME /usr/local/bin/
COPY --from=go-discover /go/bin/discover /usr/local/bin/
COPY --from=envoy-binary /usr/local/bin/envoy /usr/local/bin/envoy
COPY LICENSE /licenses/copyright.txt

USER 100
ENTRYPOINT ["/usr/local/bin/dumb-init", "/usr/local/bin/consul-dataplane"]

# ===================================
#
#   Set default target to 'release-default'.
#
# ===================================
FROM release-default
