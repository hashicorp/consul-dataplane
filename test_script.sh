#!/bin/bash

CONSUL_DATAPLANE=../consul-dataplane
CONSUL_K8S=../consul-k8s
HELM_CHART=../consul-k8s/charts/consul

cd $CONSUL_DATAPLANE
make docker
docker tag consul-dataplane:1.7.0-dev consul-dataplane:local
kind load docker-image consul-dataplane:local
cd -

cd $CONSUL_K8S
make control-plane-dev-docker
docker tag consul-k8s-control-plane-dev consul-k8s-control-plane-dev:local
kind load docker-image consul-k8s-control-plane-dev:local
cd -

#helm install consul $HELM_CHART --namespace consul --create-namespace \
#    --set global.imageK8S=consul-k8s-control-plane-dev:local \
#    --set global.imageConsulDataplane=consul-dataplane:local 