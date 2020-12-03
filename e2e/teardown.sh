#!/bin/sh
#set -o errexit

reg_name='kind-registry'
export PATH=${PATH}:./bin

# delete cluster kind
kind delete cluster
docker kill ${reg_name}
docker rm ${reg_name}
