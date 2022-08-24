# Dockerfile to build a docker image for containarized platforms.
# The Docker image has:
#  - consul-dataplane CLI
#  - envoy binary
#  - [to-be-included] go-discover CLI

FROM envoyproxy/envoy:v1.23-latest as envoy-binary

FROM golang:1.18 as consul-dataplane-binary
WORKDIR /cdp
# TODO: Directly go install the consul-dataplane CLI once repo is public
COPY cmd ./cmd
COPY pkg ./pkg
COPY internal ./internal
COPY go.mod ./
COPY go.sum ./
RUN go mod download
RUN go build ./cmd/consul-dataplane
# TODO (NET-722): Add go-discover to the docker image once PR
# to fix depencencies is merged. (https://github.com/hashicorp/go-discover/pull/202)
# RUN go get -u github.com/hashicorp/go-discover/cmd/discover

FROM ubuntu:latest as consul-dataplane-container
WORKDIR /root/
COPY --from=consul-dataplane-binary /cdp/consul-dataplane ./
COPY --from=envoy-binary /usr/local/bin/envoy /usr/local/bin/envoy
ENTRYPOINT [ "./consul-dataplane" ]
