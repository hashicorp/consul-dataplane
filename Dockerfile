# Dockerfile to build a docker image for containarized platforms.
# The Docker image has:
#  - consul-dataplane CLI
#  - envoy binary
#  - go-discover CLI

FROM envoyproxy/envoy:v1.23-latest as envoy-binary

FROM golang:1.18-alpine as consul-dataplane-binary
WORKDIR /cdp
# TODO: Directly go install the consul-dataplane CLI once repo is public
COPY cmd ./cmd
COPY pkg ./pkg
COPY internal ./internal
COPY go.mod ./
COPY go.sum ./
RUN go mod download
RUN go build ./cmd/consul-dataplane

FROM golang:1.18-alpine as go-discover-binary
RUN go install github.com/hashicorp/go-discover/cmd/discover@latest

FROM alpine:3.16 as consul-dataplane-container
WORKDIR /root/
RUN apk add gcompat
COPY --from=consul-dataplane-binary /cdp/consul-dataplane /usr/local/bin/consul-dataplane
COPY --from=envoy-binary /usr/local/bin/envoy /usr/local/bin/envoy
COPY --from=go-discover-binary /go/bin/discover /usr/local/bin/discover
ENTRYPOINT [ "consul-dataplane" ]
