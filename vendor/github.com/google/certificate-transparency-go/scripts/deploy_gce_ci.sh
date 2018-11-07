#!/usr/bin/env bash
#set -o pipefail
#set -o errexit
#set -o nounset
#set -o xtrace

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

export PROJECT_NAME=trillian-opensource-ci
export CLUSTER_NAME=trillian-opensource-ci
export MASTER_ZONE=us-central1-a

${DIR}/../trillian/examples/deployment/kubernetes/deploy.sh
