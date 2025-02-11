#!/bin/bash

CONSUL_DATAPLANE=../consul-dataplane
CONSUL_K8S=../thomas/consul-k8s
HELM_CHART=../thomas/consul-k8s/charts/consul

cd $CONSUL_DATAPLANE
make docker
docker tag consul-dataplane/release-default:1.3.0-dev consul-dataplane/release-default:local
kind load docker-image consul-dataplane/release-default:local
cd -

cd $CONSUL_K8S
make control-plane-dev-docker
docker tag consul-k8s-control-plane-dev consul-k8s-control-plane-dev:local
kind load docker-image consul-k8s-control-plane-dev:local
cd -

helm install consul $HELM_CHART --namespace consul --create-namespace \
    --set global.imageK8S=consul-k8s-control-plane-dev:local \
    --set global.imageConsulDataplane=consul-dataplane/release-default:local 