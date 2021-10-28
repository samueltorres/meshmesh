helm-dependencies:
	helm repo add nats https://nats-io.github.io/k8s/helm/charts/
	helm repo add cilium https://helm.cilium.io/
	helm repo add jetstack https://charts.jetstack.io
	helm repo update

create-test-clusters: create-test-cluster-mm1 create-test-cluster-mm2 create-test-cluster-mm3

create-test-cluster-mm1:
	./hack/test-clusters/create-cluster.sh mm1

create-test-cluster-mm2:
	./hack/test-clusters/create-cluster.sh mm2

create-test-cluster-mm3:
	./hack/test-clusters/create-cluster.sh mm3

destroy-test-clusters:
	kind delete cluster --name=mm1
	kind delete cluster --name=mm2
	kind delete cluster --name=mm3

run-mm1:
	go run main.go \
	--context="kind-mm1" \
	--cluster-name="mm1" \
	--gossip-node-name="mm1" \
	--gossip-advertise-addr="127.0.0.1" \
	--gossip-advertise-port=9001 \
	--gossip-port=9001 \
	--out-of-cluster-config \
	--gossip-join-servers="127.0.0.1:9001"

run-mm2:
	go run main.go \
	--context="kind-mm2" \
	--cluster-name="mm2" \
	--gossip-node-name="mm2" \
	--gossip-advertise-addr="127.0.0.1" \
	--gossip-advertise-port=9002 \
	--gossip-port=9002 \
	--out-of-cluster-config \
	--gossip-join-servers="127.0.0.1:9001"

run-mm3:
	go run main.go \
	--context="kind-mm3" \
	--cluster-name="mm3" \
	--gossip-node-name="mm3" \
	--gossip-advertise-addr="127.0.0.1" \
	--gossip-advertise-port=9003 \
	--gossip-port=9003 \
	--out-of-cluster-config \
	--gossip-join-servers="127.0.0.1:9001"

docker-build:
	docker build -t meshmesh:0.0.1 .

load-image-on-kind:
	kind load docker-image meshmesh:0.0.1 --name=mm1
	kind load docker-image meshmesh:0.0.1 --name=mm2
	kind load docker-image meshmesh:0.0.1 --name=mm3

build-and-load-on-kind: docker-build load-image-on-kind

helm-install-mm1:
	helm upgrade -i meshmesh ./helm -f ./hack/test-clusters/mm1/meshmesh-values.yaml --kube-context=kind-mm1
helm-install-mm2:
	helm upgrade -i meshmesh ./helm -f ./hack/test-clusters/mm2/meshmesh-values.yaml --kube-context=kind-mm2
helm-install-mm3:
	helm upgrade -i meshmesh ./helm -f ./hack/test-clusters/mm3/meshmesh-values.yaml --kube-context=kind-mm3
