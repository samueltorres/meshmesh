#!/usr/bin/env bash

CLUSTER_NAME=$1
CILIUM_VERSION="v1.10.4"

CILIUM_CLUSTER_ID="${CLUSTER_NAME/${CLUSTER_NAME_PREFIX}/}"
echo $CILIUM_CLUSTER_ID

ROOT="$(git rev-parse --show-toplevel)"
echo $ROOT
pushd $ROOT > /dev/null

# provision cluster
kind create cluster --name=$CLUSTER_NAME --config=./hack/test-clusters/$CLUSTER_NAME/kind-cluster.yaml

# setup cilium
kubectl apply -f ./hack/test-clusters/shared/cilium-ca-secret.yaml \
   --namespace kube-system \
   --context kind-$CLUSTER_NAME

helm upgrade -i cilium cilium/cilium --version $CILIUM_VERSION \
   --namespace kube-system \
   --kube-context kind-$CLUSTER_NAME \
   -f ./hack/test-clusters/$CLUSTER_NAME/cilium-values.yaml

# setup metallb
kubectl apply -f https://raw.githubusercontent.com/metallb/metallb/master/manifests/namespace.yaml --context kind-$CLUSTER_NAME
kubectl create secret generic -n metallb-system memberlist --from-literal=secretkey="$(openssl rand -base64 128)" --context kind-$CLUSTER_NAME
kubectl apply -f https://raw.githubusercontent.com/metallb/metallb/master/manifests/metallb.yaml --context kind-$CLUSTER_NAME
kubectl apply -f ./hack/test-clusters/$CLUSTER_NAME/metallb-config-map.yaml --context kind-$CLUSTER_NAME


popd > /dev/null
