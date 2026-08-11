#!/bin/sh
set -o errexit

if [ ! -d bin ]; then
	mkdir bin
fi

curl -Lo ./bin/kind "https://github.com/kubernetes-sigs/kind/releases/download/v0.27.0/kind-$(uname)-amd64"
chmod +x ./bin/kind
curl -Lo ./bin/kubectl https://storage.googleapis.com/kubernetes-release/release/`curl -s https://storage.googleapis.com/kubernetes-release/release/stable.txt`/bin/linux/amd64/kubectl
chmod +x ./bin/kubectl
curl -Lo ./bin/koko https://github.com/redhat-nfvpe/koko/releases/download/v0.83/koko_0.83_linux_amd64
chmod +x ./bin/koko
curl -Lo ./bin/jq https://github.com/stedolan/jq/releases/download/jq-1.6/jq-linux64
chmod +x ./bin/jq
wget -qO- https://get.helm.sh/helm-v3.14.3-linux-amd64.tar.gz | tar xvzf - --strip-components=1 -C ./bin linux-amd64/helm

KUBE_BURNER_VERSION=V2.7.3
curl -L --fail -o /tmp/kube-burner.tar.gz \
	"https://github.com/kube-burner/kube-burner/releases/download/v2.7.3/kube-burner-${KUBE_BURNER_VERSION}-linux-x86_64.tar.gz"
tar -xz -C ./bin -f /tmp/kube-burner.tar.gz kube-burner
rm -f /tmp/kube-burner.tar.gz
chmod +x ./bin/kube-burner
