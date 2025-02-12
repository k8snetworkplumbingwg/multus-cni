#!/bin/sh
set -o errexit

if [ ! -d bin ]; then
	mkdir bin
fi

GOBIN=$(pwd)/bin go install sigs.k8s.io/kind@53ff1fb77a1e
curl -Lo ./bin/kubectl https://storage.googleapis.com/kubernetes-release/release/`curl -s https://storage.googleapis.com/kubernetes-release/release/stable.txt`/bin/linux/amd64/kubectl
chmod +x ./bin/kubectl
curl -Lo ./bin/koko https://github.com/redhat-nfvpe/koko/releases/download/v0.83/koko_0.83_linux_amd64
chmod +x ./bin/koko
curl -Lo ./bin/jq https://github.com/stedolan/jq/releases/download/jq-1.6/jq-linux64
chmod +x ./bin/jq
wget -qO- https://get.helm.sh/helm-v3.14.3-linux-amd64.tar.gz | tar xvzf - --strip-components=1 -C ./bin linux-amd64/helm
