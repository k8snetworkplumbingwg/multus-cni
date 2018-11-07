#!/usr/bin/env bash
#set -o pipefail
#set -o errexit
#set -o nounset
#set -o xtrace

function checkEnv() {
  if [ -z ${PROJECT_ID+x} ] ||
     [ -z ${CLUSTER_NAME+x} ] ||
     [ -z ${MASTER_ZONE+x} ]; then
    echo "You must either pass an argument which is a config file, or set all the required environment variables"
    exit 1
  fi
}

set -e

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
if [ $# -eq 1 ]; then
  source $1
else
  checkEnv
fi

export IMAGE_TAG=${IMAGE_TAG:-$(git rev-parse HEAD)}

gcloud --quiet config set project ${PROJECT_ID}
gcloud --quiet config set container/cluster ${CLUSTER_NAME}
gcloud --quiet config set compute/zone ${MASTER_ZONE}
gcloud --quiet container clusters get-credentials ${CLUSTER_NAME}

configmaps=$(kubectl get configmaps)
if [[ ! "${configmaps}" =~ "ctfe-configmap" ]]; then
  echo "Missing ctfe config map."
  echo
  echo "Ensure you have a PEM file containing all the roots your log should accept."
  echo "and a working CTFE configuration file, then create a CTFE configmap by"
  echo "running the following command:"
  echo "  kubectl create configmap ctfe-configmap \\"
  echo "     --from-file=roots=path/to/all-roots.pem \\"
  echo "     --from-file=ctfe-config-file=path/to/ct_server.cfg \\"
  echo "     --from-literal=cloud-project=${PROJECT_ID}"
  echo
  echo "Once you've created the configmap, re-run this script"
  exit 1
fi


echo "Building docker images.."
cd $GOPATH/src/github.com/google/certificate-transparency-go
docker build --quiet -f trillian/examples/deployment/docker/ctfe/Dockerfile -t gcr.io/${PROJECT_ID}/ctfe:${IMAGE_TAG} .

echo "Pushing docker image..."
gcloud docker -- push gcr.io/${PROJECT_ID}/ctfe:${IMAGE_TAG}

echo "Tagging docker image..."
gcloud --quiet container images add-tag gcr.io/${PROJECT_ID}/ctfe:${IMAGE_TAG} gcr.io/${PROJECT_ID}/ctfe:latest

echo "Updating jobs..."
envsubst < trillian/examples/deployment/kubernetes/ctfe-deployment.yaml | kubectl apply -f -
envsubst < trillian/examples/deployment/kubernetes/ctfe-service.yaml | kubectl apply -f -
envsubst < trillian/examples/deployment/kubernetes/ctfe-ingress.yaml | kubectl apply -f -
kubectl set image deployment/trillian-ctfe-deployment trillian-ctfe=gcr.io/${PROJECT_ID}/ctfe:${IMAGE_TAG}

echo "CTFE is available at:"
kubectl get ingress
