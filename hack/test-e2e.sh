#!/usr/bin/env bash

set -ex

KUBECONFIG=${KUBECONFIG:-"$HOME/.kube/config"}
go test ./e2e --kubeconfig ${KUBECONFIG} -ginkgo.v
